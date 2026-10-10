package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

type startupProofTest struct {
	observation contracts.JournalObservation
	readyErr    error
	verifyErr   error
	checks      int
}

func (p *startupProofTest) Position() contracts.JournalPosition { return p.observation.Position }
func (p *startupProofTest) BuildSHA() string                    { return "startup-test-build" }
func (p *startupProofTest) VerifyObservation(_ context.Context, o contracts.JournalObservation) error {
	if o.HistorySHA256 != p.observation.HistorySHA256 {
		return errors.New("wrong history")
	}
	return p.verifyErr
}
func (p *startupProofTest) CheckReadiness(context.Context) error { p.checks++; return p.readyErr }

type startupReaderTest struct{ observation contracts.JournalObservation }

func (r startupReaderTest) Observe(context.Context) (contracts.JournalObservation, error) {
	return r.observation, nil
}

type startupLeaseTest struct{ close func(context.Context) error }

func (l startupLeaseTest) Close(ctx context.Context) error { return l.close(ctx) }
func startupDeps(t *testing.T) (LifecycleDependencies, *startupProofTest) {
	t.Helper()
	d := testLifecycleDependencies(t)
	d.Recovery = nil
	p := &startupProofTest{observation: contracts.JournalObservation{Position: contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis"}, HistorySHA256: "verified-test-history"}}
	d.Startup = p
	d.BuildSHA = p.BuildSHA()
	d.JournalReader = startupReaderTest{p.observation}
	d.StartupLease = startupLeaseTest{func(context.Context) error { return nil }}
	return d, p
}
func TestStartupRequiresCompleteExclusiveTuple(t *testing.T) {
	for _, name := range []string{"missing-reader", "missing-lease", "mixed-recovery", "wrong-build", "typed-nil-proof"} {
		t.Run(name, func(t *testing.T) {
			d, _ := startupDeps(t)
			switch name {
			case "missing-reader":
				d.JournalReader = nil
			case "missing-lease":
				d.StartupLease = nil
			case "mixed-recovery":
				d.Recovery = testLifecycleDependencies(t).Recovery
			case "wrong-build":
				d.BuildSHA = "forged"
			case "typed-nil-proof":
				var p *startupProofTest
				d.Startup = p
			}
			if !errors.Is(d.validate(), ErrStartupUnavailable) {
				t.Fatal("invalid tuple accepted")
			}
		})
	}
}
func TestStartupUnsealedObservationDoesNotWeakenRecovery(t *testing.T) {
	db, cfg, _, _ := deletionFixture(t)
	d, p := startupDeps(t)
	s, err := AssembleWithDependencies(context.Background(), cfg, true, db, nil, deletionEmbedding{}, d)
	if err != nil {
		t.Fatal(err)
	}
	if p.checks != 3 {
		t.Fatalf("wanted pre/post and startup sweep admission checks, got %d", p.checks)
	}
	if s.Handler == nil {
		t.Fatal("handlers absent after verified ordinary startup")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestStartupRejectsObservationBeforePublishingHandlers(t *testing.T) {
	db, cfg, _, _ := deletionFixture(t)
	d, p := startupDeps(t)
	p.verifyErr = errors.New("untrusted observation")
	if s, err := AssembleWithDependencies(context.Background(), cfg, true, db, nil, deletionEmbedding{}, d); err == nil || s != nil {
		t.Fatal("untrusted history admitted")
	}
}
func TestAuthorityReadinessBypassesLocalHealthCache(t *testing.T) {
	_, p := startupDeps(t)
	s := &Service{startup: p, ready: true, readyAt: time.Now()}
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		s.readiness(w, httptest.NewRequest("GET", "/readyz", nil))
		if w.Code != http.StatusOK {
			t.Fatal(w.Code)
		}
	}
	p.readyErr = errors.New("revoked")
	w := httptest.NewRecorder()
	s.readiness(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != http.StatusServiceUnavailable || p.checks != 3 {
		t.Fatalf("cached authority readiness: status=%d checks=%d", w.Code, p.checks)
	}
}
func TestStartupCloseRevokesAfterDatabaseAndIgnoresCanceledParent(t *testing.T) {
	db, cfg, _, _ := deletionFixture(t)
	d, _ := startupDeps(t)
	closed := false
	d.StartupLease = startupLeaseTest{func(ctx context.Context) error {
		if ctx.Err() != nil {
			t.Fatal("cleanup inherited cancellation")
		}
		if db.DB().PingContext(ctx) == nil {
			t.Fatal("revoked before database closed")
		}
		closed = true
		return nil
	}}
	s, err := AssembleWithDependencies(context.Background(), cfg, true, db, nil, deletionEmbedding{}, d)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.CloseContext(ctx); err != nil {
		t.Fatal(err)
	}
	if !closed {
		t.Fatal("lease not revoked")
	}
}

func TestRevokedStartupPreventsWorkersBeforeDependencyIO(t *testing.T) {
	_, p := startupDeps(t)
	p.readyErr = errors.New("revoked")
	s := &Service{startup: p}
	if _, err := s.reconcileWith(context.Background(), nil); err == nil {
		t.Fatal("operation sweep ignored revoked authority")
	}
	if _, err := s.billingAccountPage(context.Background(), ""); err == nil {
		t.Fatal("billing page ignored revoked authority")
	}
	if err := s.reconcileBillingAccount(context.Background(), "account"); err == nil {
		t.Fatal("billing customer ignored revoked authority")
	}
	w := httptest.NewRecorder()
	s.AdminHandler().ServeHTTP(w, httptest.NewRequest("POST", "/backup", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal("private handler admitted revoked authority")
	}
}
