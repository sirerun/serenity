package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/operation"
	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/hosted/store"
	"github.com/sirerun/serenity/internal/writer"
)

type reviewAdmissionFunc func(context.Context, *http.Request, string, string) (ApprovedOperatorReview, error)

func (f reviewAdmissionFunc) Authorize(ctx context.Context, r *http.Request, operationID, reviewRef string) (ApprovedOperatorReview, error) {
	return f(ctx, r, operationID, reviewRef)
}

func TestParseOperatorReviewRequestStrict(t *testing.T) {
	valid := `{"operation_id":"op-1","review_ref":"case-1"}`
	for _, tc := range []struct {
		name string
		body string
		ok   bool
	}{
		{name: "valid", body: valid, ok: true},
		{name: "duplicate", body: `{"operation_id":"op-1","operation_id":"op-2","review_ref":"case-1"}`},
		{name: "case alias", body: `{"Operation_ID":"op-1","review_ref":"case-1"}`},
		{name: "extra field", body: `{"operation_id":"op-1","review_ref":"case-1","outcome":"released"}`},
		{name: "number value", body: `{"operation_id":1,"review_ref":"case-1"}`},
		{name: "nested value", body: `{"operation_id":{},"review_ref":"case-1"}`},
		{name: "trailing document", body: valid + ` {}`},
		{name: "trailing scalar", body: valid + ` false`},
		{name: "missing field", body: `{"operation_id":"op-1"}`},
		{name: "whitespace identifier", body: `{"operation_id":"op 1","review_ref":"case-1"}`},
		{name: "control identifier", body: `{"operation_id":"op\u0001","review_ref":"case-1"}`},
		{name: "oversize", body: `{"operation_id":"` + strings.Repeat("x", operatorReviewRequestLimit) + `","review_ref":"case-1"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/operations/resolve", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			got, err := parseOperatorReviewRequest(w, r)
			if (err == nil) != tc.ok {
				t.Fatalf("parse result = %+v, %v; want success=%v", got, err, tc.ok)
			}
			if tc.ok && (got.operationID != "op-1" || got.reviewRef != "case-1") {
				t.Fatalf("parsed request %+v", got)
			}
		})
	}
}

type reviewRecordLedger struct {
	mu          sync.Mutex
	row         contracts.OperationRecord
	lookups     int
	resolutions int
	resolveHook func()
}

func (l *reviewRecordLedger) Lookup(_ context.Context, id string) (contracts.OperationRecord, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lookups++
	if id != l.row.ID {
		return contracts.OperationRecord{}, contracts.ErrOperationNotFound
	}
	return l.row, nil
}

func (l *reviewRecordLedger) ResolvePendingReview(_ context.Context, id string, outcome contracts.OperationPhase, ev contracts.Evidence) (contracts.OperationRecord, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.resolutions++
	if l.resolveHook != nil {
		l.resolveHook()
	}
	if l.row.ID != id {
		return contracts.OperationRecord{}, contracts.ErrOperationNotFound
	}
	if l.row.Phase == contracts.OperationCommitted {
		if l.row.Evidence.Kind == ev.Kind && l.row.Evidence.Ref == ev.Ref {
			return l.row, nil
		}
		return contracts.OperationRecord{}, contracts.ErrOperationConflict
	}
	if l.row.Phase != contracts.OperationPendingReview || outcome != contracts.OperationCommitted {
		return contracts.OperationRecord{}, contracts.ErrOperationConflict
	}
	l.row.Phase = outcome
	l.row.Evidence = ev
	return l.row, nil
}

type reviewFence struct {
	mu       sync.Mutex
	active   bool
	check    func(context.Context, contracts.OperationRecord) (contracts.CanonicalVerdict, error)
	checks   int
	releases int
}

func (f *reviewFence) Fence(_ context.Context, _ string) (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active = true
	var once sync.Once
	return func() {
		once.Do(func() {
			f.mu.Lock()
			f.active = false
			f.releases++
			f.mu.Unlock()
		})
	}, nil
}

func (f *reviewFence) Check(ctx context.Context, rec contracts.OperationRecord) (contracts.CanonicalVerdict, error) {
	f.mu.Lock()
	active := f.active
	f.checks++
	fn := f.check
	f.mu.Unlock()
	if !active {
		return contracts.CanonicalVerdict{}, errors.New("check without fence")
	}
	return fn(ctx, rec)
}

func TestAdminOperatorReviewAdmissionBeforeLookupAndNoAdapter(t *testing.T) {
	body := `{"operation_id":"op-1","review_ref":"case-1"}`
	var typedNil *reviewAdmissionFunc
	for _, tc := range []struct {
		name      string
		admission OperatorReviewAdmission
		want      int
	}{
		{name: "missing", want: http.StatusServiceUnavailable},
		{name: "typed nil", admission: typedNil, want: http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{operatorReviewAdmission: tc.admission}
			w := httptest.NewRecorder()
			s.AdminHandler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/operations/resolve", strings.NewReader(body)))
			if w.Code != tc.want {
				t.Fatalf("status=%d want%d", w.Code, tc.want)
			}
		})
	}

	lookupCalls := 0
	ledger := &reviewRecordLedger{row: contracts.OperationRecord{ID: "op-1", BrainID: "brain-1", Phase: contracts.OperationPendingReview}}
	ledger.resolveHook = func() { t.Fatal("unauthorized case reached transition") }
	fence := &reviewFence{}
	s := &Service{
		operatorReviewAdmission: reviewAdmissionFunc(func(context.Context, *http.Request, string, string) (ApprovedOperatorReview, error) {
			return ApprovedOperatorReview{}, errors.New("private admission failure")
		}),
		operatorReviewLedger: operatorReviewLedgerFunc(func(ctx context.Context, id string) (contracts.OperationRecord, error) {
			lookupCalls++
			return ledger.Lookup(ctx, id)
		}),
		operatorReviewFence: fence,
	}
	w := httptest.NewRecorder()
	s.AdminHandler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/operations/resolve", strings.NewReader(body)))
	if w.Code != http.StatusForbidden || lookupCalls != 0 || fence.checks != 0 || fence.releases != 0 {
		t.Fatalf("denied status=%d lookups=%d checks=%d releases=%d", w.Code, lookupCalls, fence.checks, fence.releases)
	}
}

type operatorReviewLedgerFunc func(context.Context, string) (contracts.OperationRecord, error)

func (f operatorReviewLedgerFunc) Lookup(ctx context.Context, id string) (contracts.OperationRecord, error) {
	return f(ctx, id)
}

func (f operatorReviewLedgerFunc) ResolvePendingReview(context.Context, string, contracts.OperationPhase, contracts.Evidence) (contracts.OperationRecord, error) {
	return contracts.OperationRecord{}, errors.New("unexpected resolution")
}

func TestAdminOperatorReviewHoldsFenceThroughCheckAndTransition(t *testing.T) {
	approved := ApprovedOperatorReview{OperationID: "op-1", ReviewRef: "case-1", OperatorID: "operator-7", Outcome: contracts.OperationCommitted, ExpectedCanonicalRef: "landed-ref-1", ApprovedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}
	var order []string
	ledger := &reviewRecordLedger{row: contracts.OperationRecord{ID: approved.OperationID, AccountID: "account-1", BrainID: "brain-1", Phase: contracts.OperationPendingReview}}
	fence := &reviewFence{check: func(context.Context, contracts.OperationRecord) (contracts.CanonicalVerdict, error) {
		order = append(order, "check")
		return contracts.CanonicalVerdict{Outcome: contracts.CanonicalLanded, Ref: approved.ExpectedCanonicalRef}, nil
	}}
	ledger.resolveHook = func() {
		fence.mu.Lock()
		defer fence.mu.Unlock()
		if !fence.active {
			t.Error("ledger transition ran after releasing runtime fence")
		}
		order = append(order, "transition")
	}
	s := &Service{
		operatorReviewAdmission: reviewAdmissionFunc(func(_ context.Context, _ *http.Request, id, ref string) (ApprovedOperatorReview, error) {
			order = append(order, "authorize")
			if id != approved.OperationID || ref != approved.ReviewRef {
				t.Fatalf("authorization requested for %q/%q", id, ref)
			}
			return approved, nil
		}),
		operatorReviewLedger: ledger,
		operatorReviewFence:  fence,
	}
	request := httptest.NewRequest(http.MethodPost, "/operations/resolve", strings.NewReader(`{"operation_id":"op-1","review_ref":"case-1"}`))
	w := httptest.NewRecorder()
	s.AdminHandler().ServeHTTP(w, request)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
	if got := strings.Join(order, ","); got != "authorize,check,transition" {
		t.Fatalf("event order %q", got)
	}
	if ledger.row.Phase != contracts.OperationCommitted || ledger.row.Evidence != (contracts.Evidence{Kind: contracts.EvidenceOperatorReview, Ref: approved.ReviewRef}) {
		t.Fatalf("resolved row %+v", ledger.row)
	}
	if fence.active || fence.releases != 1 || fence.checks != 1 {
		t.Fatalf("fence state active=%v releases=%d checks=%d", fence.active, fence.releases, fence.checks)
	}
	// Same immutable case replay is idempotent and does not check or transition again.
	w = httptest.NewRecorder()
	s.AdminHandler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/operations/resolve", strings.NewReader(`{"operation_id":"op-1","review_ref":"case-1"}`)))
	if w.Code != http.StatusNoContent || fence.checks != 1 || ledger.resolutions != 1 {
		t.Fatalf("replay status=%d checks=%d transitions=%d", w.Code, fence.checks, ledger.resolutions)
	}
}

func TestAdminOperatorReviewCancellationDuringProofKeepsPendingReview(t *testing.T) {
	approved := ApprovedOperatorReview{OperationID: "op-cancel", ReviewRef: "case-cancel", OperatorID: "operator-7", Outcome: contracts.OperationCommitted, ExpectedCanonicalRef: "landed-ref", ApprovedAt: time.Now()}
	ledger := &reviewRecordLedger{row: contracts.OperationRecord{ID: approved.OperationID, AccountID: "account-1", BrainID: "brain-1", Phase: contracts.OperationPendingReview}}
	entered := make(chan struct{})
	fence := &reviewFence{check: func(ctx context.Context, _ contracts.OperationRecord) (contracts.CanonicalVerdict, error) {
		close(entered)
		<-ctx.Done()
		return contracts.CanonicalVerdict{}, ctx.Err()
	}}
	s := &Service{
		operatorReviewAdmission: reviewAdmissionFunc(func(context.Context, *http.Request, string, string) (ApprovedOperatorReview, error) {
			return approved, nil
		}),
		operatorReviewLedger: ledger,
		operatorReviewFence:  fence,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodPost, "/operations/resolve", strings.NewReader(`{"operation_id":"op-cancel","review_ref":"case-cancel"}`)).WithContext(ctx)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		s.AdminHandler().ServeHTTP(w, request)
		close(done)
	}()
	<-entered
	cancel()
	<-done
	if w.Code != http.StatusServiceUnavailable || ledger.resolutions != 0 || ledger.row.Phase != contracts.OperationPendingReview {
		t.Fatalf("cancel status=%d resolutions=%d row=%+v", w.Code, ledger.resolutions, ledger.row)
	}
	if fence.active || fence.releases != 1 {
		t.Fatalf("canceled proof leaked fence: active=%v releases=%d", fence.active, fence.releases)
	}
}

func TestAdminOperatorReviewRejectsCaseAndProofMismatch(t *testing.T) {
	for _, tc := range []struct {
		name    string
		caseFn  func() ApprovedOperatorReview
		verdict contracts.CanonicalVerdict
	}{
		{name: "case binding", caseFn: func() ApprovedOperatorReview {
			return ApprovedOperatorReview{OperationID: "other", ReviewRef: "case-1", OperatorID: "operator-7", Outcome: contracts.OperationCommitted, ExpectedCanonicalRef: "landed-ref-1", ApprovedAt: time.Now()}
		}},
		{name: "released outcome", caseFn: func() ApprovedOperatorReview {
			return ApprovedOperatorReview{OperationID: "op-1", ReviewRef: "case-1", OperatorID: "operator-7", Outcome: contracts.OperationReleased, ExpectedCanonicalRef: "landed-ref-1", ApprovedAt: time.Now()}
		}},
		{name: "proof mismatch", caseFn: func() ApprovedOperatorReview {
			return ApprovedOperatorReview{OperationID: "op-1", ReviewRef: "case-1", OperatorID: "operator-7", Outcome: contracts.OperationCommitted, ExpectedCanonicalRef: "expected", ApprovedAt: time.Now()}
		}, verdict: contracts.CanonicalVerdict{Outcome: contracts.CanonicalLanded, Ref: "other"}},
		{name: "unknown", caseFn: func() ApprovedOperatorReview {
			return ApprovedOperatorReview{OperationID: "op-1", ReviewRef: "case-1", OperatorID: "operator-7", Outcome: contracts.OperationCommitted, ExpectedCanonicalRef: "expected", ApprovedAt: time.Now()}
		}, verdict: contracts.CanonicalVerdict{Outcome: contracts.CanonicalUnknown, Ref: "expected"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ledger := &reviewRecordLedger{row: contracts.OperationRecord{ID: "op-1", AccountID: "account-1", BrainID: "brain-1", Phase: contracts.OperationPendingReview}}
			fence := &reviewFence{check: func(context.Context, contracts.OperationRecord) (contracts.CanonicalVerdict, error) {
				return tc.verdict, nil
			}}
			s := &Service{operatorReviewAdmission: reviewAdmissionFunc(func(context.Context, *http.Request, string, string) (ApprovedOperatorReview, error) {
				return tc.caseFn(), nil
			}), operatorReviewLedger: ledger, operatorReviewFence: fence}
			w := httptest.NewRecorder()
			s.AdminHandler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/operations/resolve", strings.NewReader(`{"operation_id":"op-1","review_ref":"case-1"}`)))
			if w.Code < 400 || ledger.resolutions != 0 || ledger.row.Phase != contracts.OperationPendingReview {
				t.Fatalf("status=%d row=%+v resolutions=%d", w.Code, ledger.row, ledger.resolutions)
			}
		})
	}
}

func TestAdminOperatorReviewCommitsOnlyMatchingLandedProofOnce(t *testing.T) {
	db, cfg, account, brainPath := deletionFixture(t)
	brainID := filepath.Base(brainPath)
	ledger := &operation.Ledger{Store: db, Clock: func() time.Time { return time.Unix(100, 0).UTC() }}
	rec, err := ledger.Reserve(context.Background(), contracts.ReserveRequest{
		AccountID: account, BrainID: brainID, ClientKey: "operator-review", Fingerprint: "review-proof", QuotaPeriod: "1970-01", Source: "gateway.remember", LeaseFor: time.Second,
		Deltas: []contracts.ReserveDelta{{Metric: "writes", Units: 1, Limit: 2}, {Metric: "input_tokens", Units: 7, Limit: 20}},
	})
	if err != nil {
		t.Fatal(err)
	}
	oldPool, err := pool.New(pool.Config{MaxOpen: 2, MaxInFlight: 4, BrainsRoot: filepath.Dir(brainPath), Embedder: deletionEmbedding{}})
	if err != nil {
		t.Fatal(err)
	}
	runtime, release, err := oldPool.Acquire(context.Background(), brainID)
	if err != nil {
		_ = oldPool.Close()
		t.Fatal(err)
	}
	canonicalCtx := writer.WithCanonicalOperation(context.Background(), writer.CanonicalOperation{ID: rec.ID, BeforeCommit: func(ctx context.Context, _ string) error {
		_, err := ledger.EnterCanonical(ctx, rec.ID)
		return err
	}})
	args, err := json.Marshal(map[string]string{"operation_key": rec.ID, "fact": "The operator recovery marker is silver cedar", "provenance": "human-reviewed proof fixture", "kind": "fact", "visibility": "world"})
	if err != nil {
		release()
		_ = oldPool.Close()
		t.Fatal(err)
	}
	wrote := false
	for _, tool := range runtime.Tools {
		if tool.Name == "remember" {
			result, writeErr := tool.Handler(canonicalCtx, args)
			if writeErr != nil || result.IsError {
				release()
				_ = oldPool.Close()
				t.Fatalf("actual canonical write: %+v %v", result, writeErr)
			}
			wrote = true
			break
		}
	}
	if !wrote {
		release()
		_ = oldPool.Close()
		t.Fatal("remember tool missing")
	}
	release()
	if err = oldPool.Close(); err != nil {
		t.Fatal(err)
	}
	checkPool, err := pool.New(pool.Config{MaxOpen: 2, MaxInFlight: 4, BrainsRoot: filepath.Dir(brainPath), Embedder: deletionEmbedding{}})
	if err != nil {
		t.Fatal(err)
	}
	checkRuntime, checkRelease, err := checkPool.Acquire(context.Background(), brainID)
	if err != nil {
		_ = checkPool.Close()
		t.Fatal(err)
	}
	entered, err := ledger.Lookup(context.Background(), rec.ID)
	if err != nil {
		checkRelease()
		_ = checkPool.Close()
		t.Fatal(err)
	}
	verdict, err := checkRuntime.Check(context.Background(), entered)
	checkRelease()
	if closeErr := checkPool.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil || verdict.Outcome != contracts.CanonicalLanded || !strings.HasPrefix(verdict.Ref, "fact:") {
		t.Fatalf("landed fixture evidence = %+v, %v", verdict, err)
	}
	if _, err = ledger.Finalize(context.Background(), rec.ID, contracts.OperationPendingReview, contracts.Evidence{Kind: contracts.EvidenceUnknown, Ref: "human-review-required"}); err != nil {
		t.Fatalf("hold canonical landed operation for review: %v", err)
	}
	approved := ApprovedOperatorReview{OperationID: rec.ID, ReviewRef: "case-2026-10-01-1", OperatorID: "operator-7", Outcome: contracts.OperationCommitted, ExpectedCanonicalRef: verdict.Ref, ApprovedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}
	deps := testLifecycleDependencies(t)
	deps.OperatorReviewAdmission = reviewAdmissionFunc(func(_ context.Context, _ *http.Request, id, ref string) (ApprovedOperatorReview, error) {
		if id != approved.OperationID || ref != approved.ReviewRef {
			return ApprovedOperatorReview{}, errors.New("case not found")
		}
		return approved, nil
	})
	svc, err := AssembleWithDependencies(context.Background(), cfg, true, db, nil, deletionEmbedding{}, deps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	_, warmRelease, err := svc.Pool.Acquire(context.Background(), brainID)
	if err != nil {
		t.Fatal(err)
	}
	warmRelease()
	publicResponse := httptest.NewRecorder()
	svc.Handler.ServeHTTP(publicResponse, httptest.NewRequest(http.MethodPost, "/operations/resolve", strings.NewReader(`{"operation_id":"`+rec.ID+`","review_ref":"`+approved.ReviewRef+`"}`)))
	if publicResponse.Code == http.StatusNoContent {
		t.Fatal("public handler performed operator resolution")
	}
	publicRow, err := svc.operations.Lookup(context.Background(), rec.ID)
	if err != nil || publicRow.Phase != contracts.OperationPendingReview {
		t.Fatalf("public handler changed review row: %+v, %v", publicRow, err)
	}
	adminRequest := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		svc.AdminHandler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/operations/resolve", strings.NewReader(`{"operation_id":"`+rec.ID+`","review_ref":"`+approved.ReviewRef+`"}`)))
		return w
	}
	if w := adminRequest(); w.Code != http.StatusNoContent {
		t.Fatalf("operator resolve status %d: %s", w.Code, w.Body.String())
	}
	for metric, want := range map[string]int64{"writes": 1, "input_tokens": 7} {
		var used int64
		if err = db.DB().QueryRow(`SELECT committed FROM usage_windows WHERE account_id=? AND window_key='1970-01' AND metric=?`, account, metric).Scan(&used); err != nil {
			t.Fatal(err)
		}
		if used != want {
			t.Fatalf("first resolution original-period %s=%d want %d", metric, used, want)
		}
	}
	if w := adminRequest(); w.Code != http.StatusNoContent {
		t.Fatalf("exact case replay status %d: %s", w.Code, w.Body.String())
	}
	for metric, want := range map[string]int64{"writes": 1, "input_tokens": 7} {
		var used int64
		if err = db.DB().QueryRow(`SELECT committed FROM usage_windows WHERE account_id=? AND window_key='1970-01' AND metric=?`, account, metric).Scan(&used); err != nil {
			t.Fatal(err)
		}
		if used != want {
			t.Fatalf("replay changed original-period %s=%d want %d", metric, used, want)
		}
	}
}

func TestAdminOperatorReviewColdInactiveAndCanceledKeepPendingReview(t *testing.T) {
	for _, tc := range []struct {
		name           string
		prepare        func(*testing.T, *store.Store, string)
		enterCanonical bool
		warm           bool
		cancelled      bool
	}{
		{name: "cold runtime", prepare: func(t *testing.T, _ *store.Store, _ string) {}},
		{name: "inactive brain", prepare: func(t *testing.T, db *store.Store, id string) {
			if _, err := db.DB().Exec(`UPDATE brains SET state='deleted' WHERE id=?`, id); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "unknown proof", prepare: func(t *testing.T, _ *store.Store, _ string) {}, enterCanonical: true, warm: true},
		{name: "canceled", prepare: func(t *testing.T, _ *store.Store, _ string) {}, cancelled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, cfg, account, brainPath := deletionFixture(t)
			brainID := filepath.Base(brainPath)
			ledger := &operation.Ledger{Store: db, Clock: func() time.Time { return time.Unix(100, 0).UTC() }}
			rec, err := ledger.Reserve(context.Background(), contracts.ReserveRequest{AccountID: account, BrainID: brainID, ClientKey: "held-review", Fingerprint: "held-review", QuotaPeriod: "1970-01", Source: "gateway.remember", LeaseFor: time.Second, Deltas: []contracts.ReserveDelta{{Metric: "writes", Units: 1, Limit: 2}}})
			if err != nil {
				t.Fatal(err)
			}
			if tc.enterCanonical {
				if _, err = ledger.EnterCanonical(context.Background(), rec.ID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = ledger.Finalize(context.Background(), rec.ID, contracts.OperationPendingReview, contracts.Evidence{Kind: contracts.EvidenceUnknown, Ref: "needs-proof"}); err != nil {
				t.Fatal(err)
			}
			deps := testLifecycleDependencies(t)
			deps.OperatorReviewAdmission = reviewAdmissionFunc(func(context.Context, *http.Request, string, string) (ApprovedOperatorReview, error) {
				return ApprovedOperatorReview{OperationID: rec.ID, ReviewRef: "case-held", OperatorID: "operator-7", Outcome: contracts.OperationCommitted, ExpectedCanonicalRef: "expected-fact", ApprovedAt: time.Now()}, nil
			})
			svc, err := AssembleWithDependencies(context.Background(), cfg, true, db, nil, deletionEmbedding{}, deps)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = svc.Close() })
			tc.prepare(t, db, brainID)
			if tc.warm {
				_, release, acquireErr := svc.Pool.Acquire(context.Background(), brainID)
				if acquireErr != nil {
					t.Fatal(acquireErr)
				}
				release()
			}
			request := httptest.NewRequest(http.MethodPost, "/operations/resolve", strings.NewReader(`{"operation_id":"`+rec.ID+`","review_ref":"case-held"}`))
			if tc.cancelled {
				ctx, cancel := context.WithCancel(request.Context())
				cancel()
				request = request.WithContext(ctx)
			}
			w := httptest.NewRecorder()
			svc.AdminHandler().ServeHTTP(w, request)
			if w.Code == http.StatusNoContent {
				t.Fatal("held operation resolved without live matching positive proof")
			}
			current, err := svc.operations.Lookup(context.Background(), rec.ID)
			if err != nil || current.Phase != contracts.OperationPendingReview {
				t.Fatalf("refusal changed review row: %+v, %v", current, err)
			}
			var committed int64
			if err = db.DB().QueryRow(`SELECT COALESCE((SELECT committed FROM usage_windows WHERE account_id=? AND window_key='1970-01' AND metric='writes'),0)`, account).Scan(&committed); err != nil {
				t.Fatal(err)
			}
			if committed != 0 {
				t.Fatalf("refusal changed committed usage to %d", committed)
			}
			if tc.name == "cold runtime" {
				if _, err = os.Lstat(filepath.Join(brainPath, ".git")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("refusal initialized cold brain: %v", err)
				}
			}
		})
	}
}
