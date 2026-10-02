package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/admintransport"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/operation"
	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/writer"
)

// Test-only admission: it requires the real admintransport peer UID and an
// immutable in-memory case binding. It is not a human authenticator.
func TestAdminOperatorReviewOverAuthenticatedUnixSocket(t *testing.T) {
	db, cfg, account, brainPath := deletionFixture(t)
	initializeWarmCanonicalGitFixture(t, brainPath)
	brainID := filepath.Base(brainPath)
	ledger := &operation.Ledger{Store: db, Clock: func() time.Time { return time.Unix(100, 0).UTC() }}
	rec, err := ledger.Reserve(context.Background(), contracts.ReserveRequest{
		AccountID: account, BrainID: brainID, ClientKey: "operator-review-unix", Fingerprint: "unix-proof",
		QuotaPeriod: "1970-01", Source: "gateway.remember", LeaseFor: time.Second,
		Deltas: []contracts.ReserveDelta{{Metric: "writes", Units: 1, Limit: 2}, {Metric: "input_tokens", Units: 7, Limit: 20}},
	})
	if err != nil {
		t.Fatal(err)
	}

	writerPool, err := pool.New(pool.Config{MaxOpen: 2, MaxInFlight: 4, BrainsRoot: filepath.Dir(brainPath), Embedder: deletionEmbedding{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writerPool.Close() })
	runtime, releaseWriter, err := writerPool.Acquire(context.Background(), brainID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseWriter)
	canonicalCtx := writer.WithCanonicalOperation(context.Background(), writer.CanonicalOperation{ID: rec.ID, BeforeCommit: func(ctx context.Context, _ string) error {
		_, err := ledger.EnterCanonical(ctx, rec.ID)
		return err
	}})
	args, err := json.Marshal(map[string]string{
		"operation_key": rec.ID, "fact": "The operator recovery marker is silver cedar",
		"provenance": "human-reviewed proof fixture", "kind": "fact", "visibility": "world",
	})
	if err != nil {
		t.Fatal(err)
	}
	wrote := false
	for _, tool := range runtime.Tools {
		if tool.Name == "remember" {
			result, writeErr := tool.Handler(canonicalCtx, args)
			if writeErr != nil || result.IsError {
				t.Fatalf("write canonical fixture: %+v %v", result, writeErr)
			}
			wrote = true
			break
		}
	}
	if !wrote {
		t.Fatal("remember tool missing")
	}
	releaseWriter()
	if err = writerPool.Close(); err != nil {
		t.Fatal(err)
	}

	proofPool, err := pool.New(pool.Config{MaxOpen: 2, MaxInFlight: 4, BrainsRoot: filepath.Dir(brainPath), Embedder: deletionEmbedding{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = proofPool.Close() })
	proofRuntime, releaseProof, err := proofPool.Acquire(context.Background(), brainID)
	if err != nil {
		t.Fatal(err)
	}
	entered, err := ledger.Lookup(context.Background(), rec.ID)
	if err != nil {
		releaseProof()
		t.Fatal(err)
	}
	verdict, err := proofRuntime.Check(context.Background(), entered)
	releaseProof()
	if closeErr := proofPool.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil || verdict.Outcome != contracts.CanonicalLanded || !strings.HasPrefix(verdict.Ref, "fact:") {
		t.Fatalf("canonical proof = %+v, %v", verdict, err)
	}
	if _, err = ledger.Finalize(context.Background(), rec.ID, contracts.OperationPendingReview, contracts.Evidence{Kind: contracts.EvidenceUnknown, Ref: "human-review-required"}); err != nil {
		t.Fatal(err)
	}

	const reviewRef = "case-unix-2026-10-01-1"
	approved := ApprovedOperatorReview{
		OperationID: rec.ID, ReviewRef: reviewRef, OperatorID: "fixture-operator",
		Outcome: contracts.OperationCommitted, ExpectedCanonicalRef: verdict.Ref,
		ApprovedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
	}
	var admissionCalls atomic.Int32
	deps := testLifecycleDependencies(t)
	deps.OperatorReviewAdmission = reviewAdmissionFunc(func(ctx context.Context, _ *http.Request, operationID, ref string) (ApprovedOperatorReview, error) {
		admissionCalls.Add(1)
		uid, ok := admintransport.AuthenticatedUID(ctx)
		if !ok || uid != uint32(os.Geteuid()) || operationID != approved.OperationID || ref != approved.ReviewRef {
			return ApprovedOperatorReview{}, errors.New("fixture admission refused unbound peer or case")
		}
		return approved, nil
	})
	svc, err := AssembleWithDependencies(context.Background(), cfg, true, db, nil, deletionEmbedding{}, deps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	_, releaseWarm, err := svc.Pool.Acquire(context.Background(), brainID)
	if err != nil {
		t.Fatal(err)
	}
	releaseWarm()

	body := `{"operation_id":"` + rec.ID + `","review_ref":"` + approved.ReviewRef + `"}`
	publicResponse := httptest.NewRecorder()
	svc.Handler.ServeHTTP(publicResponse, httptest.NewRequest(http.MethodPost, "/operations/resolve", strings.NewReader(body)))
	if publicResponse.Code == http.StatusNoContent || admissionCalls.Load() != 0 {
		t.Fatalf("public handler exposed operator route: status=%d admission calls=%d", publicResponse.Code, admissionCalls.Load())
	}

	socketDir, err := os.MkdirTemp(os.Getenv("TMPDIR"), "oprv-")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(socketDir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socketDir, err = filepath.EvalSymlinks(socketDir)
	if err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(socketDir, "a.sock")
	listener, err := admintransport.Listen(context.Background(), socketPath)
	if err != nil {
		t.Fatal(err)
	}
	adminServer, err := listener.HTTPServer(svc.AdminHandler())
	if err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- adminServer.Serve(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = adminServer.Shutdown(ctx)
		_ = listener.Close()
		if err := <-serveDone; err != http.ErrServerClosed {
			t.Errorf("admin HTTP server Serve: %v", err)
		}
	})

	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
	}}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	t.Cleanup(transport.CloseIdleConnections)
	post := func() (*http.Response, error) {
		return client.Post("http://admin/operations/resolve", "application/json", strings.NewReader(body))
	}
	assertUsage := func(want int64) {
		t.Helper()
		for metric, expected := range map[string]int64{"writes": want, "input_tokens": 7 * want} {
			var committed int64
			if err := db.DB().QueryRow(`SELECT committed FROM usage_windows WHERE account_id=? AND window_key='1970-01' AND metric=?`, account, metric).Scan(&committed); err != nil {
				t.Fatal(err)
			}
			if committed != expected {
				t.Fatalf("original-period %s usage = %d, want %d", metric, committed, expected)
			}
		}
	}

	resp, err := post()
	if err != nil {
		t.Fatal(err)
	}
	firstBody, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil || resp.StatusCode != http.StatusNoContent || len(firstBody) != 0 || admissionCalls.Load() != 1 {
		t.Fatalf("authenticated Unix resolution: status=%d body=%q admission calls=%d readErr=%v", resp.StatusCode, firstBody, admissionCalls.Load(), readErr)
	}
	assertUsage(1)

	resp, err = post()
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || admissionCalls.Load() != 2 {
		t.Fatalf("same-case replay: status=%d admission calls=%d", resp.StatusCode, admissionCalls.Load())
	}
	assertUsage(1)
	final, err := svc.operations.Lookup(context.Background(), rec.ID)
	if err != nil || final.Phase != contracts.OperationCommitted || final.Evidence != (contracts.Evidence{Kind: contracts.EvidenceOperatorReview, Ref: reviewRef}) {
		t.Fatalf("resolved ledger row = %+v, %v", final, err)
	}

	// The HTTPServer wrapper refuses forged headers without its peer context.
	forged := httptest.NewRequest(http.MethodPost, "/operations/resolve", strings.NewReader(body))
	forged.Header.Set("X-Operator-UID", "0")
	forged.Header.Set("Authorization", "Bearer claimed-owner")
	forgedResponse := httptest.NewRecorder()
	adminServer.Handler.ServeHTTP(forgedResponse, forged)
	if forgedResponse.Code != http.StatusForbidden || admissionCalls.Load() != 2 {
		t.Fatalf("forged transport identity: status=%d admission calls=%d", forgedResponse.Code, admissionCalls.Load())
	}
}
