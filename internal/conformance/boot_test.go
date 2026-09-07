package conformance

// This file is T4.15's own acc-line test: "go test ./internal/conformance
// boots the server and replays every transcript; a deliberately broken
// build (env flag) fails the suite."
//
// Scope, matching T4.15's declared deps (T4.13 fixtures, T4.4 disposition,
// T4.6 direction -- not T4.1/T4.20/T4.21's memory-server assembly): this
// file boots real DISPOSITION and DIRECTION listeners (the identical
// real-listener harness each protocol's own *_test.go and
// gen_transcripts.go already use) and replays every one of their frozen
// transcripts through this package's own ReplayTranscript/CompareBodies
// engine. MEMORY_VERBS' equivalent boot-and-replay-in-process coverage
// already exists as internal/server/memory's own TestMemoryV1AllPinnedCases
// (T4.20, the same cases.json this task's fixtures vendor) -- reconstructing
// memory.Deps' full provider/embedder/composer/git-brain-repo stack a
// second time here would duplicate that suite for no new signal. This
// package's own memory_verbs replay path (MCPClient/ReplayMemoryVerbCases)
// is instead proven by replay_test.go's httptest-backed round trip and is
// meant for the CLI to run against a real `serenity serve --http` process.
//
// DISPOSITION's list_pending and dispose need seeded server-side state
// their own transcript doesn't carry (testdata/conformance/README.md's
// disclosed gap: item ids are `crypto/rand`, real captured values, not
// reproducible run to run). Each subtest below re-seeds the identical
// scenario gen_transcripts.go used (same kind/group/clock sequence) and
// substitutes the freshly-seeded live id for the fixture's recorded one
// before replaying -- the recorded id strings below are copied directly
// from testdata/conformance/disposition/{list_pending,dispose}.json, not
// invented. DIRECTION needs no such substitution: its ledger entry ids
// (cst-0001, qst-0001, ...) are caller-chosen literals in
// gen_transcripts.go, not server-random, so seeding the same entries
// reproduces byte-identical responses outright.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/dira/ledger"
	coredirection "github.com/sirerun/serenity/internal/direction"
	coredisp "github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/events"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/secrets"
	"github.com/sirerun/serenity/internal/server"
	serverdirection "github.com/sirerun/serenity/internal/server/direction"
	serverdisposition "github.com/sirerun/serenity/internal/server/disposition"
	"github.com/sirerun/serenity/internal/writer"
)

// bootFixedClock mirrors gen_transcripts.go's own fakeClock -- an
// injectable Clock so seeded created_at/disposed_at timestamps are
// reproducible, satisfying serverdisposition.Clock and
// serverdirection.Clock (both `interface{ Now() time.Time }`).
type bootFixedClock struct{ now time.Time }

func (c bootFixedClock) Now() time.Time { return c.now }

var bootFixedNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

// bootServer is one fresh, isolated real HTTP listener over a real
// protocol Handlers registration -- the same env shape
// gen_transcripts.go's own disposition/direction generators use, adapted
// to close over *testing.T instead of log.Fatal.
//
// This is each package's real, sole, production registration path, not a
// parallel test-only one: newBootDispositionServer/newBootDirectionServer
// call h.Register(s) below, the exact same *serverdisposition.Handlers /
// *serverdirection.Handlers method a future `serve.go` wiring would call
// once these two protocols are mounted onto a live daemon -- both packages'
// own doc comments confirm that wiring doesn't exist in production yet
// (only `serve --http`'s MEMORY_VERBS mount does today). So this test boots
// the real thing disposition/direction ship, on a real TCP loopback listener
// (*internal/server.Server, bearer-token authenticated per RFC 0001 section
// 14), ahead of that wiring landing -- it is not simulating or mocking the
// registration Register itself performs.
type bootServer struct {
	base   string
	token  string
	client *http.Client
}

func newBootDispositionServer(t *testing.T, opts ...serverdisposition.Option) (*bootServer, *coredisp.Store) {
	t.Helper()
	secrets.MockForTesting()
	dir := t.TempDir()
	eng, err := index.Open(filepath.Join(dir, "index.db"))
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	dispStore := coredisp.NewStore(eng)
	evStore := events.NewStore(eng)
	allOpts := append([]serverdisposition.Option{serverdisposition.WithClock(bootFixedClock{bootFixedNow})}, opts...)
	h := serverdisposition.New(dispStore, evStore, allOpts...)
	return startBootServer(t, func(s *server.Server) { h.Register(s) }), dispStore
}

func newBootDirectionServer(t *testing.T) (*bootServer, *coredirection.Store) {
	t.Helper()
	secrets.MockForTesting()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".dira", "entries"), 0o755); err != nil {
		t.Fatalf("MkdirAll .dira/entries: %v", err)
	}
	q := writer.NewQueue(nil)
	t.Cleanup(q.Close)
	ledgerStore := coredirection.NewStore(root, q)

	eng, err := index.Open(filepath.Join(root, "index.db"))
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	dispStore := coredisp.NewStore(eng)

	h := serverdirection.New(ledgerStore, dispStore, root, serverdirection.WithClock(bootFixedClock{bootFixedNow}))
	return startBootServer(t, func(s *server.Server) { h.Register(s) }), ledgerStore
}

// startBootServer starts a real *server.Server (loopback, bearer-token
// authenticated, RFC 0001 §14) and hands it to register -- a closure over
// whichever protocol's *Handlers this call is standing up, so this helper
// stays agnostic to serverdisposition.Registrar vs serverdirection.Registrar
// being distinct (structurally identical but nominally different)
// interface types.
func startBootServer(t *testing.T, register func(*server.Server)) *bootServer {
	t.Helper()
	const token = "s3cr3t-conformance-boot-token"
	s := server.New(server.Config{Bind: "127.0.0.1:0", TokenSource: func() (string, error) { return token, nil }})
	register(s)
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = s.Serve(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("boot server did not shut down within 5s")
		}
	})
	return &bootServer{
		base:   "http://" + s.Addr(),
		token:  token,
		client: &http.Client{Transport: &http.Transport{DisableKeepAlives: true}},
	}
}

// caseByName finds one named Case in a loaded Transcript, failing the
// test immediately if the fixture's own case names ever drift -- this
// test must fail loudly, not silently skip, if it can no longer find the
// scenario it was written against.
func caseByName(t *testing.T, tr Transcript, name string) Case {
	t.Helper()
	for _, c := range tr.Cases {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("%s/%s: no case named %q (fixture case names changed?)", tr.Protocol, tr.Operation, name)
	return Case{}
}

// substituteCase rewrites every occurrence of each substitutions key
// (a fixture's own frozen, unreproducible value) to its paired value (the
// live value this test just seeded) across a Case's own request and
// response bodies, byte-for-byte. Safe as a plain substring replace: every
// substituted value here is an opaque hex token that cannot collide with
// JSON syntax or another field's own content.
func substituteCase(c Case, subs map[string]string) Case {
	apply := func(s string) string {
		for from, to := range subs {
			s = strings.ReplaceAll(s, from, to)
		}
		return s
	}
	out := Case{Name: c.Name, Steps: make([]Step, len(c.Steps))}
	for i, step := range c.Steps {
		if len(step.RequestBody) > 0 {
			step.RequestBody = json.RawMessage(apply(string(step.RequestBody)))
		}
		step.Response.Body = apply(step.Response.Body)
		out.Steps[i] = step
	}
	return out
}

func requireCasePassed(t *testing.T, outcome CaseOutcome) {
	t.Helper()
	if outcome.Passed() {
		return
	}
	for _, s := range outcome.Steps {
		if s.Err != nil {
			t.Errorf("%s %s: %v", s.Method, s.Path, s.Err)
			continue
		}
		if s.StatusExpected != s.StatusActual {
			t.Errorf("%s %s: status expected %d, got %d", s.Method, s.Path, s.StatusExpected, s.StatusActual)
		}
		if len(s.Mismatches) > 0 {
			t.Errorf("%s %s:\n%s", s.Method, s.Path, FormatMismatches(s.Mismatches))
		}
	}
}

func loadFixtureTranscript(t *testing.T, protocol, operation string) Transcript {
	t.Helper()
	tr, err := LoadTranscript(filepath.Join(DefaultFixturesDir(), protocol, operation+".json"))
	if err != nil {
		t.Fatalf("LoadTranscript %s/%s: %v", protocol, operation, err)
	}
	return tr
}

// --- DISPOSITION: list_pending and dispose (need seeding + substitution) ---

func TestBootDispositionReplaysListPending(t *testing.T) {
	tr := loadFixtureTranscript(t, "disposition", "list_pending")
	ctx := context.Background()

	t.Run("list_pending filters by kind: only the reconcile item is returned", func(t *testing.T) {
		srv, store := newBootDispositionServer(t)
		if _, err := store.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", bootFixedNow); err != nil {
			t.Fatalf("seed distill: %v", err)
		}
		reconcile, err := store.Create(ctx, coredisp.KindReconcile, json.RawMessage(`{}`), "", bootFixedNow.Add(time.Second))
		if err != nil {
			t.Fatalf("seed reconcile: %v", err)
		}
		c := substituteCase(caseByName(t, tr, "list_pending filters by kind: only the reconcile item is returned"),
			map[string]string{"ebd72d47d6cb7145b1f581425deb94f0": reconcile.ID})
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token, c))
	})

	t.Run("list_pending with group:true collapses items sharing a group_id into one row", func(t *testing.T) {
		srv, store := newBootDispositionServer(t)
		const group = "g1"
		m1, err := store.Create(ctx, coredisp.KindDecompose, json.RawMessage(`{}`), group, bootFixedNow)
		if err != nil {
			t.Fatalf("seed member 1: %v", err)
		}
		m2, err := store.Create(ctx, coredisp.KindDecompose, json.RawMessage(`{}`), group, bootFixedNow.Add(time.Second))
		if err != nil {
			t.Fatalf("seed member 2: %v", err)
		}
		ungrouped, err := store.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", bootFixedNow.Add(2*time.Second))
		if err != nil {
			t.Fatalf("seed ungrouped: %v", err)
		}
		c := substituteCase(caseByName(t, tr, "list_pending with group:true collapses items sharing a group_id into one row"),
			map[string]string{
				"8c7be57a3e70e66072de44faa97e112e": m1.ID,
				"da88f8f13ad0bde81615619a019a88bf": m2.ID,
				"9cfd8aa2ea89f87c73cc22822551d0d5": ungrouped.ID,
			})
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token, c))
	})

	t.Run("parked items appear only with the parked filter; the default view excludes them", func(t *testing.T) {
		srv, store := newBootDispositionServer(t)
		toBePark, err := store.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", bootFixedNow)
		if err != nil {
			t.Fatalf("seed parked-to-be: %v", err)
		}
		sweepNow := bootFixedNow
		for i := 0; i < 3; i++ {
			sweepNow = sweepNow.Add(15 * 24 * time.Hour)
			if _, err := coredisp.Sweep(ctx, store, nil, sweepNow); err != nil {
				t.Fatalf("Sweep %d: %v", i, err)
			}
		}
		pending, err := store.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", sweepNow)
		if err != nil {
			t.Fatalf("seed pending: %v", err)
		}
		c := substituteCase(caseByName(t, tr, "parked items appear only with the parked filter; the default view excludes them"),
			map[string]string{
				"e2d0059ec88559a717f4011b0b1ec344": pending.ID,
				"876e169deb3801fb3fd96299f27c008c": toBePark.ID,
			})
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token, c))
	})

	t.Run("cursor pagination walks all 5 items in pages of 2 and terminates with no next_cursor", func(t *testing.T) {
		srv, store := newBootDispositionServer(t)
		recordedIDs := []string{
			"6cd3f245564041105b3849dc82287d14",
			"4b897ff110f07533d546403b0ca59d5b",
			"72f05146034026824af83535349619b0",
			"b97a22d4d24c64e499f2b42b03da597a",
			"7b752da0af4db42b23986e61840d27ee",
		}
		subs := map[string]string{}
		for i, recorded := range recordedIDs {
			item, err := store.Create(ctx, coredisp.KindDistill, json.RawMessage(fmt.Sprintf(`{"n":%d}`, i)), "", bootFixedNow.Add(time.Duration(i)*time.Second))
			if err != nil {
				t.Fatalf("seed item %d: %v", i, err)
			}
			subs[recorded] = item.ID
		}
		c := substituteCase(caseByName(t, tr, "cursor pagination walks all 5 items in pages of 2 and terminates with no next_cursor"), subs)
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token, c))
	})
}

func TestBootDispositionReplaysDispose(t *testing.T) {
	tr := loadFixtureTranscript(t, "disposition", "dispose")
	ctx := context.Background()

	t.Run("dispose accept records the verdict", func(t *testing.T) {
		srv, store := newBootDispositionServer(t)
		item, err := store.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", bootFixedNow)
		if err != nil {
			t.Fatalf("seed item: %v", err)
		}
		c := substituteCase(caseByName(t, tr, "dispose accept records the verdict"),
			map[string]string{"cda08aff5a5749c7f6ad9b50d90554ac": item.ID})
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token, c))
	})

	t.Run("reject without a note returns the reject_requires_note protocol error", func(t *testing.T) {
		srv, store := newBootDispositionServer(t)
		item, err := store.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", bootFixedNow)
		if err != nil {
			t.Fatalf("seed item: %v", err)
		}
		c := substituteCase(caseByName(t, tr, "reject without a note returns the reject_requires_note protocol error"),
			map[string]string{"0a13575759c2272ce8f7959b6b7f215e": item.ID})
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token, c))
	})

	t.Run("dispose without idempotency_key returns invalid_request", func(t *testing.T) {
		srv, store := newBootDispositionServer(t)
		item, err := store.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", bootFixedNow)
		if err != nil {
			t.Fatalf("seed item: %v", err)
		}
		c := substituteCase(caseByName(t, tr, "dispose without idempotency_key returns invalid_request"),
			map[string]string{"46f2b88c3b64599c78f7a0666d6c5e63": item.ID})
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token, c))
	})

	t.Run("replaying dispose with the same idempotency_key returns replayed:true, never a second write", func(t *testing.T) {
		srv, store := newBootDispositionServer(t)
		item, err := store.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", bootFixedNow)
		if err != nil {
			t.Fatalf("seed item: %v", err)
		}
		c := substituteCase(caseByName(t, tr, "replaying dispose with the same idempotency_key returns replayed:true, never a second write"),
			map[string]string{"d0d31506c9a304c9e3c81664795dc521": item.ID})
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token, c))
	})

	t.Run("dispose by group_id disposes every member individually, one result per member", func(t *testing.T) {
		srv, store := newBootDispositionServer(t)
		const group = "g2"
		m1, err := store.Create(ctx, coredisp.KindDecompose, json.RawMessage(`{}`), group, bootFixedNow)
		if err != nil {
			t.Fatalf("seed member 1: %v", err)
		}
		m2, err := store.Create(ctx, coredisp.KindDecompose, json.RawMessage(`{}`), group, bootFixedNow.Add(time.Second))
		if err != nil {
			t.Fatalf("seed member 2: %v", err)
		}
		c := substituteCase(caseByName(t, tr, "dispose by group_id disposes every member individually, one result per member"),
			map[string]string{
				"bf87faff5713ba6f5fe5e9b624356cbb": m1.ID,
				"c07701dc57cb9777ab687f8942c10c09": m2.ID,
			})
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token, c))
	})

	t.Run("dispose by an unknown group_id returns not_found", func(t *testing.T) {
		srv, _ := newBootDispositionServer(t)
		c := caseByName(t, tr, "dispose by an unknown group_id returns not_found") // no pre-existing state needed
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token, c))
	})
}

// --- DISPOSITION: capture and subscribe_longpoll (self-contained: every
// id they reference is created by the transcript's own first step, so no
// seeding or substitution is needed -- only CompareBodies' normalization). ---

func TestBootDispositionReplaysCaptureAndSubscribeLongpoll(t *testing.T) {
	ctx := context.Background()

	t.Run("capture", func(t *testing.T) {
		srv, _ := newBootDispositionServer(t)
		out := ReplayTranscript(ctx, srv.client, srv.base, srv.token, loadFixtureTranscript(t, "disposition", "capture"))
		for _, c := range out.Cases {
			requireCasePassed(t, c)
		}
	})

	t.Run("subscribe_longpoll", func(t *testing.T) {
		srv, _ := newBootDispositionServer(t, serverdisposition.WithLongPollWait(2*time.Second))
		out := ReplayTranscript(ctx, srv.client, srv.base, srv.token, loadFixtureTranscript(t, "disposition", "subscribe_longpoll"))
		for _, c := range out.Cases {
			requireCasePassed(t, c)
		}
	})
}

// --- DIRECTION: brief, check_plan, propose (ledger entry ids are
// caller-chosen literals, not server-random -- seeding is enough, no
// substitution needed; propose's own dynamic disposition item_id is
// handled by CompareBodies' normalization, the same as capture's). ---

func seedBootConstraint(s *coredirection.Store, id, title, action, paramsYAML, whyNot, revisitIf string) error {
	body := "Fixture constraint.\n\n```serenity:applies_when\naction: " + action + "\n"
	if paramsYAML != "" {
		body += "params: " + paramsYAML + "\n"
	}
	body += "```\n"
	entry := &ledger.Entry{
		ID: id, Kind: ledger.KindConstraint, Title: title, State: ledger.StateActive,
		Created:      "2026-08-28T00:00:00Z",
		Alternatives: []ledger.Alternative{{Option: "no ceiling", WhyNot: whyNot, RevisitIf: revisitIf}},
		Body:         body,
	}
	return s.Create(context.Background(), entry)
}

func seedBootQuestion(s *coredirection.Store, id, title string) error {
	entry := &ledger.Entry{ID: id, Kind: ledger.KindQuestion, Title: title, State: ledger.StateOpen, Created: "2026-08-28T00:00:00Z"}
	return s.Create(context.Background(), entry)
}

func TestBootDirectionReplaysBrief(t *testing.T) {
	tr := loadFixtureTranscript(t, "direction", "brief")
	ctx := context.Background()

	t.Run("brief on a fresh ledger returns all four fixed sections, empty", func(t *testing.T) {
		srv, _ := newBootDirectionServer(t)
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token,
			caseByName(t, tr, "brief on a fresh ledger returns all four fixed sections, empty")))
	})

	t.Run("a zero token_budget returns the minimal valid object: four sections, real candidates all omitted", func(t *testing.T) {
		srv, store := newBootDirectionServer(t)
		if err := seedBootConstraint(store, "cst-0001", "fixture spend ceiling", "spend_over", "{amount: {gte: 200}}",
			"unbounded spend risk", "quarterly review"); err != nil {
			t.Fatalf("seedBootConstraint: %v", err)
		}
		if err := seedBootQuestion(store, "qst-0001", "should we raise the ceiling?"); err != nil {
			t.Fatalf("seedBootQuestion: %v", err)
		}
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token,
			caseByName(t, tr, "a zero token_budget returns the minimal valid object: four sections, real candidates all omitted")))
	})

	t.Run("budget 800 fills sections by priority and drops an overflowing precepts section whole, never starving questions", func(t *testing.T) {
		srv, store := newBootDirectionServer(t)
		const preceptCap = 12
		longWord := "supercalifragilisticexpialidocious "
		var longWhyNot strings.Builder
		for i := 0; i < 70; i++ {
			longWhyNot.WriteString(longWord)
		}
		for i := 1; i <= preceptCap; i++ {
			id := fmt.Sprintf("cst-%04d", i)
			if err := seedBootConstraint(store, id, fmt.Sprintf("fixture precept number %d", i), "spend_over", "{amount: {gte: 200}}",
				longWhyNot.String(), "quarterly review"); err != nil {
				t.Fatalf("seedBootConstraint %s: %v", id, err)
			}
		}
		if err := seedBootQuestion(store, "qst-0001", "should we raise the ceiling?"); err != nil {
			t.Fatalf("seedBootQuestion qst-0001: %v", err)
		}
		if err := seedBootQuestion(store, "qst-0002", "who owns this precept?"); err != nil {
			t.Fatalf("seedBootQuestion qst-0002: %v", err)
		}
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token,
			caseByName(t, tr, "budget 800 fills sections by priority and drops an overflowing precepts section whole, never starving questions")))
	})
}

func TestBootDirectionReplaysCheckPlan(t *testing.T) {
	tr := loadFixtureTranscript(t, "direction", "check_plan")
	ctx := context.Background()

	t.Run("check_plan with neither plan_text nor actions returns invalid_request", func(t *testing.T) {
		srv, _ := newBootDirectionServer(t)
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token,
			caseByName(t, tr, "check_plan with neither plan_text nor actions returns invalid_request")))
	})

	t.Run("check_plan with a structured action matching an active constraint returns status violated", func(t *testing.T) {
		srv, store := newBootDirectionServer(t)
		if err := seedBootConstraint(store, "cst-0001", "fixture spend ceiling", "spend_over", "{amount: {gte: 200}}",
			`Unbounded spend risk: "no ceiling" was rejected outright.`, "quarterly budget review"); err != nil {
			t.Fatalf("seedBootConstraint: %v", err)
		}
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token,
			caseByName(t, tr, "check_plan with a structured action matching an active constraint returns status violated")))
	})

	t.Run("check_plan matching zero active constraints returns no_applicable_constraints, never a bare pass", func(t *testing.T) {
		srv, _ := newBootDirectionServer(t)
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token,
			caseByName(t, tr, "check_plan matching zero active constraints returns no_applicable_constraints, never a bare pass")))
	})

	t.Run("check_plan free text with no classifier router configured returns status unverified, never a silent pass", func(t *testing.T) {
		srv, _ := newBootDirectionServer(t)
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token,
			caseByName(t, tr, "check_plan free text with no classifier router configured returns status unverified, never a silent pass")))
	})
}

func TestBootDirectionReplaysPropose(t *testing.T) {
	tr := loadFixtureTranscript(t, "direction", "propose")
	ctx := context.Background()

	t.Run("propose with an unsupported kind returns invalid_kind", func(t *testing.T) {
		srv, _ := newBootDirectionServer(t)
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token,
			caseByName(t, tr, "propose with an unsupported kind returns invalid_kind")))
	})

	t.Run("propose precept_draft with no alternatives is rejected before anything is staged", func(t *testing.T) {
		srv, _ := newBootDirectionServer(t)
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token,
			caseByName(t, tr, "propose precept_draft with no alternatives is rejected before anything is staged")))
	})

	t.Run("propose precept_draft with a title and one alternative creates a pending disposition item", func(t *testing.T) {
		srv, store := newBootDirectionServer(t)
		if err := seedBootConstraint(store, "cst-0001", "fixture spend ceiling", "spend_over", "{amount: {gte: 200}}",
			"unbounded spend risk", "quarterly review"); err != nil {
			t.Fatalf("seedBootConstraint: %v", err)
		}
		// The item_id this case's response carries is a fresh
		// internal/disposition random id (propose stages into the
		// DISPOSITION queue) -- CompareBodies' own shape-based
		// normalization excuses it, the same as capture's item_id.
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token,
			caseByName(t, tr, "propose precept_draft with a title and one alternative creates a pending disposition item (.dira/ is never touched by propose -- see internal/server/direction's own doc comment and test)")))
	})

	t.Run("propose effect creates a pending disposition item", func(t *testing.T) {
		srv, _ := newBootDirectionServer(t)
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token,
			caseByName(t, tr, "propose effect creates a pending disposition item")))
	})

	t.Run("propose with no payload returns invalid_payload", func(t *testing.T) {
		srv, _ := newBootDirectionServer(t)
		requireCasePassed(t, replayCase(ctx, srv.client, srv.base, srv.token,
			caseByName(t, tr, "propose with no payload returns invalid_payload")))
	})
}

// --- Anti-vacuous-green: a deliberately broken build must fail this
// suite (same discipline as manifest_test.go's own
// TestVerifyManifestDetectsTamper). Gated by an env var this test file
// alone reads -- never a code path production serve.go can reach -- so
// there is no way to accidentally ship a build with conformance checking
// disabled. ---

// breakingRoundTripper corrupts one disposition dispose response's
// verdict field in flight when SERENITY_CONFORMANCE_BREAK_TEST is set,
// standing in for "a deliberately broken build": a live server that
// silently returns the wrong verdict. It never touches any other
// response, so the rest of this suite's expectations still hold even
// while it's active.
type breakingRoundTripper struct{ inner http.RoundTripper }

func (b breakingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := b.inner.RoundTrip(req)
	if err != nil || req.URL.Path != "/disposition/dispose" {
		return resp, err
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return resp, err
	}
	corrupted := strings.Replace(string(body), `"verdict":"accept"`, `"verdict":"rejected-by-a-deliberately-broken-build"`, 1)
	resp.Body = io.NopCloser(strings.NewReader(corrupted))
	resp.ContentLength = int64(len(corrupted))
	return resp, nil
}

// TestBootDeliberatelyBrokenBuildFailsDisposeConformance is the acc line's
// second clause, literally: "a deliberately broken build (env flag) fails
// the suite." It is opt-in, not run by `go test ./internal/conformance`
// in CI by default (that must stay green) -- set
// SERENITY_CONFORMANCE_BREAK_TEST=1 to run it, the same way a developer
// proves a checksum/tamper detector isn't vacuous by actually tampering
// (see manifest_test.go's own TestVerifyManifestDetectsTamper for the
// unconditional version of this discipline; this one is conditional only
// because IT is meant to fail on purpose, which an always-on version
// could never do without breaking ordinary CI runs). With the flag set,
// this test corrupts a real dispose response in flight and then fails
// itself, deliberately and loudly, if -- and only if -- ReplayTranscript
// correctly caught the corruption; the harness having a hole would be
// the one way for this test to pass while the flag is set, and that
// path fails too, with a distinguishing message.
func TestBootDeliberatelyBrokenBuildFailsDisposeConformance(t *testing.T) {
	if os.Getenv("SERENITY_CONFORMANCE_BREAK_TEST") == "" {
		t.Skip("set SERENITY_CONFORMANCE_BREAK_TEST=1 to run this deliberately-broken-build self-check")
	}
	tr := loadFixtureTranscript(t, "disposition", "dispose")
	ctx := context.Background()

	srv, store := newBootDispositionServer(t)
	item, err := store.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", bootFixedNow)
	if err != nil {
		t.Fatalf("seed item: %v", err)
	}
	c := substituteCase(caseByName(t, tr, "dispose accept records the verdict"),
		map[string]string{"cda08aff5a5749c7f6ad9b50d90554ac": item.ID})

	brokenClient := &http.Client{Transport: breakingRoundTripper{inner: srv.client.Transport}}
	outcome := replayCase(ctx, brokenClient, srv.base, srv.token, c)
	if outcome.Passed() {
		t.Fatal("SERENITY_CONFORMANCE_BREAK_TEST was set but the corrupted verdict still passed conformance -- " +
			"the replay harness has a hole, this is a bug in CompareBodies/ReplayTranscript, not the expected chaos-test failure")
	}
	if len(outcome.Steps) == 0 || len(outcome.Steps[0].Mismatches) == 0 {
		t.Fatal("expected the corruption to surface as a reported mismatch")
	}
	t.Fatalf("deliberately broken build correctly detected and failed conformance (as designed): %s",
		FormatMismatches(outcome.Steps[0].Mismatches))
}
