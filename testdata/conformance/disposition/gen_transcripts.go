//go:build ignore

// Command gen_transcripts captures testdata/conformance/disposition/'s
// four transcript files (T4.13) by running real HTTP round trips against
// a real *internal/server.Server wrapping real
// internal/server/disposition.Handlers over a real internal/disposition.Store
// and internal/events.Store (index.Open-backed) -- the identical
// real-listener harness internal/server/disposition/disposition_test.go
// itself uses (T4.4), just driven from a standalone command instead of
// *testing.T so its captures can be written to disk and frozen.
//
// These are recordings, not hand-authored fixtures: every request_body
// and response in the generated JSON is exactly what a live handler sent
// and received. Re-run after a DELIBERATE change to DISPOSITION v1's wire
// shapes or this generator's own scenarios, then re-pin the manifest:
//
//	GOWORK=off go run testdata/conformance/disposition/gen_transcripts.go
//
// (GOWORK=off per docs/lore.md L-0001 -- unnecessary from an offloaded
// worktree, required from a plain sirerun/serenity checkout.)
//
// This intentionally overwrites the checksum-frozen fixtures in place --
// review the diff before committing, the same discipline
// evals/corpora/*/gen_manifest.go and gen_corpus.go already establish for
// their own checksum-pinned corpora.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sirerun/serenity/internal/conformance"
	coredisp "github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/events"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/secrets"
	"github.com/sirerun/serenity/internal/server"
	serverdisposition "github.com/sirerun/serenity/internal/server/disposition"
)

const outDir = "testdata/conformance/disposition"

var fixedNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

// env is one fresh, isolated server + store pair -- one per Case, the
// same isolation newTestEnv gives each *testing.T in the real test
// suite, so cases never see each other's seeded items.
type env struct {
	dispStore *coredisp.Store
	evStore   *events.Store
	base      string
	token     string
	client    *http.Client
	cancel    context.CancelFunc
	done      chan struct{}
}

func newEnv(opts ...serverdisposition.Option) *env {
	dir, err := os.MkdirTemp("", "gen-disposition-*")
	if err != nil {
		log.Fatalf("MkdirTemp: %v", err)
	}
	eng, err := index.Open(filepath.Join(dir, "index.db"))
	if err != nil {
		log.Fatalf("index.Open: %v", err)
	}
	dispStore := coredisp.NewStore(eng)
	evStore := events.NewStore(eng)
	allOpts := append([]serverdisposition.Option{
		serverdisposition.WithClock(fakeClock{fixedNow}),
		serverdisposition.WithPollInterval(20 * time.Millisecond),
	}, opts...)
	h := serverdisposition.New(dispStore, evStore, allOpts...)

	const token = "s3cr3t-token"
	s := server.New(server.Config{Bind: "127.0.0.1:0", TokenSource: func() (string, error) { return token, nil }})
	h.Register(s)
	if err := s.Listen(); err != nil {
		log.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = s.Serve(ctx)
		close(done)
	}()
	return &env{
		dispStore: dispStore, evStore: evStore,
		base: "http://" + s.Addr(), token: token,
		client: &http.Client{Transport: &http.Transport{DisableKeepAlives: true}},
		cancel: cancel, done: done,
	}
}

func (e *env) close() {
	e.cancel()
	select {
	case <-e.done:
	case <-time.After(8 * time.Second):
		log.Fatal("server did not shut down within 8s")
	}
}

// step performs one real HTTP request and returns the captured
// conformance.Step. body may be nil for a bodyless GET.
func (e *env) step(method, path string, body any, noAuth bool) conformance.Step {
	var reqBytes json.RawMessage
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			log.Fatalf("marshal request body: %v", err)
		}
		reqBytes = b
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.base+path, reader)
	if err != nil {
		log.Fatalf("NewRequest: %v", err)
	}
	if !noAuth {
		req.Header.Set("Authorization", "Bearer "+e.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.client.Do(req)
	if err != nil {
		log.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("read response body: %v", err)
	}
	return conformance.Step{
		Method: method, Path: path, RequestBody: reqBytes, NoAuth: noAuth,
		Response: conformance.StepResponse{Status: resp.StatusCode, Body: string(respBody)},
	}
}

func writeTranscript(operation string, cases ...conformance.Case) {
	tr := conformance.Transcript{Protocol: "disposition", Operation: operation, Cases: cases}
	b, err := json.MarshalIndent(tr, "", "  ")
	if err != nil {
		log.Fatalf("marshal %s transcript: %v", operation, err)
	}
	path := filepath.Join(outDir, operation+".json")
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		log.Fatalf("write %s: %v", path, err)
	}
	fmt.Printf("wrote %s (%d cases)\n", path, len(cases))
}

func main() {
	secrets.MockForTesting() // never touch the real OS keychain from a generator run
	genListPending()
	genDispose()
	genCapture()
	genSubscribeLongpoll()

	if err := conformance.WriteManifest(outDir); err != nil {
		log.Fatalf("WriteManifest: %v", err)
	}
	fmt.Printf("wrote %s\n", filepath.Join(outDir, conformance.ManifestFile))
}

func genListPending() {
	var cases []conformance.Case

	// list_pending filters by kind (T4.4 acc: exercised generally by
	// list_pending's own filter path).
	func() {
		e := newEnv()
		defer e.close()
		ctx := context.Background()
		if _, err := e.dispStore.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", fixedNow); err != nil {
			log.Fatalf("seed distill: %v", err)
		}
		if _, err := e.dispStore.Create(ctx, coredisp.KindReconcile, json.RawMessage(`{}`), "", fixedNow.Add(time.Second)); err != nil {
			log.Fatalf("seed reconcile: %v", err)
		}
		s := e.step("POST", "/disposition/list_pending",
			serverdisposition.ListPendingRequest{Kinds: []string{string(coredisp.KindReconcile)}}, false)
		cases = append(cases, conformance.Case{
			Name:  "list_pending filters by kind: only the reconcile item is returned",
			Steps: []conformance.Step{s},
		})
	}()

	// list_pending with group:true collapses a shared group_id into one
	// row carrying every member; an ungrouped item stays its own row.
	func() {
		e := newEnv()
		defer e.close()
		ctx := context.Background()
		const group = "g1"
		if _, err := e.dispStore.Create(ctx, coredisp.KindDecompose, json.RawMessage(`{}`), group, fixedNow); err != nil {
			log.Fatalf("seed member 1: %v", err)
		}
		if _, err := e.dispStore.Create(ctx, coredisp.KindDecompose, json.RawMessage(`{}`), group, fixedNow.Add(time.Second)); err != nil {
			log.Fatalf("seed member 2: %v", err)
		}
		if _, err := e.dispStore.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", fixedNow.Add(2*time.Second)); err != nil {
			log.Fatalf("seed ungrouped: %v", err)
		}
		s := e.step("POST", "/disposition/list_pending", serverdisposition.ListPendingRequest{Group: true}, false)
		cases = append(cases, conformance.Case{
			Name:  "list_pending with group:true collapses items sharing a group_id into one row",
			Steps: []conformance.Step{s},
		})
	}()

	// Default view shows only pending items; parked:true shows only
	// parked items -- one case, two steps against the same seeded state
	// (mirrors TestListPendingParkedFilter).
	func() {
		e := newEnv()
		defer e.close()
		ctx := context.Background()
		if _, err := e.dispStore.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", fixedNow); err != nil {
			log.Fatalf("seed parked-to-be: %v", err)
		}
		sweepNow := fixedNow
		for i := 0; i < 3; i++ {
			sweepNow = sweepNow.Add(15 * 24 * time.Hour)
			if _, err := coredisp.Sweep(ctx, e.dispStore, nil, sweepNow); err != nil {
				log.Fatalf("Sweep %d: %v", i, err)
			}
		}
		if _, err := e.dispStore.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", sweepNow); err != nil {
			log.Fatalf("seed pending: %v", err)
		}
		s1 := e.step("POST", "/disposition/list_pending", serverdisposition.ListPendingRequest{}, false)
		s2 := e.step("POST", "/disposition/list_pending", serverdisposition.ListPendingRequest{Parked: true}, false)
		cases = append(cases, conformance.Case{
			Name:  "parked items appear only with the parked filter; the default view excludes them",
			Steps: []conformance.Step{s1, s2},
		})
	}()

	// Cursor pagination: 5 items, pages of 2, three calls to exhaustion
	// (a smaller instance of T4.4's own "cursor walk of 120 items in
	// pages of 50 terminates" acc line).
	func() {
		e := newEnv()
		defer e.close()
		ctx := context.Background()
		for i := 0; i < 5; i++ {
			if _, err := e.dispStore.Create(ctx, coredisp.KindDistill, json.RawMessage(fmt.Sprintf(`{"n":%d}`, i)), "", fixedNow.Add(time.Duration(i)*time.Second)); err != nil {
				log.Fatalf("seed item %d: %v", i, err)
			}
		}
		s1 := e.step("POST", "/disposition/list_pending", serverdisposition.ListPendingRequest{Limit: 2}, false)
		s2 := e.step("POST", "/disposition/list_pending", serverdisposition.ListPendingRequest{Limit: 2, Cursor: "2"}, false)
		s3 := e.step("POST", "/disposition/list_pending", serverdisposition.ListPendingRequest{Limit: 2, Cursor: "4"}, false)
		cases = append(cases, conformance.Case{
			Name:  "cursor pagination walks all 5 items in pages of 2 and terminates with no next_cursor",
			Steps: []conformance.Step{s1, s2, s3},
		})
	}()

	writeTranscript("list_pending", cases...)
}

func genDispose() {
	var cases []conformance.Case

	func() {
		e := newEnv()
		defer e.close()
		ctx := context.Background()
		item, err := e.dispStore.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", fixedNow)
		if err != nil {
			log.Fatalf("seed item: %v", err)
		}
		s := e.step("POST", "/disposition/dispose", serverdisposition.DisposeRequest{
			ItemID: item.ID, Verdict: string(coredisp.VerdictAccept), IdempotencyKey: "accept-key-1", Actor: "tester",
		}, false)
		cases = append(cases, conformance.Case{Name: "dispose accept records the verdict", Steps: []conformance.Step{s}})
	}()

	func() {
		e := newEnv()
		defer e.close()
		ctx := context.Background()
		item, err := e.dispStore.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", fixedNow)
		if err != nil {
			log.Fatalf("seed item: %v", err)
		}
		s := e.step("POST", "/disposition/dispose", serverdisposition.DisposeRequest{
			ItemID: item.ID, Verdict: string(coredisp.VerdictReject), IdempotencyKey: "reject-key-1",
		}, false)
		cases = append(cases, conformance.Case{
			Name:  "reject without a note returns the reject_requires_note protocol error",
			Steps: []conformance.Step{s},
		})
	}()

	func() {
		e := newEnv()
		defer e.close()
		ctx := context.Background()
		item, err := e.dispStore.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", fixedNow)
		if err != nil {
			log.Fatalf("seed item: %v", err)
		}
		s := e.step("POST", "/disposition/dispose", serverdisposition.DisposeRequest{
			ItemID: item.ID, Verdict: string(coredisp.VerdictAccept),
		}, false)
		cases = append(cases, conformance.Case{
			Name:  "dispose without idempotency_key returns invalid_request",
			Steps: []conformance.Step{s},
		})
	}()

	func() {
		e := newEnv()
		defer e.close()
		ctx := context.Background()
		item, err := e.dispStore.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", fixedNow)
		if err != nil {
			log.Fatalf("seed item: %v", err)
		}
		req := serverdisposition.DisposeRequest{
			ItemID: item.ID, Verdict: string(coredisp.VerdictAccept), IdempotencyKey: "replay-key-1", Actor: "tester",
		}
		s1 := e.step("POST", "/disposition/dispose", req, false)
		s2 := e.step("POST", "/disposition/dispose", req, false)
		cases = append(cases, conformance.Case{
			Name:  "replaying dispose with the same idempotency_key returns replayed:true, never a second write",
			Steps: []conformance.Step{s1, s2},
		})
	}()

	func() {
		e := newEnv()
		defer e.close()
		ctx := context.Background()
		const group = "g2"
		if _, err := e.dispStore.Create(ctx, coredisp.KindDecompose, json.RawMessage(`{}`), group, fixedNow); err != nil {
			log.Fatalf("seed member 1: %v", err)
		}
		if _, err := e.dispStore.Create(ctx, coredisp.KindDecompose, json.RawMessage(`{}`), group, fixedNow.Add(time.Second)); err != nil {
			log.Fatalf("seed member 2: %v", err)
		}
		s := e.step("POST", "/disposition/dispose", serverdisposition.DisposeRequest{
			GroupID: group, Verdict: string(coredisp.VerdictAccept), IdempotencyKey: "group-key-1",
		}, false)
		cases = append(cases, conformance.Case{
			Name:  "dispose by group_id disposes every member individually, one result per member",
			Steps: []conformance.Step{s},
		})
	}()

	func() {
		e := newEnv()
		defer e.close()
		s := e.step("POST", "/disposition/dispose", serverdisposition.DisposeRequest{
			GroupID: "no-such-group", Verdict: string(coredisp.VerdictAccept), IdempotencyKey: "group-key-2",
		}, false)
		cases = append(cases, conformance.Case{
			Name:  "dispose by an unknown group_id returns not_found",
			Steps: []conformance.Step{s},
		})
	}()

	writeTranscript("dispose", cases...)
}

func genCapture() {
	var cases []conformance.Case

	func() {
		e := newEnv()
		defer e.close()
		s := e.step("POST", "/disposition/capture", serverdisposition.CaptureRequest{Text: "call the landlord about the lease"}, false)
		cases = append(cases, conformance.Case{Name: "capture text stages a distill item and returns its id", Steps: []conformance.Step{s}})
	}()

	func() {
		e := newEnv()
		defer e.close()
		s := e.step("POST", "/disposition/capture", serverdisposition.CaptureRequest{}, false)
		cases = append(cases, conformance.Case{Name: "capture with no text and no audio_ref returns capture_empty", Steps: []conformance.Step{s}})
	}()

	writeTranscript("capture", cases...)
}

func genSubscribeLongpoll() {
	var cases []conformance.Case

	// capture publishes one event; the immediately-following long-poll
	// subscribe (no Accept: text/event-stream) returns it right away,
	// without waiting out the long-poll timeout.
	func() {
		e := newEnv(serverdisposition.WithLongPollWait(2 * time.Second))
		defer e.close()
		s1 := e.step("POST", "/disposition/capture", serverdisposition.CaptureRequest{Text: "note 0"}, false)
		s2 := e.step("GET", "/disposition/subscribe?cursor=0", nil, false)
		cases = append(cases, conformance.Case{
			Name:  "long-poll subscribe returns immediately once an event already exists past the cursor",
			Steps: []conformance.Step{s1, s2},
		})
	}()

	func() {
		e := newEnv()
		defer e.close()
		s := e.step("GET", "/disposition/subscribe?cursor=0", nil, true)
		cases = append(cases, conformance.Case{
			Name:  "subscribe with no bearer token returns 401 like every other route",
			Steps: []conformance.Step{s},
		})
	}()

	writeTranscript("subscribe_longpoll", cases...)
}
