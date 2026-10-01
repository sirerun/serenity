package billing

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func TestCheckoutRateLimiterWindowRetryAndAccountIsolation(t *testing.T) {
	var limiter checkoutRateLimiter
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < checkoutAttemptsPerAccount; i++ {
		allowed, retry, err := limiter.charge(context.Background(), "account-a", now)
		if err != nil || !allowed || retry != 0 {
			t.Fatalf("attempt %d: allowed=%v retry=%s err=%v", i+1, allowed, retry, err)
		}
	}
	allowed, retry, err := limiter.charge(context.Background(), "account-a", now.Add(7*time.Second))
	if err != nil || allowed || retry != 53*time.Second {
		t.Fatalf("sixth attempt: allowed=%v retry=%s err=%v; want 53s", allowed, retry, err)
	}
	allowed, _, err = limiter.charge(context.Background(), "account-b", now)
	if err != nil || !allowed {
		t.Fatalf("account B was charged against account A: allowed=%v err=%v", allowed, err)
	}
	allowed, _, err = limiter.charge(context.Background(), "account-a", now.Add(checkoutRateWindow))
	if err != nil || !allowed {
		t.Fatalf("window did not reset: allowed=%v err=%v", allowed, err)
	}
}

func TestCheckoutRateLimiterCancellationAndBoundedIdleCleanup(t *testing.T) {
	var limiter checkoutRateLimiter
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if allowed, _, err := limiter.charge(canceled, "cancelled", now); allowed || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled attempt: allowed=%v err=%v", allowed, err)
	}
	if len(limiter.accounts) != 0 {
		t.Fatalf("canceled attempt consumed quota: %d keys", len(limiter.accounts))
	}
	for i := 0; i < checkoutTrackedAccountCap; i++ {
		if allowed, _, err := limiter.charge(context.Background(), strconv.Itoa(i), now); err != nil || !allowed {
			t.Fatalf("fill key %d: allowed=%v err=%v", i, allowed, err)
		}
	}
	if allowed, retry, err := limiter.charge(context.Background(), "overflow", now); err != nil || allowed || retry != checkoutRateWindow {
		t.Fatalf("capacity admission: allowed=%v retry=%s err=%v", allowed, retry, err)
	}
	if got := len(limiter.accounts); got != checkoutTrackedAccountCap {
		t.Fatalf("tracked key count=%d, want capped at %d", got, checkoutTrackedAccountCap)
	}
	if allowed, _, err := limiter.charge(context.Background(), "after-window", now.Add(checkoutRateWindow)); err != nil || !allowed {
		t.Fatalf("idle key cleanup failed: allowed=%v err=%v", allowed, err)
	}
	if got := len(limiter.accounts); got != 1 {
		t.Fatalf("idle sweep retained %d keys, want 1", got)
	}
}

func TestCheckoutPreflightDoesNotConsumeRateLimitOrCallProvider(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	account, err := db.CreateAccount(ctx, "checkout-preflight@example.test")
	if err != nil {
		t.Fatal(err)
	}
	var providerCalls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		http.Error(w, "unexpected provider request", http.StatusServiceUnavailable)
	}))
	defer provider.Close()
	svc := &Service{Store: db, Config: Config{BuilderPrice: "price_builder", ScalePrice: "price_scale", BaseURL: provider.URL}}
	if _, err = svc.Checkout(ctx, account.ID, "unknown"); err == nil {
		t.Fatal("unknown plan was accepted")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = svc.Checkout(canceled, account.ID, "builder"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled checkout error=%v", err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET status='deleting' WHERE id=?`, account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Checkout(ctx, account.ID, "builder"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("frozen account error=%v, want no active account", err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET status='active' WHERE id=?`, account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `INSERT INTO subscriptions(id,account_id,price_id,plan_id,status) VALUES('sub_existing',?,'price_builder','builder','active')`, account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Checkout(ctx, account.ID, "builder"); err == nil {
		t.Fatal("checkout with existing local subscription was accepted")
	}
	if len(svc.checkoutLimiter.accounts) != 0 {
		t.Fatalf("preflight consumed quota: %d account keys", len(svc.checkoutLimiter.accounts))
	}
	if providerCalls.Load() != 0 {
		t.Fatalf("preflight called provider %d times", providerCalls.Load())
	}
}

func TestCheckoutProviderFailureConsumesAttemptAndHTTPReturnsRetryAfter(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	account, err := db.CreateAccount(ctx, "checkout-rate@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_checkout' WHERE id=?`, account.ID); err != nil {
		t.Fatal(err)
	}
	var providerCalls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		http.Error(w, "provider unavailable", http.StatusServiceUnavailable)
	}))
	defer provider.Close()
	identityService := &identity.Service{Store: db}
	svc := &Service{Store: db, Identity: identityService, Config: Config{WebhookSecret: "secret", BuilderPrice: "price_builder", ScalePrice: "price_scale", BaseURL: provider.URL}}
	if _, err = svc.Checkout(ctx, account.ID, "builder"); err == nil {
		t.Fatal("provider failure was hidden")
	}
	entry := svc.checkoutLimiter.accounts[account.ID]
	if entry.count != 1 {
		t.Fatalf("provider failure charged %d attempts, want 1", entry.count)
	}
	for i := 1; i < checkoutAttemptsPerAccount; i++ {
		if allowed, _, err := svc.checkoutLimiter.charge(ctx, account.ID, time.Now().UTC()); err != nil || !allowed {
			t.Fatalf("seed attempt %d: allowed=%v err=%v", i+1, allowed, err)
		}
	}
	_, checkoutErr := svc.Checkout(ctx, account.ID, "builder")
	var rateLimitErr *CheckoutRateLimitError
	if !errors.As(checkoutErr, &rateLimitErr) || rateLimitErr.RetryAfter() <= 0 {
		t.Fatalf("direct checkout error=%v; want typed rate-limit error with retry duration", checkoutErr)
	}

	const sessionToken = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	now := time.Now().UTC()
	if _, err = db.DB().ExecContext(ctx, `INSERT INTO sessions(id,account_id,token_hash,created_at,expires_at,csrf_secret) VALUES(?,?,?,?,?,?)`, store.ID(), account.ID, store.Hash(sessionToken), store.Stamp(now), store.Stamp(now.Add(time.Hour)), "csrf-token"); err != nil {
		t.Fatal(err)
	}
	form := url.Values{"plan": {"builder"}, "csrf": {"csrf-token"}}
	request := httptest.NewRequest(http.MethodPost, "https://app.example.test/billing/checkout", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(&http.Cookie{Name: "serenity_session", Value: sessionToken})
	response := httptest.NewRecorder()
	svc.ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "60" {
		t.Fatalf("rate limit response status=%d Retry-After=%q body=%q", response.Code, response.Header().Get("Retry-After"), response.Body.String())
	}
	if providerCalls.Load() != 1 {
		t.Fatalf("limited checkout made additional provider calls: %d", providerCalls.Load())
	}
}
