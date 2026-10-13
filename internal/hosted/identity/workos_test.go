package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/store"
)

type workOSTransport func(*http.Request) (*http.Response, error)

func (f workOSTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type workOSFixture struct {
	s         *identity.Service
	now       time.Time
	id, email string
	verified  bool
	calls     int
	status    int
}

func newWorkOSFixture(t *testing.T) *workOSFixture {
	t.Helper()
	db, e := store.Open(filepath.Join(t.TempDir(), "control.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := db.Close(); e != nil {
			t.Error(e)
		}
	})
	f := &workOSFixture{now: time.Now().UTC(), id: "user_one", email: "person@example.com", verified: true, status: 200}
	f.s = &identity.Service{Store: db, Origin: "https://serenity.example", Clock: func() time.Time { return f.now }}
	f.s.WorkOS = &identity.WorkOS{ClientID: "client_test", APIKey: "secret-never-logged", HTTPClient: &http.Client{Transport: workOSTransport(func(r *http.Request) (*http.Response, error) {
		f.calls++
		if r.URL.String() != "https://api.workos.com/user_management/authenticate" || r.Method != "POST" {
			t.Errorf("unexpected API target %s %s", r.Method, r.URL)
		}
		var body map[string]string
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Fatal(e)
		}
		if body["client_secret"] != "secret-never-logged" || body["client_id"] != "client_test" || len(body["code_verifier"]) != 43 || body["grant_type"] != "authorization_code" {
			t.Error("invalid exchange payload")
		}
		data, _ := json.Marshal(map[string]any{"user": map[string]any{"id": f.id, "email": f.email, "email_verified": f.verified}})
		return &http.Response{StatusCode: f.status, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}, nil
	})}}
	return f
}
func (f *workOSFixture) begin(t *testing.T, session string) string {
	t.Helper()
	state, location, e := f.s.BeginWorkOS(context.Background(), session, "ip-"+store.ID())
	if e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(location)
	if e != nil {
		t.Fatal(e)
	}
	if u.Host != "api.workos.com" || u.Query().Get("state") != state || u.Query().Get("code_challenge_method") != "S256" || len(u.Query().Get("code_challenge")) != 43 || u.Query().Get("redirect_uri") != "https://serenity.example/login/workos/callback" {
		t.Fatal("incorrect authorization URL")
	}
	if strings.Contains(location, "secret-never-logged") {
		t.Fatal("secret in URL")
	}
	return state
}
func (f *workOSFixture) complete(t *testing.T, state, session string) string {
	t.Helper()
	raw, e := f.s.CompleteWorkOS(context.Background(), state, state, "code", session)
	if e != nil {
		t.Fatal(e)
	}
	return raw
}
func TestWorkOSNewIdentityUsesStableSubjectAndSingleUseState(t *testing.T) {
	f := newWorkOSFixture(t)
	ctx := context.Background()
	state := f.begin(t, "")
	raw := f.complete(t, state, "")
	first, e := f.s.Session(ctx, raw)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.CompleteWorkOS(ctx, state, state, "code", ""); !errors.Is(e, identity.ErrWorkOS) {
		t.Fatalf("replay %v", e)
	}
	if f.calls != 1 {
		t.Fatal("replay reached provider")
	}
	// Provider email changes must never switch local account authority.
	f.email = "changed@example.com"
	raw = f.complete(t, f.begin(t, ""), "")
	second, e := f.s.Session(ctx, raw)
	if e != nil || first.AccountID != second.AccountID {
		t.Fatalf("subject changed account %v", e)
	}
	var email string
	if e = f.s.Store.DB().QueryRow(`SELECT email FROM accounts WHERE id=?`, first.AccountID).Scan(&email); e != nil || email != "person@example.com" {
		t.Fatal("provider rewrote local recovery email")
	}
}
func TestWorkOSExistingAccountRequiresExplicitSameSessionLink(t *testing.T) {
	f := newWorkOSFixture(t)
	ctx := context.Background()
	mail := &sender{}
	f.s.Sender = mail
	if e := f.s.RequestLink(ctx, f.email, "mail-ip"); e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(mail.link)
	session, e := f.s.Consume(ctx, u.Query().Get("token"))
	if e != nil {
		t.Fatal(e)
	}
	original, e := f.s.Session(ctx, session)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Store.DB().Exec(`UPDATE accounts SET stripe_customer_id='cus_preserved',plan_id='builder' WHERE id=?`, original.AccountID); e != nil {
		t.Fatal(e)
	}
	state := f.begin(t, "")
	if _, e = f.s.CompleteWorkOS(ctx, state, state, "code", session); !errors.Is(e, identity.ErrLinkRequired) {
		t.Fatalf("silent email link %v", e)
	}
	state = f.begin(t, session)
	if _, e = f.s.CompleteWorkOS(ctx, state, state, "code", "substituted"); !errors.Is(e, identity.ErrWorkOS) {
		t.Fatalf("session substitution %v", e)
	}
	raw := f.complete(t, f.begin(t, session), session)
	linked, e := f.s.Session(ctx, raw)
	if e != nil || linked.AccountID != original.AccountID {
		t.Fatalf("did not preserve account: %v", e)
	}
	var customer, plan string
	if e = f.s.Store.DB().QueryRow(`SELECT stripe_customer_id,plan_id FROM accounts WHERE id=?`, original.AccountID).Scan(&customer, &plan); e != nil || customer != "cus_preserved" || plan != "builder" {
		t.Fatal("billing changed")
	}
	f.complete(t, f.begin(t, ""), "")
	// Linking to a different verified email never creates a second account.
	f.id = "other-subject"
	f.email = "another@example.com"
	state = f.begin(t, session)
	if _, e = f.s.CompleteWorkOS(ctx, state, state, "code", session); !errors.Is(e, identity.ErrLinkRequired) {
		t.Fatalf("link switched account %v", e)
	}
	var count int
	if e = f.s.Store.DB().QueryRow(`SELECT count(*) FROM accounts`).Scan(&count); e != nil || count != 1 {
		t.Fatal("link created account")
	}
}
func TestWorkOSRejectsInvalidCallbacksBeforeProvider(t *testing.T) {
	for _, kind := range []string{"browser", "expired", "denied", "client"} {
		t.Run(kind, func(t *testing.T) {
			f := newWorkOSFixture(t)
			state := f.begin(t, "")
			browser, code := state, "code"
			switch kind {
			case "browser":
				browser = "wrong"
			case "expired":
				f.now = f.now.Add(11 * time.Minute)
			case "denied":
				code = ""
			case "client":
				f.s.WorkOS.ClientID = "client_other"
			}
			if _, e := f.s.CompleteWorkOS(context.Background(), state, browser, code, ""); !errors.Is(e, identity.ErrWorkOS) {
				t.Fatalf("accepted invalid callback %v", e)
			}
			if f.calls != 0 {
				t.Fatal("invalid callback reached provider")
			}
		})
	}
}
func TestWorkOSRegistrationAndProviderFailures(t *testing.T) {
	for _, kind := range []string{"unverified", "bad-email", "empty-subject", "provider-error", "invite", "cap", "disabled"} {
		t.Run(kind, func(t *testing.T) {
			f := newWorkOSFixture(t)
			ctx := context.Background()
			state := f.begin(t, "")
			switch kind {
			case "unverified":
				f.verified = false
			case "bad-email":
				f.email = "not-email"
			case "empty-subject":
				f.id = ""
			case "provider-error":
				f.status = 500
			case "invite":
				f.s.RegistrationMode = contracts.RegistrationInviteOnly
			case "cap":
				f.s.AccountCap = 1
				if _, e := f.s.Store.CreateAccount(ctx, "other@example.com"); e != nil {
					t.Fatal(e)
				}
			case "disabled":
				f.s.WorkOS = nil
			}
			raw, e := f.s.CompleteWorkOS(ctx, state, state, "code", "")
			if e == nil || raw != "" {
				t.Fatal("invalid auth issued session")
			}
			var count int
			if e = f.s.Store.DB().QueryRow(`SELECT count(*) FROM sessions`).Scan(&count); e != nil || count != 0 {
				t.Fatal("failed login created session")
			}
		})
	}
}
func TestWorkOSDeletedAccountAndRevokedLinkSession(t *testing.T) {
	f := newWorkOSFixture(t)
	ctx := context.Background()
	raw := f.complete(t, f.begin(t, ""), "")
	s, e := f.s.Session(ctx, raw)
	if e != nil {
		t.Fatal(e)
	}
	state := f.begin(t, raw)
	if e = f.s.Logout(ctx, raw); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.CompleteWorkOS(ctx, state, state, "code", raw); !errors.Is(e, identity.ErrLinkRequired) {
		t.Fatalf("revoked linking session %v", e)
	}
	if _, e = f.s.Store.DB().Exec(`UPDATE accounts SET status='deleting' WHERE id=?`, s.AccountID); e != nil {
		t.Fatal(e)
	}
	state = f.begin(t, "")
	if _, e = f.s.CompleteWorkOS(ctx, state, state, "code", ""); !errors.Is(e, identity.ErrAccountUnavailable) {
		t.Fatalf("inactive account %v", e)
	}
}

func TestWorkOSConcurrentCallbackExchangesOnlyOnce(t *testing.T) {
	f := newWorkOSFixture(t)
	state := f.begin(t, "")
	results := make(chan error, 2)
	for range 2 {
		go func() { _, e := f.s.CompleteWorkOS(context.Background(), state, state, "code", ""); results <- e }()
	}
	good := 0
	for range 2 {
		if e := <-results; e == nil {
			good++
		} else if !errors.Is(e, identity.ErrWorkOS) {
			t.Fatal(e)
		}
	}
	if good != 1 || f.calls != 1 {
		t.Fatalf("concurrent callback successes %d provider calls %d", good, f.calls)
	}
}
func TestWorkOSRejectsCrossAccountLink(t *testing.T) {
	f := newWorkOSFixture(t)
	ctx := context.Background()
	first := f.complete(t, f.begin(t, ""), "")
	f.id = "user_second"
	f.email = "second@example.com"
	second := f.complete(t, f.begin(t, ""), "")
	state := f.begin(t, first)
	if _, e := f.s.CompleteWorkOS(ctx, state, state, "code", first); !errors.Is(e, identity.ErrLinkRequired) {
		t.Fatalf("cross-account link accepted %v", e)
	}
	// A different valid browser session is also rejected before exchange.
	state = f.begin(t, first)
	before := f.calls
	if _, e := f.s.CompleteWorkOS(ctx, state, state, "code", second); !errors.Is(e, identity.ErrWorkOS) || f.calls != before {
		t.Fatalf("substituted valid session %v", e)
	}
}
func TestWorkOSPendingCapacityAndExpiry(t *testing.T) {
	f := newWorkOSFixture(t)
	_, e := f.s.Store.DB().Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<10000) INSERT INTO workos_logins(state_hash,verifier,session_hash,client_id,expires_at) SELECT x,'v','','client_test',? FROM n`, store.Stamp(f.now.Add(time.Minute)))
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = f.s.BeginWorkOS(context.Background(), "", "capacity-ip"); !errors.Is(e, identity.ErrRateLimited) {
		t.Fatalf("pending capacity %v", e)
	}
	f.now = f.now.Add(2 * time.Minute)
	f.begin(t, "")
	var n int
	if e = f.s.Store.DB().QueryRow(`SELECT count(*) FROM workos_logins`).Scan(&n); e != nil || n != 1 {
		t.Fatalf("expired cleanup %d %v", n, e)
	}
}

func TestWorkOSExplicitReplacementRevokesBrowserSessions(t *testing.T) {
	f := newWorkOSFixture(t)
	ctx := context.Background()
	first := f.complete(t, f.begin(t, ""), "")
	second := f.complete(t, f.begin(t, ""), "")
	original, e := f.s.Session(ctx, first)
	if e != nil {
		t.Fatal(e)
	}
	// Existing MCP grant authority is independent of browser authentication.
	if _, e = f.s.Store.DB().Exec(`INSERT INTO oauth_grants(id,record,expires_at) VALUES(?,?,?)`, "preserved-grant", "{}", store.Stamp(f.now.Add(time.Hour))); e != nil {
		t.Fatal(e)
	}
	pending := f.begin(t, second)
	f.id = "user_recreated"
	replacement := f.complete(t, f.begin(t, first), first)
	current, e := f.s.Session(ctx, replacement)
	if e != nil || current.AccountID != original.AccountID {
		t.Fatalf("replacement lost account %v", e)
	}
	for _, raw := range []string{first, second} {
		if _, e = f.s.Session(ctx, raw); e == nil {
			t.Fatal("old browser session survived replacement")
		}
	}
	if _, e = f.s.CompleteWorkOS(ctx, pending, pending, "code", second); !errors.Is(e, identity.ErrWorkOS) {
		t.Fatalf("pending old link survived %v", e)
	}
	var n int
	if e = f.s.Store.DB().QueryRow(`SELECT count(*) FROM oauth_grants WHERE id='preserved-grant'`).Scan(&n); e != nil || n != 1 {
		t.Fatal("replacement removed separate MCP grant")
	}
	var subject string
	if e = f.s.Store.DB().QueryRow(`SELECT subject FROM workos_identities WHERE account_id=?`, original.AccountID).Scan(&subject); e != nil || subject != "user_recreated" {
		t.Fatalf("replacement subject %s %v", subject, e)
	}
	f.id = "user_one"
	state := f.begin(t, "")
	if _, e = f.s.CompleteWorkOS(ctx, state, state, "code", ""); !errors.Is(e, identity.ErrLinkRequired) {
		t.Fatalf("old subject still signs in %v", e)
	}
}
