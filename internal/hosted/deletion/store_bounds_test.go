package deletion

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type boundedBody struct {
	io.Reader
	read   int
	closed bool
}

func (b *boundedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}
func (b *boundedBody) Close() error { b.closed = true; return nil }

type boundsS3 struct {
	S3API
	body   *boundedBody
	length *int64
	puts   int
}

func (s *boundsS3) GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return &s3.GetObjectOutput{Body: s.body, ContentLength: s.length}, nil
}
func (s *boundsS3) PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	s.puts++
	return &s3.PutObjectOutput{}, nil
}

func TestS3GetObjectBound(t *testing.T) {
	const limit = 64 * 1024
	for _, tc := range []struct {
		name      string
		size      int
		length    *int64
		wantError bool
	}{
		{"exact limit", limit, aws.Int64(limit), false},
		{"one over", limit + 1, aws.Int64(limit + 1), true},
		{"unknown length", limit * 8, nil, true},
		{"underreported length", limit * 8, aws.Int64(1), true},
		{"negative length", limit * 8, aws.Int64(-1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &boundedBody{Reader: bytes.NewReader(bytes.Repeat([]byte("x"), tc.size))}
			client := &boundsS3{body: body, length: tc.length}
			store, err := NewS3ObjectStore(client, "test-bucket")
			if err != nil {
				t.Fatal(err)
			}
			got, found, err := store.Get(context.Background(), "deletion-journal/1/entry.json")
			if (err != nil) != tc.wantError {
				t.Errorf("Get error=%v, want error=%v", err, tc.wantError)
			}
			if tc.wantError && (found || got != nil) {
				t.Error("oversized response exposed partial data")
			}
			if !tc.wantError && (!found || len(got) != tc.size) {
				t.Errorf("valid response: found=%v bytes=%d", found, len(got))
			}
			if body.read > limit+1 {
				t.Errorf("read %d bytes, limit is %d", body.read, limit+1)
			}
			if !body.closed {
				t.Error("response body was not closed")
			}
		})
	}
}

func TestS3PutObjectBound(t *testing.T) {
	for _, size := range []int{64 * 1024, 64*1024 + 1} {
		client := &boundsS3{}
		store, err := NewS3ObjectStore(client, "test-bucket")
		if err != nil {
			t.Fatal(err)
		}
		created, err := store.PutIfAbsent(context.Background(), "deletion-journal/1/entry.json", make([]byte, size))
		if size > 64*1024 {
			if err == nil || created || client.puts != 0 {
				t.Errorf("oversized put: created=%v err=%v calls=%d", created, err, client.puts)
			}
		} else if err != nil || !created || client.puts != 1 {
			t.Errorf("valid put: created=%v err=%v calls=%d", created, err, client.puts)
		}
	}
}
