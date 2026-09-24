// Package deletion implements the durable hosted deletion journal.
package deletion

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// S3API is the subset of the AWS SDK used by S3ObjectStore.
type S3API interface {
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	ListObjectVersions(context.Context, *s3.ListObjectVersionsInput, ...func(*s3.Options)) (*s3.ListObjectVersionsOutput, error)
}

// S3ObjectStore maps the append-only journal object contract to a versioned
// S3 bucket. The configured bucket must have no lifecycle expiration rule for
// deletion-journal/, must require If-None-Match: *, and must deny all deletes
// on that prefix. The journal checks version listings so a delete marker is a
// visible gap rather than an empty journal.
type S3ObjectStore struct {
	client S3API
	bucket string
}

func NewS3ObjectStore(client S3API, bucket string) (*S3ObjectStore, error) {
	if client == nil || strings.TrimSpace(bucket) == "" {
		return nil, errors.New("hosted/deletion: S3 client and bucket are required")
	}
	return &S3ObjectStore{client: client, bucket: bucket}, nil
}

func (s *S3ObjectStore) validateKey(key string) error {
	if !strings.HasPrefix(key, "deletion-journal/") || path.Clean(key) != key || strings.Contains(key, "\\") {
		return fmt.Errorf("hosted/deletion: invalid journal key %q", key)
	}
	return nil
}

// PutIfAbsent reports false only for S3's definitive 412 precondition result.
// A lost response is resolved by reading the exact key and comparing bytes;
// 409, 404, cancellation, and other unresolved errors are preserved as errors.
func (s *S3ObjectStore) PutIfAbsent(ctx context.Context, key string, body []byte) (bool, error) {
	if err := s.validateKey(key); err != nil {
		return false, err
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(body),
		IfNoneMatch: aws.String("*"),
	})
	if err == nil {
		return true, nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == "PreconditionFailed" {
		return false, nil
	}
	if ctx.Err() != nil || !ambiguousPut(err) {
		return false, err
	}
	stored, found, readErr := s.Get(ctx, key)
	if readErr == nil && found && bytes.Equal(stored, body) {
		return true, nil
	}
	return false, errors.Join(err, readErr)
}

func ambiguousPut(err error) bool {
	var response *smithyhttp.ResponseError
	if errors.As(err, &response) {
		return response.HTTPStatusCode() >= 500
	}
	var netErr net.Error
	if errors.As(err, &netErr) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	return false
}

func (s *S3ObjectStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if err := s.validateKey(key); err != nil {
		return nil, false, err
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchKey" {
			return nil, false, nil
		}
		var response *smithyhttp.ResponseError
		if errors.As(err, &response) && response.HTTPStatusCode() == 404 {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer func() { _ = out.Body.Close() }()
	body, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, false, err
	}
	return body, true, nil
}

// ListAfter uses ListObjectVersions, not ListObjectsV2, so a current delete
// marker remains visible to the journal verifier. Repeated versions of one key
// collapse to a single sorted key.
func (s *S3ObjectStore) ListAfter(ctx context.Context, prefix, startAfter string, limit int) ([]string, bool, error) {
	if !strings.HasPrefix(prefix, "deletion-journal/") || path.Clean(prefix)+"/" != prefix {
		return nil, false, errors.New("hosted/deletion: invalid journal prefix")
	}
	if limit <= 0 {
		return nil, false, errors.New("hosted/deletion: list limit must be positive")
	}
	keys := make([]string, 0, limit)
	seen := make(map[string]struct{}, limit)
	keyMarker := startAfter
	var versionMarker *string
	for {
		input := &s3.ListObjectVersionsInput{
			Bucket:          aws.String(s.bucket),
			Prefix:          aws.String(prefix),
			VersionIdMarker: versionMarker,
			MaxKeys:         aws.Int32(1000),
		}
		if keyMarker != "" {
			input.KeyMarker = aws.String(keyMarker)
		}
		out, err := s.client.ListObjectVersions(ctx, input)
		if err != nil {
			return nil, false, err
		}
		pageKeys := make([]string, 0, len(out.Versions)+len(out.DeleteMarkers))
		for _, version := range out.Versions {
			if version.Key != nil {
				pageKeys = append(pageKeys, *version.Key)
			}
		}
		for _, marker := range out.DeleteMarkers {
			if marker.Key != nil {
				pageKeys = append(pageKeys, *marker.Key)
			}
		}
		sort.Strings(pageKeys)
		for _, k := range pageKeys {
			if k <= startAfter {
				continue
			}
			if _, exists := seen[k]; exists {
				continue
			}
			seen[k] = struct{}{}
			keys = append(keys, k)
			if len(keys) > limit {
				return keys[:limit], true, nil
			}
		}
		if aws.ToBool(out.IsTruncated) {
			keyMarker = aws.ToString(out.NextKeyMarker)
			versionMarker = out.NextVersionIdMarker
			continue
		}
		return keys, false, nil
	}
}

// FileObjectStore is an explicit durable fake for local runs and tests. It
// offers the same exclusive-create property without pretending to be the
// production substrate.
type FileObjectStore struct{ root string }

func NewFileObjectStore(root string) (*FileObjectStore, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("hosted/deletion: filesystem journal root must be absolute")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &FileObjectStore{root: root}, nil
}

func (s *FileObjectStore) objectPath(key string) (string, error) {
	if !strings.HasPrefix(key, "deletion-journal/") || path.Clean(key) != key || strings.Contains(key, "\\") {
		return "", fmt.Errorf("hosted/deletion: invalid journal key %q", key)
	}
	p := filepath.Join(s.root, filepath.FromSlash(key))
	rel, err := filepath.Rel(s.root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("hosted/deletion: journal key escapes filesystem root")
	}
	return p, nil
}

func (s *FileObjectStore) PutIfAbsent(_ context.Context, key string, body []byte) (bool, error) {
	p, err := s.objectPath(key)
	if err != nil {
		return false, err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return false, err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, writeErr := f.Write(body)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return false, err
	}
	return true, nil
}

func (s *FileObjectStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	p, err := s.objectPath(key)
	if err != nil {
		return nil, false, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return b, err == nil, err
}

func (s *FileObjectStore) ListAfter(_ context.Context, prefix, startAfter string, limit int) ([]string, bool, error) {
	if limit <= 0 || !strings.HasPrefix(prefix, "deletion-journal/") {
		return nil, false, errors.New("hosted/deletion: invalid journal list request")
	}
	base, err := s.objectPath(strings.TrimSuffix(prefix, "/") + "/placeholder")
	if err != nil {
		return nil, false, err
	}
	dir := filepath.Dir(base)
	var keys []string
	err = filepath.WalkDir(dir, func(file string, entry os.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return filepath.SkipDir
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("hosted/deletion: non-regular object in filesystem journal")
		}
		rel, err := filepath.Rel(s.root, file)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if strings.HasPrefix(key, prefix) && key > startAfter {
			keys = append(keys, key)
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	sort.Strings(keys)
	if len(keys) > limit {
		return keys[:limit], true, nil
	}
	return keys, false, nil
}

var _ interface {
	PutIfAbsent(context.Context, string, []byte) (bool, error)
	Get(context.Context, string) ([]byte, bool, error)
	ListAfter(context.Context, string, string, int) ([]string, bool, error)
} = (*S3ObjectStore)(nil)

var _ interface {
	PutIfAbsent(context.Context, string, []byte) (bool, error)
	Get(context.Context, string) ([]byte, bool, error)
	ListAfter(context.Context, string, string, int) ([]string, bool, error)
} = (*FileObjectStore)(nil)
