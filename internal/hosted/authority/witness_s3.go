package authority

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type WitnessS3API interface {
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

type S3Witness struct {
	api            WitnessS3API
	bucket, prefix string
}

func NewS3Witness(api WitnessS3API, bucket, prefix string) (*S3Witness, error) {
	if api == nil || bucket == "" || prefix == "" || strings.HasPrefix(prefix, "/") || strings.Contains(prefix, "..") || !strings.HasSuffix(prefix, "/") {
		return nil, ErrInvalid
	}
	return &S3Witness{api: api, bucket: bucket, prefix: prefix}, nil
}
func missing(err error) bool {
	var e smithy.APIError
	return errors.As(err, &e) && (e.ErrorCode() == "NoSuchKey" || e.ErrorCode() == "NotFound")
}
func (s *S3Witness) get(ctx context.Context, key string, max int64) ([]byte, string, bool, error) {
	out, err := s.api.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(s.prefix + key)})
	if err != nil {
		if missing(err) {
			return nil, "", false, nil
		}
		return nil, "", false, err
	}
	if out == nil || out.Body == nil {
		return nil, "", false, ErrUnavailable
	}
	defer out.Body.Close()
	if out.DeleteMarker != nil && *out.DeleteMarker {
		return nil, "", false, ErrConflict
	}
	if out.ContentLength != nil && (*out.ContentLength < 0 || *out.ContentLength > max) {
		return nil, "", false, ErrInvalid
	}
	body, err := io.ReadAll(io.LimitReader(out.Body, max+1))
	if err != nil {
		return nil, "", false, err
	}
	if int64(len(body)) > max {
		return nil, "", false, ErrInvalid
	}
	if out.ContentLength != nil && *out.ContentLength != int64(len(body)) {
		return nil, "", false, ErrUnavailable
	}
	return body, aws.ToString(out.ETag), true, nil
}
func (s *S3Witness) tip(ctx context.Context) (WitnessTip, string, bool, error) {
	b, etag, found, err := s.get(ctx, "TIP", 4096)
	if err != nil || !found {
		return WitnessTip{}, "", found, err
	}
	var t WitnessTip
	if err = strictDecode(b, &t); err != nil {
		return WitnessTip{}, "", false, err
	}
	if !validTip(t) || etag == "" {
		return WitnessTip{}, "", false, ErrInvalid
	}
	return t, etag, true, nil
}
func (s *S3Witness) Tip(ctx context.Context) (WitnessTip, bool, error) {
	t, _, f, e := s.tip(ctx)
	return t, f, e
}
func validTip(t WitnessTip) bool {
	return t.Lineage != "" && t.TableARN != "" && t.Sequence > 0 && t.Sequence <= 100000 && t.TotalBytes > 0 && t.TotalBytes <= 64*1024*1024 && validID(t.SHA256) && t.Key == fmt.Sprintf("%020d/%s.json", t.Sequence, t.SHA256)
}
func (s *S3Witness) Read(ctx context.Context, t WitnessTip) ([]byte, error) {
	if !validTip(t) {
		return nil, ErrInvalid
	}
	b, _, found, err := s.get(ctx, "objects/"+t.Key, 4*MaxBodyBytes)
	if err != nil {
		return nil, err
	}
	if !found || digest(b) != t.SHA256 {
		return nil, ErrConflict
	}
	return b, nil
}
func (s *S3Witness) Advance(ctx context.Context, p Pending) error {
	if !validTip(p.Next) || digest(p.Witness) != p.Next.SHA256 || p.Next.Sequence != p.Previous.Sequence+1 || p.Next.TotalBytes != p.Previous.TotalBytes+int64(len(p.Witness)) {
		return ErrInvalid
	}
	if p.Bootstrap {
		if p.Previous != (WitnessTip{}) || p.Next.Sequence != 1 {
			return ErrForbidden
		}
	} else if !validTip(p.Previous) || p.Next.Lineage != p.Previous.Lineage || p.Next.TableARN != p.Previous.TableARN && !p.OwnerRebind {
		return ErrForbidden
	}
	// A conflicting immutable witness is fatal. Every uncertain PUT is resolved
	// by bounded byte-for-byte readback; a bare 412 is never durability proof.
	_, putErr := s.api.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(s.prefix + "objects/" + p.Next.Key), Body: bytes.NewReader(p.Witness), ContentLength: aws.Int64(int64(len(p.Witness))), IfNoneMatch: aws.String("*")})
	stored, err := s.Read(ctx, p.Next)
	if err != nil {
		if putErr != nil {
			return fmt.Errorf("witness write unresolved: %w", ErrUnavailable)
		}
		return err
	}
	if !bytes.Equal(stored, p.Witness) {
		return ErrConflict
	}
	actual, etag, found, err := s.tip(ctx)
	if err != nil {
		return err
	}
	if found && actual == p.Next {
		return nil
	}
	if p.Bootstrap {
		if found {
			return ErrConflict
		}
	} else if !found || actual != p.Previous {
		return ErrConflict
	}
	body, err := json.Marshal(p.Next)
	if err != nil {
		return err
	}
	in := &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(s.prefix + "TIP"), Body: bytes.NewReader(body), ContentLength: aws.Int64(int64(len(body)))}
	if p.Bootstrap {
		in.IfNoneMatch = aws.String("*")
	} else {
		in.IfMatch = aws.String(etag)
	}
	_, putErr = s.api.PutObject(ctx, in)
	actual, _, found, err = s.tip(ctx)
	if err != nil || !found {
		return ErrUnavailable
	}
	if actual != p.Next {
		if putErr != nil {
			return ErrConflict
		}
		return ErrUnavailable
	}
	return nil
}

// History follows the independently authenticated effective TIP. Orphan
// candidate witnesses are never listed, adopted, or mistaken for acknowledged
// history. A bounded traversal that reaches a limit fails closed.
func History(ctx context.Context, s WitnessStore, tip WitnessTip, maxRecords, maxBytes int) ([][]byte, error) {
	if s == nil || maxRecords < 1 || maxRecords > 100000 || maxBytes < 1 || maxBytes > 64*1024*1024 || !validTip(tip) {
		return nil, ErrInvalid
	}
	current := tip
	var reversed [][]byte
	total := 0
	for current != (WitnessTip{}) {
		if len(reversed) >= maxRecords {
			return nil, ErrInvalid
		}
		b, err := s.Read(ctx, current)
		if err != nil {
			return nil, err
		}
		total += len(b)
		if total > maxBytes {
			return nil, ErrInvalid
		}
		var w witnessRecord
		if err = strictDecode(b, &w); err != nil {
			return nil, err
		}
		if w.Version != 1 || w.Sequence != current.Sequence || w.Lineage != current.Lineage || w.TableARN != current.TableARN || w.Previous.Sequence+1 != w.Sequence || current.TotalBytes != w.Previous.TotalBytes+int64(len(b)) {
			return nil, ErrConflict
		}
		if w.Sequence == 1 && w.Previous != (WitnessTip{}) {
			return nil, ErrConflict
		}
		reversed = append(reversed, b)
		current = w.Previous
	}
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return reversed, nil
}
func strictDecode(b []byte, out any) error {
	if !json.Valid(b) || rejectDuplicateKeys(json.NewDecoder(bytes.NewReader(b)), 0) != nil {
		return ErrInvalid
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("decode: %w", ErrInvalid)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return ErrInvalid
	}
	return nil
}

func rejectDuplicateKeys(d *json.Decoder, depth int) error {
	if depth > 64 {
		return ErrInvalid
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case 123:
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return ErrInvalid
			}
			seen[name] = true
			if err = rejectDuplicateKeys(d, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(125) {
			return ErrInvalid
		}
	case 91:
		for d.More() {
			if err = rejectDuplicateKeys(d, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(93) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}
