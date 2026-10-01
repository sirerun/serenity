package deletion

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type fakeS3State struct {
	mu             sync.Mutex
	objects        map[string][]byte
	putHeaderSeen  bool
	failAfterWrite bool
	conflict       bool
}

func newFakeS3(t *testing.T) (*S3ObjectStore, *fakeS3State) {
	t.Helper()
	state := &fakeS3State{objects: map[string][]byte{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/test-bucket/"), "/", 2)
		key := parts[0]
		if len(parts) > 1 {
			key += "/" + parts[1]
		}
		state.mu.Lock()
		defer state.mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			state.putHeaderSeen = r.Header.Get("If-None-Match") == "*"
			if !state.putHeaderSeen {
				s3Error(w, http.StatusForbidden, "AccessDenied")
				return
			}
			if state.conflict {
				state.conflict = false
				s3Error(w, http.StatusConflict, "ConditionalRequestConflict")
				return
			}
			if _, exists := state.objects[key]; exists {
				s3Error(w, http.StatusPreconditionFailed, "PreconditionFailed")
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read put body: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			state.objects[key] = body
			if state.failAfterWrite {
				state.failAfterWrite = false
				s3Error(w, http.StatusInternalServerError, "InternalError")
				return
			}
			w.Header().Set("ETag", `"etag"`)
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			if _, versions := r.URL.Query()["versions"]; versions {
				keys := make([]string, 0, len(state.objects))
				for k := range state.objects {
					if strings.HasPrefix(k, r.URL.Query().Get("prefix")) && k > r.URL.Query().Get("key-marker") {
						keys = append(keys, k)
					}
				}
				sort.Strings(keys)
				_, _ = fmt.Fprint(w, `<ListVersionsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>test-bucket</Name><Prefix>`, r.URL.Query().Get("prefix"), `</Prefix><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>`)
				for _, k := range keys {
					_, _ = fmt.Fprintf(w, `<Version><Key>%s</Key><VersionId>v1</VersionId><IsLatest>true</IsLatest></Version>`, k)
				}
				_, _ = fmt.Fprint(w, `</ListVersionsResult>`)
				return
			}
			body, ok := state.objects[key]
			if !ok {
				s3Error(w, http.StatusNotFound, "NoSuchKey")
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		default:
			s3Error(w, http.StatusMethodNotAllowed, "MethodNotAllowed")
		}
	}))
	t.Cleanup(server.Close)
	cfg := aws.Config{Region: "us-west-2", Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""), Retryer: func() aws.Retryer { return aws.NopRetryer{} }}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(server.URL)
		o.UsePathStyle = true
	})
	store, err := NewS3ObjectStore(client, "test-bucket")
	if err != nil {
		t.Fatal(err)
	}
	return store, state
}

func s3Error(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<Error><Code>%s</Code><Message>test</Message><RequestId>test</RequestId></Error>`, code)
}

func TestS3ObjectStoreConditionalWriteAndAmbiguousRetry(t *testing.T) {
	store, state := newFakeS3(t)
	ctx := context.Background()
	key := "deletion-journal/0000000001/0000000001.json"
	created, err := store.PutIfAbsent(ctx, key, []byte("first"))
	if err != nil || !created {
		t.Fatalf("first put = %v, %v", created, err)
	}
	if !state.putHeaderSeen {
		t.Fatal("PutObject did not include If-None-Match: *")
	}
	created, err = store.PutIfAbsent(ctx, key, []byte("different"))
	if err != nil || created {
		t.Fatalf("duplicate put = %v, %v; want definitive 412", created, err)
	}
	state.mu.Lock()
	state.failAfterWrite = true
	state.mu.Unlock()
	key2 := "deletion-journal/0000000001/0000000002.json"
	created, err = store.PutIfAbsent(ctx, key2, []byte("ambiguous"))
	if err != nil || !created {
		t.Fatalf("ambiguous put/readback = %v, %v", created, err)
	}
	body, found, err := store.Get(ctx, key2)
	if err != nil || !found || string(body) != "ambiguous" {
		t.Fatalf("get after ambiguous put = %q, %v, %v", body, found, err)
	}
	keys, more, err := store.ListAfter(ctx, "deletion-journal/0000000001/", "", 1)
	if err != nil || !more || len(keys) != 1 || keys[0] != key {
		t.Fatalf("first list page = %v, %v, %v", keys, more, err)
	}
	keys, more, err = store.ListAfter(ctx, "deletion-journal/0000000001/", keys[0], 1)
	if err != nil || more || len(keys) != 1 || keys[0] != key2 {
		t.Fatalf("second list page = %v, %v, %v", keys, more, err)
	}
	state.mu.Lock()
	state.conflict = true
	state.mu.Unlock()
	if _, err = store.PutIfAbsent(ctx, "deletion-journal/0000000001/0000000003.json", []byte("conflict")); err == nil {
		t.Fatal("409 conflict was treated as a definite occupied slot")
	}
}
