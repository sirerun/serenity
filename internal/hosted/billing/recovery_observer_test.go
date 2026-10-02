package billing

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/store"
)

const recoveryTestAccount = "accountobserver123456"
const recoveryTestCustomer = "cus_observer_test"

type recoveryTestHandler func(http.ResponseWriter, *http.Request)

type recoveryTestRoundTripper struct {
	serverURL  *url.URL
	underlying http.RoundTripper
	mu         sync.Mutex
	requests   []*http.Request
}

func (r *recoveryTestRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method != http.MethodGet || request.URL.Scheme != "https" || request.URL.Host != "api.stripe.com" {
		return nil, fmt.Errorf("unexpected observer request destination")
	}
	if user, password, ok := request.BasicAuth(); !ok || password != "" || user != "sk_test_"+strings.Repeat("K", 32) {
		return nil, fmt.Errorf("unexpected observer authorization")
	}
	if request.Header.Get("Stripe-Version") != APIVersion {
		return nil, fmt.Errorf("unexpected observer API version")
	}
	r.mu.Lock()
	copyRequest := request.Clone(context.Background())
	r.requests = append(r.requests, copyRequest)
	r.mu.Unlock()
	clone := request.Clone(request.Context())
	u := *request.URL
	u.Scheme, u.Host = r.serverURL.Scheme, r.serverURL.Host
	clone.URL = &u
	clone.Host = ""
	return r.underlying.RoundTrip(clone)
}

func (r *recoveryTestRoundTripper) snapshot() []*http.Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*http.Request(nil), r.requests...)
}

type recoveryTestEnv struct {
	db           *store.Store
	service      *Service
	observer     *RecoveryObserver
	roundTripper *recoveryTestRoundTripper
	accountID    string
	customer     string
}

func testRecoveryLimits() RecoveryObserverLimits {
	return RecoveryObserverLimits{Timeout: 30 * time.Second, ResponseBytes: 1 << 20, TotalResponseBytes: 64 << 20, JSONDepth: 32, JSONNodes: 100_000, ProviderGETs: 128, EventPages: 32, Invoices: 16, InvoiceLinePages: 4}
}

func newRecoveryTestEnv(t *testing.T, handler recoveryTestHandler, limits RecoveryObserverLimits) *recoveryTestEnv {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(handler))
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "recovery.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	accountID, customer := recoveryTestAccount, recoveryTestCustomer
	_, err = db.DB().Exec(`INSERT INTO accounts(id,email_hash,email,created_at,status,plan_id,plan_version,stripe_customer_id) VALUES(?,?,?,?,?,?,?,?)`, accountID, "hash-"+accountID, accountID+"@example.invalid", store.Stamp(time.Now()), "restore_pending", "free", 1, customer)
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{Store: db, Config: Config{SecretKey: "sk_test_" + strings.Repeat("K", 32), BuilderPrice: "price_builder", ScalePrice: "price_scale", BaseURL: "https://wrong.example/v1", Client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("shared client was used") })}}}
	observer, err := NewRecoveryObserver(service, limits)
	if err != nil {
		t.Fatal(err)
	}
	rt := &recoveryTestRoundTripper{serverURL: serverURL, underlying: server.Client().Transport}
	observer.client = &http.Client{Transport: rt, CheckRedirect: observer.client.CheckRedirect}
	return &recoveryTestEnv{db: db, service: service, observer: observer, roundTripper: rt, accountID: accountID, customer: customer}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func recoveryWriteJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func recoverySubscriptionList(customer, status string, start, end int64, id, itemID, price string) map[string]any {
	return map[string]any{"object": "list", "data": []any{map[string]any{
		"id": id, "object": "subscription", "customer": customer, "status": status, "cancel_at_period_end": false,
		"items": map[string]any{"object": "list", "data": []any{map[string]any{
			"id": itemID, "object": "subscription_item", "price": map[string]any{"id": price, "object": "price", "active": true, "currency": "usd", "metadata": map[string]any{"benign": "yes"}},
			"current_period_start": start, "current_period_end": end, "quantity": 1, "metadata": map[string]any{"benign": "yes"},
		}}, "has_more": false, "url": "/v1/subscription_items"},
		"metadata": map[string]any{"benign": "yes"}, "latest_invoice": "in_latest_is_not_authority",
	}}, "has_more": false, "url": "/v1/subscriptions"}
}

func recoveryLine(invoice, id, subscription, item, price string, start, end int64, proration bool) map[string]any {
	return map[string]any{
		"id": id, "object": "line_item", "amount": 1000, "currency": "usd", "description": "bounded harmless extra", "discount_amounts": []any{}, "discountable": true, "discounts": []any{}, "invoice": invoice, "livemode": false, "metadata": map[string]any{"ignored": "bounded"}, "taxes": []any{},
		"parent":   map[string]any{"type": "subscription_item_details", "subscription_item_details": map[string]any{"subscription": subscription, "subscription_item": item, "proration": proration, "invoice_item": nil, "proration_details": nil}},
		"period":   map[string]any{"start": start, "end": end},
		"pricing":  map[string]any{"type": "price_details", "price_details": map[string]any{"price": price, "product": "prod_observer"}, "unit_amount_decimal": "1000"},
		"quantity": 1,
	}
}

func recoveryInvoiceEvent(id, invoiceID, customer, subscription, item, price string, created, start, end int64) map[string]any {
	line := recoveryLine(invoiceID, "il_"+invoiceID, subscription, item, price, start, end, false)
	return map[string]any{"id": id, "object": "event", "api_version": APIVersion, "created": created, "livemode": false, "pending_webhooks": 1, "type": "invoice.payment_failed", "data": map[string]any{"object": map[string]any{
		"id": invoiceID, "object": "invoice", "customer": customer, "livemode": false, "lines": map[string]any{"object": "list", "data": []any{line}, "has_more": false, "url": "/v1/invoices/" + invoiceID + "/lines"}, "amount_due": 1000, "currency": "usd", "status": "open", "description": "harmless invoice attribute",
	}}, "harmless_event_attribute": map[string]any{"bounded": true}}
}

func recoverySubscriptionEvent(id, kind, customer, subscription, item, price, status, oldStatus string, created, start, end int64) map[string]any {
	resource := recoverySubscriptionList(customer, status, start, end, subscription, item, price)["data"].([]any)[0]
	data := map[string]any{"object": resource}
	if kind == "customer.subscription.updated" {
		previous := map[string]any{}
		if oldStatus != "" {
			previous["status"] = oldStatus
		}
		data["previous_attributes"] = previous
	}
	return map[string]any{"id": id, "object": "event", "api_version": APIVersion, "created": created, "livemode": false, "type": kind, "data": data, "request": nil, "harmless_event_attribute": "bounded"}
}

func recoveryEventPage(w http.ResponseWriter, r *http.Request, events []map[string]any) {
	if r.URL.Query().Get("limit") != "100" {
		http.Error(w, "bad page size", http.StatusBadRequest)
		return
	}
	cursor := r.URL.Query().Get("starting_after")
	start := 0
	if cursor != "" {
		start = -1
		for i, event := range events {
			if event["id"] == cursor {
				start = i + 1
				break
			}
		}
		if start < 0 {
			http.Error(w, "bad cursor", http.StatusBadRequest)
			return
		}
	}
	end := start + 100
	if end > len(events) {
		end = len(events)
	}
	data := make([]any, 0, end-start)
	for _, event := range events[start:end] {
		data = append(data, event)
	}
	recoveryWriteJSON(w, map[string]any{"object": "list", "data": data, "has_more": end < len(events), "url": "/v1/events"})
}

func recoveryInvoiceHandler(w http.ResponseWriter, r *http.Request, invoiceID, customer string, lines []map[string]any) {
	switch r.URL.Path {
	case "/v1/invoices/" + invoiceID:
		recoveryWriteJSON(w, map[string]any{"id": invoiceID, "object": "invoice", "customer": customer, "livemode": false, "lines": map[string]any{"object": "list", "data": []any{}, "has_more": true}, "status": "open", "metadata": map[string]any{"mutable": true}})
	case "/v1/invoices/" + invoiceID + "/lines":
		data := make([]any, len(lines))
		for i, line := range lines {
			data[i] = line
		}
		recoveryWriteJSON(w, map[string]any{"object": "list", "data": data, "has_more": false, "url": "/v1/invoices/" + invoiceID + "/lines"})
	default:
		http.NotFound(w, r)
	}
}

func recoverySQLSnapshot(t *testing.T, db *sql.DB) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, table := range []string{"accounts", "checkout_attempts", "subscriptions", "billing_failures", "audit_log"} {
		rows, err := db.Query("SELECT * FROM " + table + " ORDER BY rowid")
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			values := make([]any, len(columns))
			dest := make([]any, len(columns))
			for i := range values {
				dest[i] = &values[i]
			}
			if err := rows.Scan(dest...); err != nil {
				t.Fatal(err)
			}
			out[table] = append(out[table], fmt.Sprintf("%#v", values))
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		sort.Strings(out[table])
	}
	return out
}

func seedRecoveryLocalEvidence(t *testing.T, env *recoveryTestEnv) {
	t.Helper()
	now := time.Now().UTC()
	_, err := env.db.DB().Exec(`INSERT INTO subscriptions(id,account_id,price_id,plan_id,status,current_period_start,current_period_end,cancel_at_period_end,grace_until,grace_invoice_id) VALUES(?,?,?,?,?,?,?,?,?,?)`, "sub_local", env.accountID, "price_scale", "scale", "past_due", store.Stamp(now.Add(-24*time.Hour)), store.Stamp(now.Add(24*time.Hour)), 0, store.Stamp(now.Add(48*time.Hour)), "in_local")
	if err != nil {
		t.Fatal(err)
	}
	_, err = env.db.DB().Exec(`INSERT INTO billing_failures(account_id,subscription_id,invoice_id,event_id,first_failed_at) VALUES(?,?,?,?,?)`, env.accountID, "sub_local", "in_local", "evt_local", store.Stamp(now.Add(-24*time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = env.db.DB().Exec(`INSERT INTO audit_log(account_id,actor,action,created_at,detail) VALUES(?,?,?,?,?)`, env.accountID, "test", "seed", store.Stamp(now), "preserve")
	if err != nil {
		t.Fatal(err)
	}
}

func assertZeroObservation(t *testing.T, got contracts.RecoveryBillingObservation) {
	t.Helper()
	if got != (contracts.RecoveryBillingObservation{}) {
		t.Fatalf("error returned nonzero observation: %#v", got)
	}
}

func TestRecoveryObserverConstructorAndLimits(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "config.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	service := &Service{Store: db, Config: Config{SecretKey: "sk_test_" + strings.Repeat("K", 32), BuilderPrice: "price_builder", ScalePrice: "price_scale", BaseURL: "http://changed.invalid"}}
	if _, err := NewRecoveryObserver(service, testRecoveryLimits()); err != nil {
		t.Fatal(err)
	}
	limits := testRecoveryLimits()
	invalid := []RecoveryObserverLimits{{}, func() RecoveryObserverLimits { x := limits; x.TotalResponseBytes = 64<<20 + 1; return x }(), func() RecoveryObserverLimits { x := limits; x.ProviderGETs = 129; return x }(), func() RecoveryObserverLimits { x := limits; x.EventPages = 0; return x }()}
	for i, value := range invalid {
		if _, err := NewRecoveryObserver(service, value); !errors.Is(err, ErrRecoveryObserverConfiguration) {
			t.Errorf("case %d: got %v", i, err)
		}
	}
	badKey := &Service{Store: db, Config: service.Config}
	badKey.Config.SecretKey = "sk_test_" + strings.Repeat("K", 20) + "\n"
	if _, err := NewRecoveryObserver(badKey, limits); !errors.Is(err, ErrRecoveryObserverConfiguration) {
		t.Fatalf("bad key accepted: %v", err)
	}
}

func TestRecoveryObserverActiveResultUsesPrivateGETAndReadOnlySQL(t *testing.T) {
	now := time.Now().UTC()
	start, end := now.Add(-24*time.Hour).Unix(), now.Add(24*time.Hour).Unix()
	env := newRecoveryTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/subscriptions" || r.URL.Query().Get("customer") != recoveryTestCustomer || r.URL.Query().Get("status") != "all" || r.URL.Query().Get("limit") != "100" {
			http.NotFound(w, r)
			return
		}
		recoveryWriteJSON(w, recoverySubscriptionList(recoveryTestCustomer, "active", start, end, "sub_current", "si_current", "price_builder"))
	}, testRecoveryLimits())
	seedRecoveryLocalEvidence(t, env)
	before := recoverySQLSnapshot(t, env.db.DB())
	got, err := env.observer.ObserveRestorePending(context.Background(), env.accountID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Eligible || got.PlanID != "builder" || got.AccountID != env.accountID || got.Source != recoverySource || !got.CurrentWindowStart.Equal(time.Unix(start, 0).UTC()) || !got.CurrentWindowEnd.Equal(time.Unix(end, 0).UTC()) || got.ObservedAt.IsZero() || !got.GraceUntil.IsZero() {
		t.Fatalf("unexpected active observation: %#v", got)
	}
	if after := recoverySQLSnapshot(t, env.db.DB()); fmt.Sprint(before) != fmt.Sprint(after) {
		t.Fatal("observer changed billing SQL state")
	}
	requests := env.roundTripper.snapshot()
	if len(requests) != 1 {
		t.Fatalf("provider GET count=%d", len(requests))
	}
	for _, request := range requests {
		if request.Method != "GET" || request.URL.Host != "api.stripe.com" {
			t.Fatalf("request escaped private GET policy: %s %s", request.Method, request.URL)
		}
	}
}

func TestRecoveryObserverPreflightLockAndFinalReread(t *testing.T) {
	now := time.Now().UTC()
	start, end := now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix()
	t.Run("pending checkout refuses before provider", func(t *testing.T) {
		env := newRecoveryTestEnv(t, func(w http.ResponseWriter, r *http.Request) { t.Errorf("unexpected provider request: %s", r.URL.Path) }, testRecoveryLimits())
		_, err := env.db.DB().Exec(`INSERT INTO checkout_attempts(account_id,id,price_id,created_at) VALUES(?,?,?,?)`, env.accountID, "attempt", "price_builder", store.Stamp(now))
		if err != nil {
			t.Fatal(err)
		}
		got, err := env.observer.ObserveRestorePending(context.Background(), env.accountID)
		assertZeroObservation(t, got)
		if !errors.Is(err, ErrRecoveryObserverAccountState) {
			t.Fatalf("got %v", err)
		}
		if len(env.roundTripper.snapshot()) != 0 {
			t.Fatal("preflight made a provider request")
		}
	})
	t.Run("configured deadline includes lock wait", func(t *testing.T) {
		limits := testRecoveryLimits()
		limits.Timeout = 15 * time.Millisecond
		env := newRecoveryTestEnv(t, func(w http.ResponseWriter, r *http.Request) { t.Errorf("unexpected provider request") }, limits)
		release, err := env.service.locks.acquire(context.Background(), "account:"+env.accountID)
		if err != nil {
			t.Fatal(err)
		}
		got, observeErr := env.observer.ObserveRestorePending(context.Background(), env.accountID)
		release()
		assertZeroObservation(t, got)
		if !errors.Is(observeErr, context.DeadlineExceeded) {
			t.Fatalf("got %v", observeErr)
		}
		if len(env.roundTripper.snapshot()) != 0 {
			t.Fatal("lock timeout made a provider request")
		}
	})
	t.Run("final snapshot detects concurrent status change", func(t *testing.T) {
		var env *recoveryTestEnv
		env = newRecoveryTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
			_, e := env.db.DB().Exec(`UPDATE accounts SET status='active' WHERE id=?`, env.accountID)
			if e != nil {
				t.Errorf("update test account: %v", e)
			}
			recoveryWriteJSON(w, recoverySubscriptionList(recoveryTestCustomer, "active", start, end, "sub_current", "si_current", "price_builder"))
		}, testRecoveryLimits())
		got, err := env.observer.ObserveRestorePending(context.Background(), env.accountID)
		assertZeroObservation(t, got)
		if !errors.Is(err, ErrRecoveryObserverAccountState) {
			t.Fatalf("got %v", err)
		}
		if len(env.roundTripper.snapshot()) != 1 {
			t.Fatalf("GET count=%d", len(env.roundTripper.snapshot()))
		}
	})
}

func TestRecoveryObserverRejectsInvalidUnicodeHTTPBodies(t *testing.T) {
	cases := []struct {
		name string
		body []byte
	}{{"invalid-byte-key", []byte(`{"object":"list","data":[],"has_more":false,"x` + "\xff" + `":"v"}`)}, {"invalid-byte-value", []byte(`{"object":"list","data":[],"has_more":false,"x":"` + "\xff" + `"}`)}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newRecoveryTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(tc.body)
			}, testRecoveryLimits())
			got, err := env.observer.ObserveRestorePending(context.Background(), env.accountID)
			assertZeroObservation(t, got)
			if !errors.Is(err, contracts.ErrBillingProviderAmbiguous) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestRecoveryUnicodeDecoderRejectsUnpairedSurrogates(t *testing.T) {
	for _, body := range []string{`{"x":"\uD800"}`, `{"\uDC00":"value"}`, `{"x":"\uD800\u0041"}`} {
		if _, err := decodeRecoveryJSON([]byte(body), 32, 100); err == nil {
			t.Errorf("accepted invalid Unicode: %s", body)
		}
	}
	if _, err := decodeRecoveryJSON([]byte(`{"x":"\uD83D\uDE00"}`), 32, 100); err != nil {
		t.Fatalf("valid surrogate pair refused: %v", err)
	}
}

func TestRecoveryStrictJSONShapeAndBounds(t *testing.T) {
	for _, body := range []string{`{"object":"list","object":"list"}`, `{"object":"list"} {}`, `{"x":[[[0]]]}`} {
		depth := 32
		if strings.Contains(body, "[[[") {
			depth = 2
		}
		if _, err := decodeRecoveryJSON([]byte(body), depth, 100); err == nil {
			t.Errorf("accepted malformed/bounded JSON %s", body)
		}
	}
	if _, err := decodeRecoveryJSON([]byte(`{"Object":"list"}`), 32, 100); err != nil {
		t.Fatalf("generic extra casing should parse before schema projection: %v", err)
	}
	obj := recoveryObject{"data": nil, "Data": nil}
	if _, _, err := recoveryField(obj, "data"); err == nil {
		t.Fatal("accepted recognized key alias")
	}
	if err := recoveryOnlyKeys(recoveryObject{"object": "list", "Object": "list"}, "object"); err == nil {
		t.Fatal("accepted decision key alias")
	}
	if err := recoveryOnlyKeys(recoveryObject{"object": "list", "future": true}, "object"); err == nil {
		t.Fatal("accepted unknown decision field")
	}
}

func TestRecoveryObserverRejectsRecognizedSchemaAliases(t *testing.T) {
	now := time.Now().UTC()
	start, end := now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix()
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"canonical-and-alias-object", func(response map[string]any) { response["Object"] = "list" }},
		{"alias-only-object", func(response map[string]any) { delete(response, "object"); response["Object"] = "list" }},
		{"canonical-and-alias-customer", func(response map[string]any) {
			row := response["data"].([]any)[0].(map[string]any)
			row["Customer"] = recoveryTestCustomer
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newRecoveryTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
				response := recoverySubscriptionList(recoveryTestCustomer, "active", start, end, "sub_alias", "si_alias", "price_builder")
				tc.mutate(response)
				recoveryWriteJSON(w, response)
			}, testRecoveryLimits())
			got, err := env.observer.ObserveRestorePending(context.Background(), env.accountID)
			assertZeroObservation(t, got)
			if !errors.Is(err, contracts.ErrBillingProviderAmbiguous) {
				t.Fatalf("recognized field alias was accepted: %v", err)
			}
		})
	}
}

func TestRecoveryObserverStatusResultTable(t *testing.T) {
	now := time.Now().UTC()
	start := now.Add(-48 * time.Hour).Unix()
	cases := []struct {
		name     string
		rows     []any
		plan     string
		eligible bool
		wantErr  bool
	}{
		{name: "empty list", rows: []any{}, plan: "free"},
		{name: "canceled unknown historical price", rows: []any{recoverySubscriptionList(recoveryTestCustomer, "canceled", start, start+3600, "sub_cancel", "si_cancel", "price_retired")["data"].([]any)[0]}, plan: "free"},
		{name: "incomplete expired terminal", rows: []any{recoverySubscriptionList(recoveryTestCustomer, "incomplete_expired", start, start+3600, "sub_expired", "si_expired", "price_retired")["data"].([]any)[0]}, plan: "free"},
		{name: "unpaid", rows: []any{recoverySubscriptionList(recoveryTestCustomer, "unpaid", start, start+3600, "sub_unpaid", "si_unpaid", "price_retired")["data"].([]any)[0]}, plan: "free"},
		{name: "paused", rows: []any{recoverySubscriptionList(recoveryTestCustomer, "paused", start, start+3600, "sub_paused", "si_paused", "price_retired")["data"].([]any)[0]}, plan: "free"},
		{name: "active expired period", rows: []any{recoverySubscriptionList(recoveryTestCustomer, "active", start, now.Add(-time.Minute).Unix(), "sub_active", "si_active", "price_builder")["data"].([]any)[0]}, plan: "builder"},
		{name: "trialing active window", rows: []any{recoverySubscriptionList(recoveryTestCustomer, "trialing", start, now.Add(time.Hour).Unix(), "sub_trial", "si_trial", "price_scale")["data"].([]any)[0]}, plan: "scale", eligible: true},
		{name: "incomplete refuses", rows: []any{recoverySubscriptionList(recoveryTestCustomer, "incomplete", start, start+3600, "sub_incomplete", "si_incomplete", "price_builder")["data"].([]any)[0]}, wantErr: true},
		{name: "two nonterminal rows refuse", rows: []any{recoverySubscriptionList(recoveryTestCustomer, "active", start, start+3600, "sub_multi_a", "si_multi_a", "price_builder")["data"].([]any)[0], recoverySubscriptionList(recoveryTestCustomer, "paused", start, start+3600, "sub_multi_b", "si_multi_b", "price_scale")["data"].([]any)[0]}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newRecoveryTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
				recoveryWriteJSON(w, map[string]any{"object": "list", "data": tc.rows, "has_more": false, "url": "/v1/subscriptions"})
			}, testRecoveryLimits())
			got, err := env.observer.ObserveRestorePending(context.Background(), env.accountID)
			if tc.wantErr {
				assertZeroObservation(t, got)
				if !errors.Is(err, contracts.ErrBillingProviderAmbiguous) {
					t.Fatalf("got %v", err)
				}
				return
			}
			if err != nil || got.PlanID != tc.plan || got.Eligible != tc.eligible {
				t.Fatalf("got %#v, %v", got, err)
			}
		})
	}
}

func TestRecoveryPastDueScansUnrelatedPagesAndUsesEarliestFailure(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	start, end := now.Add(-4*24*time.Hour).Unix(), now.Add(20*24*time.Hour).Unix()
	first, second := now.Add(-50*time.Hour).Unix(), now.Add(-8*time.Hour).Unix()
	const subscription, item = "sub_current", "si_current"
	invoiceIDs := []string{"in_first", "in_latest"}
	lineSets := map[string][]map[string]any{}
	for _, id := range invoiceIDs {
		lineSets[id] = []map[string]any{recoveryLine(id, "il_"+id, subscription, item, "price_builder", start, end, false)}
	}
	events := make([]map[string]any, 0, 407)
	for i := 0; i < 405; i++ {
		created := now.Add(-time.Duration(i+1) * time.Minute).Unix()
		events = append(events, recoverySubscriptionEvent(fmt.Sprintf("evt_unrelated_%04d", i), "customer.subscription.created", "cus_unrelated", "sub_unrelated_"+fmt.Sprint(i), "si_unrelated_"+fmt.Sprint(i), "price_builder", "active", "", created, start, end))
	}
	events = append(events, recoveryInvoiceEvent("evt_first", "in_first", recoveryTestCustomer, subscription, item, "price_builder", first, start, end))
	events = append(events, recoveryInvoiceEvent("evt_latest", "in_latest", recoveryTestCustomer, subscription, item, "price_builder", second, start, end))
	sort.Slice(events, func(i, j int) bool { return events[i]["created"].(int64) > events[j]["created"].(int64) })
	env := newRecoveryTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/subscriptions":
			recoveryWriteJSON(w, recoverySubscriptionList(recoveryTestCustomer, "past_due", start, end, subscription, item, "price_builder"))
		case "/v1/events":
			types := r.URL.Query()["types[]"]
			if fmt.Sprint(types) != fmt.Sprint([]string{"customer.subscription.created", "customer.subscription.updated", "invoice.payment_failed"}) {
				t.Errorf("types[] query=%v", types)
			}
			recoveryEventPage(w, r, events)
		case "/v1/invoices/in_first", "/v1/invoices/in_first/lines":
			recoveryInvoiceHandler(w, r, "in_first", recoveryTestCustomer, lineSets["in_first"])
		case "/v1/invoices/in_latest", "/v1/invoices/in_latest/lines":
			recoveryInvoiceHandler(w, r, "in_latest", recoveryTestCustomer, lineSets["in_latest"])
		default:
			http.NotFound(w, r)
		}
	}, testRecoveryLimits())
	seedRecoveryLocalEvidence(t, env)
	before := recoverySQLSnapshot(t, env.db.DB())
	got, err := env.observer.ObserveRestorePending(context.Background(), env.accountID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Eligible || got.PlanID != "builder" || got.GraceUntil.Unix() != first+72*60*60 || !got.GraceUntil.After(got.ObservedAt) {
		t.Fatalf("latest-invoice regression or bad grace: %#v", got)
	}
	if after := recoverySQLSnapshot(t, env.db.DB()); fmt.Sprint(before) != fmt.Sprint(after) {
		t.Fatal("past_due observation changed relevant SQL state")
	}
	requests := env.roundTripper.snapshot()
	eventPages := 0
	startingAfter := []string{}
	for _, request := range requests {
		if request.URL.Path == "/v1/events" {
			eventPages++
			startingAfter = append(startingAfter, request.URL.Query().Get("starting_after"))
		}
	}
	if eventPages != 5 || len(requests) != 1+5+4 {
		t.Fatalf("GET/page counts: %d requests, %d event pages", len(requests), eventPages)
	}
	if startingAfter[0] != "" || startingAfter[1] == "" || startingAfter[4] == "" {
		t.Fatalf("cursor did not advance: %v", startingAfter)
	}
}

func TestRecoveryPastDueEpisodeResetUsesNextFailure(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	start, end := now.Add(-5*24*time.Hour).Unix(), now.Add(18*24*time.Hour).Unix()
	fail1, reset, fail2 := now.Add(-60*time.Hour).Unix(), now.Add(-30*time.Hour).Unix(), now.Add(-10*time.Hour).Unix()
	const sub, item = "sub_reset", "si_reset"
	lines := map[string][]map[string]any{}
	events := []map[string]any{
		recoveryInvoiceEvent("evt_fail1", "in_fail1", recoveryTestCustomer, sub, item, "price_builder", fail1, start, end),
		recoverySubscriptionEvent("evt_reset", "customer.subscription.updated", recoveryTestCustomer, sub, item, "price_builder", "active", "past_due", reset, start, end),
		recoveryInvoiceEvent("evt_fail2", "in_fail2", recoveryTestCustomer, sub, item, "price_builder", fail2, start, end),
	}
	for _, id := range []string{"in_fail1", "in_fail2"} {
		lines[id] = []map[string]any{recoveryLine(id, "il_"+id, sub, item, "price_builder", start, end, false)}
	}
	sort.Slice(events, func(i, j int) bool { return events[i]["created"].(int64) > events[j]["created"].(int64) })
	env := newRecoveryTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/subscriptions":
			recoveryWriteJSON(w, recoverySubscriptionList(recoveryTestCustomer, "past_due", start, end, sub, item, "price_builder"))
		case "/v1/events":
			recoveryEventPage(w, r, events)
		case "/v1/invoices/in_fail1", "/v1/invoices/in_fail1/lines":
			recoveryInvoiceHandler(w, r, "in_fail1", recoveryTestCustomer, lines["in_fail1"])
		case "/v1/invoices/in_fail2", "/v1/invoices/in_fail2/lines":
			recoveryInvoiceHandler(w, r, "in_fail2", recoveryTestCustomer, lines["in_fail2"])
		default:
			http.NotFound(w, r)
		}
	}, testRecoveryLimits())
	got, err := env.observer.ObserveRestorePending(context.Background(), env.accountID)
	if err != nil {
		t.Fatal(err)
	}
	if got.GraceUntil.Unix() != fail2+72*60*60 {
		t.Fatalf("episode reset did not anchor first post-reset failure: %#v", got)
	}
}

func TestRecoveryPastDueRefusesConflictingSameSecondTransitions(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	start, end := now.Add(-5*24*time.Hour).Unix(), now.Add(18*24*time.Hour).Unix()
	fail1, transition, fail2 := now.Add(-60*time.Hour).Unix(), now.Add(-30*time.Hour).Unix(), now.Add(-10*time.Hour).Unix()
	const sub, item = "sub_tie", "si_tie"
	lines := map[string][]map[string]any{}
	events := []map[string]any{
		recoveryInvoiceEvent("evt_tie_fail1", "in_tie_fail1", recoveryTestCustomer, sub, item, "price_builder", fail1, start, end),
		recoverySubscriptionEvent("evt_tie_active", "customer.subscription.updated", recoveryTestCustomer, sub, item, "price_builder", "active", "past_due", transition, start, end),
		recoverySubscriptionEvent("evt_tie_past_due", "customer.subscription.updated", recoveryTestCustomer, sub, item, "price_builder", "past_due", "active", transition, start, end),
		recoveryInvoiceEvent("evt_tie_fail2", "in_tie_fail2", recoveryTestCustomer, sub, item, "price_builder", fail2, start, end),
	}
	for _, id := range []string{"in_tie_fail1", "in_tie_fail2"} {
		lines[id] = []map[string]any{recoveryLine(id, "il_"+id, sub, item, "price_builder", start, end, false)}
	}
	sort.Slice(events, func(i, j int) bool { return events[i]["created"].(int64) > events[j]["created"].(int64) })
	env := newRecoveryTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/subscriptions":
			recoveryWriteJSON(w, recoverySubscriptionList(recoveryTestCustomer, "past_due", start, end, sub, item, "price_builder"))
		case "/v1/events":
			recoveryEventPage(w, r, events)
		case "/v1/invoices/in_tie_fail1", "/v1/invoices/in_tie_fail1/lines":
			recoveryInvoiceHandler(w, r, "in_tie_fail1", recoveryTestCustomer, lines["in_tie_fail1"])
		case "/v1/invoices/in_tie_fail2", "/v1/invoices/in_tie_fail2/lines":
			recoveryInvoiceHandler(w, r, "in_tie_fail2", recoveryTestCustomer, lines["in_tie_fail2"])
		default:
			http.NotFound(w, r)
		}
	}, testRecoveryLimits())
	got, err := env.observer.ObserveRestorePending(context.Background(), env.accountID)
	assertZeroObservation(t, got)
	if !errors.Is(err, contracts.ErrBillingProviderAmbiguous) {
		t.Fatalf("conflicting same-second transitions should refuse, got %v", err)
	}
}

func TestRecoveryPastDueRefusesFailureTiedToNonresetStatusTransition(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	start, end := now.Add(-4*24*time.Hour).Unix(), now.Add(18*24*time.Hour).Unix()
	firstFailure, tie := now.Add(-2*time.Hour).Unix(), now.Add(-time.Hour).Unix()
	const sub, item = "sub_transition_failure_tie", "si_transition_failure_tie"
	ids := []string{"in_before_tie", "in_tied_failure"}
	events := []map[string]any{
		recoveryInvoiceEvent("evt_before_tie", ids[0], recoveryTestCustomer, sub, item, "price_builder", firstFailure, start, end),
		recoveryInvoiceEvent("evt_tied_failure", ids[1], recoveryTestCustomer, sub, item, "price_builder", tie, start, end),
		recoverySubscriptionEvent("evt_nonreset_transition", "customer.subscription.updated", recoveryTestCustomer, sub, item, "price_builder", "past_due", "active", tie, start, end),
	}
	lines := recoveryTestInvoiceLines(ids, sub, item, start, end)
	got, err := observeRecoveryPastDueEvents(t, sub, item, start, end, events, lines)
	assertZeroObservation(t, got)
	if !errors.Is(err, contracts.ErrBillingProviderAmbiguous) {
		t.Fatalf("failure tied to a status transition should refuse, got %v", err)
	}
}

func TestRecoveryPastDueAcceptsDuplicateIdenticalSameSecondTransition(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	start, end := now.Add(-4*24*time.Hour).Unix(), now.Add(18*24*time.Hour).Unix()
	firstFailure, reset, secondFailure := now.Add(-60*time.Hour).Unix(), now.Add(-30*time.Hour).Unix(), now.Add(-10*time.Hour).Unix()
	const sub, item = "sub_duplicate_transition", "si_duplicate_transition"
	ids := []string{"in_duplicate_before", "in_duplicate_after"}
	events := []map[string]any{
		recoveryInvoiceEvent("evt_duplicate_before", ids[0], recoveryTestCustomer, sub, item, "price_builder", firstFailure, start, end),
		recoverySubscriptionEvent("evt_duplicate_reset_a", "customer.subscription.updated", recoveryTestCustomer, sub, item, "price_builder", "active", "past_due", reset, start, end),
		recoverySubscriptionEvent("evt_duplicate_reset_b", "customer.subscription.updated", recoveryTestCustomer, sub, item, "price_builder", "active", "past_due", reset, start, end),
		recoveryInvoiceEvent("evt_duplicate_after", ids[1], recoveryTestCustomer, sub, item, "price_builder", secondFailure, start, end),
	}
	lines := recoveryTestInvoiceLines(ids, sub, item, start, end)
	got, err := observeRecoveryPastDueEvents(t, sub, item, start, end, events, lines)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Eligible || got.GraceUntil.Unix() != secondFailure+72*60*60 {
		t.Fatalf("identical duplicate transition should preserve episode reset: %#v", got)
	}
}

func recoveryTestInvoiceLines(ids []string, sub, item string, start, end int64) map[string][]map[string]any {
	lines := make(map[string][]map[string]any, len(ids))
	for _, id := range ids {
		lines[id] = []map[string]any{recoveryLine(id, "il_"+id, sub, item, "price_builder", start, end, false)}
	}
	return lines
}

func observeRecoveryPastDueEvents(t *testing.T, sub, item string, start, end int64, events []map[string]any, lines map[string][]map[string]any) (contracts.RecoveryBillingObservation, error) {
	t.Helper()
	sort.Slice(events, func(i, j int) bool { return events[i]["created"].(int64) > events[j]["created"].(int64) })
	env := newRecoveryTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/subscriptions":
			recoveryWriteJSON(w, recoverySubscriptionList(recoveryTestCustomer, "past_due", start, end, sub, item, "price_builder"))
		case "/v1/events":
			recoveryEventPage(w, r, events)
		default:
			for id, invoiceLines := range lines {
				if r.URL.Path == "/v1/invoices/"+id || r.URL.Path == "/v1/invoices/"+id+"/lines" {
					recoveryInvoiceHandler(w, r, id, recoveryTestCustomer, invoiceLines)
					return
				}
			}
			http.NotFound(w, r)
		}
	}, testRecoveryLimits())
	return env.observer.ObserveRestorePending(context.Background(), env.accountID)
}

func TestRecoveryPastDueRefusesTruncatedHistoryAndRequestBudget(t *testing.T) {
	now := time.Now().UTC()
	start, end := now.Add(-3*24*time.Hour).Unix(), now.Add(20*24*time.Hour).Unix()
	for _, tc := range []struct {
		name   string
		limits RecoveryObserverLimits
		events map[string]any
	}{
		{"event-page-cap", func() RecoveryObserverLimits { x := testRecoveryLimits(); x.EventPages = 1; return x }(), map[string]any{"object": "list", "data": []any{recoverySubscriptionEvent("evt_a", "customer.subscription.created", "cus_else", "sub_else", "si_else", "price_builder", "active", "", now.Unix()-100, start, end)}, "has_more": true}},
		{"global-get-cap", func() RecoveryObserverLimits { x := testRecoveryLimits(); x.ProviderGETs = 1; return x }(), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newRecoveryTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/subscriptions" {
					recoveryWriteJSON(w, recoverySubscriptionList(recoveryTestCustomer, "past_due", start, end, "sub_budget", "si_budget", "price_builder"))
					return
				}
				if r.URL.Path == "/v1/events" {
					if tc.events == nil {
						recoveryWriteJSON(w, map[string]any{"object": "list", "data": []any{}, "has_more": false})
					} else {
						_ = json.NewEncoder(w).Encode(tc.events)
					}
					return
				}
				http.NotFound(w, r)
			}, tc.limits)
			got, err := env.observer.ObserveRestorePending(context.Background(), env.accountID)
			assertZeroObservation(t, got)
			if !errors.Is(err, contracts.ErrBillingProviderAmbiguous) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestRecoveryObserverNeverFollowsRedirect(t *testing.T) {
	var target atomic.Int32
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { target.Add(1) }))
	defer redirectTarget.Close()
	env := newRecoveryTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", redirectTarget.URL)
		w.WriteHeader(http.StatusFound)
	}, testRecoveryLimits())
	got, err := env.observer.ObserveRestorePending(context.Background(), env.accountID)
	assertZeroObservation(t, got)
	if !errors.Is(err, contracts.ErrBillingProviderUnavailable) {
		t.Fatalf("got %v", err)
	}
	if target.Load() != 0 {
		t.Fatal("redirect target received a request")
	}
	var roundTrips atomic.Int32
	redirectingClient := *env.observer.client
	redirectingClient.Transport = roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		roundTrips.Add(1)
		if request.URL.Host != "api.stripe.com" {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"object":"x"}`)), Request: request}, nil
		}
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{redirectTarget.URL}}, Body: http.NoBody, Request: request}, nil
	})
	env.observer.client = &redirectingClient
	work := &recoveryObservationWork{observer: env.observer, ctx: context.Background()}
	if _, err := work.get("/redirect"); !errors.Is(err, contracts.ErrBillingProviderUnavailable) {
		t.Fatalf("redirect response was followed or misclassified: %v", err)
	}
	if target.Load() != 0 {
		t.Fatalf("redirect target received %d requests", target.Load())
	}
	if got := roundTrips.Load(); got != 1 {
		t.Fatalf("redirect caused %d provider round trips, want one", got)
	}
}

func TestRecoveryObserverEnforcesResponseAndAggregateByteCeilings(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limits RecoveryObserverLimits
		body   []byte
	}{
		{"response-valid-prefix-extra", func() RecoveryObserverLimits { x := testRecoveryLimits(); x.ResponseBytes = 64; return x }(), []byte(`{"object":"list","data":[],"has_more":false,"metadata":"` + strings.Repeat("x", 100) + `"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newRecoveryTestEnv(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(tc.body) }, tc.limits)
			got, err := env.observer.ObserveRestorePending(context.Background(), env.accountID)
			assertZeroObservation(t, got)
			if !errors.Is(err, contracts.ErrBillingProviderAmbiguous) {
				t.Fatalf("got %v", err)
			}
		})
	}
	limits := testRecoveryLimits()
	limits.ResponseBytes = 16
	body := append([]byte(`{"object":"x"}`), bytes.Repeat([]byte{' '}, 20)...)
	env := newRecoveryTestEnv(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }, limits)
	work := &recoveryObservationWork{observer: env.observer, ctx: context.Background()}
	if _, err := work.get("/valid-json-plus-whitespace"); !errors.Is(err, contracts.ErrBillingProviderAmbiguous) {
		t.Fatalf("oversized valid JSON prefix was accepted: %v", err)
	}
	limits = testRecoveryLimits()
	limits.ResponseBytes = 32
	limits.TotalResponseBytes = 20
	env = newRecoveryTestEnv(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{"object":"x"}`) }, limits)
	work = &recoveryObservationWork{observer: env.observer, ctx: context.Background()}
	if _, err := work.get("/one"); err != nil {
		t.Fatalf("first bounded response: %v", err)
	}
	if _, err := work.get("/two"); !errors.Is(err, contracts.ErrBillingProviderAmbiguous) {
		t.Fatalf("aggregate ceiling did not reject second response: %v", err)
	}
}

func TestRecoveryObserverContextCancellationInterruptsInFlightGET(t *testing.T) {
	started := make(chan struct{})
	released := make(chan struct{})
	env := newRecoveryTestEnv(t, func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(released) }, testRecoveryLimits())
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, err := env.observer.ObserveRestorePending(ctx, env.accountID); result <- err }()
	<-started
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("observer did not stop after cancellation")
	}
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("HTTP handler remained blocked")
	}
}

func TestRecoveryPastDueRefusesCreatedResetTiedToConflictingUpdate(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	start, end := now.Add(-4*24*time.Hour).Unix(), now.Add(18*24*time.Hour).Unix()
	firstFailure, conflict, secondFailure := now.Add(-2*time.Hour).Unix(), now.Add(-time.Hour).Unix(), now.Add(-30*time.Minute).Unix()
	const sub, item = "sub_created_update_tie", "si_created_update_tie"
	ids := []string{"in_before_created_update", "in_after_created_update"}
	events := []map[string]any{
		recoveryInvoiceEvent("evt_before_created_update", ids[0], recoveryTestCustomer, sub, item, "price_builder", firstFailure, start, end),
		recoverySubscriptionEvent("evt_created_active", "customer.subscription.created", recoveryTestCustomer, sub, item, "price_builder", "active", "", conflict, start, end),
		recoverySubscriptionEvent("evt_updated_past_due", "customer.subscription.updated", recoveryTestCustomer, sub, item, "price_builder", "past_due", "active", conflict, start, end),
		recoveryInvoiceEvent("evt_after_created_update", ids[1], recoveryTestCustomer, sub, item, "price_builder", secondFailure, start, end),
	}
	got, err := observeRecoveryPastDueEvents(t, sub, item, start, end, events, recoveryTestInvoiceLines(ids, sub, item, start, end))
	assertZeroObservation(t, got)
	if !errors.Is(err, contracts.ErrBillingProviderAmbiguous) {
		t.Fatalf("creation reset tied to conflicting update should refuse, got observation=%#v err=%v", got, err)
	}
}

func TestRecoveryPastDueAcceptsCreatedAndUpdatedSameResetOutcome(t *testing.T) {
	for _, status := range []string{"active", "trialing"} {
		t.Run(status, func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Second)
			start, end := now.Add(-4*24*time.Hour).Unix(), now.Add(18*24*time.Hour).Unix()
			before, reset, after := now.Add(-2*time.Hour).Unix(), now.Add(-time.Hour).Unix(), now.Add(-30*time.Minute).Unix()
			const sub, item = "sub_same_reset_outcome", "si_same_reset_outcome"
			ids := []string{"in_before_same_reset", "in_after_same_reset"}
			events := []map[string]any{
				recoveryInvoiceEvent("evt_before_same_reset", ids[0], recoveryTestCustomer, sub, item, "price_builder", before, start, end),
				recoverySubscriptionEvent("evt_created_same_reset", "customer.subscription.created", recoveryTestCustomer, sub, item, "price_builder", status, "", reset, start, end),
				recoverySubscriptionEvent("evt_updated_same_reset", "customer.subscription.updated", recoveryTestCustomer, sub, item, "price_builder", status, "past_due", reset, start, end),
				recoveryInvoiceEvent("evt_after_same_reset", ids[1], recoveryTestCustomer, sub, item, "price_builder", after, start, end),
			}
			got, err := observeRecoveryPastDueEvents(t, sub, item, start, end, events, recoveryTestInvoiceLines(ids, sub, item, start, end))
			if err != nil {
				t.Fatal(err)
			}
			if !got.Eligible || got.GraceUntil.Unix() != after+72*60*60 {
				t.Fatalf("same reset outcome should preserve post-reset failure anchor: %#v", got)
			}
		})
	}
}
