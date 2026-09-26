package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/backup"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/provision"
	"github.com/sirerun/serenity/internal/hosted/service"
	"github.com/sirerun/serenity/internal/hosted/store"
	"github.com/sirerun/serenity/internal/server/mcp"
	brainstore "github.com/sirerun/serenity/internal/store"
)

type sender struct{ link string }

func (s *sender) Send(_ context.Context, _, link string) error { s.link = link; return nil }

type embedding struct{}

func (embedding) ModelVersion() string { return "test@v1" }
func (embedding) Embed(_ context.Context, text string) ([]float32, error) {
	return []float32{1, float32(len(text)%7 + 1), 1}, nil
}
func TestHostedJourneyAndIsolation(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "brains"), 0700); err != nil {
		t.Fatal(err)
	}
	var svc *service.Service
	var server *httptest.Server
	mail := &sender{}
	start := func() {
		db, err := store.Open(filepath.Join(dir, "control.db"))
		if err != nil {
			t.Fatal(err)
		}
		cfg := service.Config{DataDir: dir, MaxOpen: 2, MaxInFlight: 8, PublicOrigin: "http://127.0.0.1", AccountCap: 100}
		svc, err = service.Assemble(cfg, true, db, mail, embedding{})
		if err != nil {
			t.Fatal(err)
		}
		server = httptest.NewServer(svc.Handler)
	}
	start()
	defer func() {
		server.Close()
		if err := svc.Close(); err != nil {
			t.Error(err)
		}
	}()
	newClient := func() *http.Client { jar, _ := cookiejar.New(nil); return &http.Client{Jar: jar} }
	login := func(email string) (*http.Client, string, string) {
		c := newClient()
		resp, e := c.PostForm(server.URL+"/login", url.Values{"email": {email}})
		if e != nil {
			t.Fatal(e)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("login %d %s", resp.StatusCode, body)
		}
		link, _ := url.Parse(mail.link)
		resp, e = c.Get(server.URL + link.RequestURI())
		if e != nil {
			t.Fatal(e)
		}
		body, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("consume %d %s", resp.StatusCode, body)
		}
		csrf := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindSubmatch(body)
		if len(csrf) != 2 {
			t.Fatalf("missing CSRF: %s", body)
		}
		resp, e = c.PostForm(server.URL+"/credentials", url.Values{"csrf": {string(csrf[1])}})
		if e != nil {
			t.Fatal(e)
		}
		body, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("credential %d %s", resp.StatusCode, body)
		}
		raw := regexp.MustCompile(`sk_live_[a-f0-9]{8}_[A-Za-z0-9_-]{43}`).Find(body)
		if len(raw) == 0 {
			t.Fatalf("no credential %s", body)
		}
		return c, string(raw), string(csrf[1])
	}
	a, tokenA, csrf := login("a@example.com")
	_, tokenB, _ := login("b@example.com")
	request := func(token, session string, payload any) (int, string, []byte) {
		data, _ := json.Marshal(payload)
		r, e := http.NewRequest(http.MethodPost, server.URL+"/mcp", bytes.NewReader(data))
		if e != nil {
			t.Fatal(e)
		}
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		if session != "" {
			r.Header.Set(mcp.SessionIDHeader, session)
			r.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
		}
		resp, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode, resp.Header.Get(mcp.SessionIDHeader), body
	}
	initialize := func(token string) string {
		status, session, body := request(token, "", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": mcp.ProtocolVersion, "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "hosted-test", "version": "1"}}})
		if status != 200 || session == "" {
			t.Fatalf("initialize %d %s", status, body)
		}
		status, _, body = request(token, session, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
		if status != 202 && status != 200 {
			t.Fatalf("initialized %d %s", status, body)
		}
		return session
	}
	sessionA := initialize(tokenA)
	sessionB := initialize(tokenB)
	call := func(token, session, name string, args map[string]any) []byte {
		status, _, body := request(token, session, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
		if status != 200 {
			t.Fatalf("call %s %d %s", name, status, body)
		}
		if bytes.Contains(body, []byte(`"isError":true`)) {
			t.Fatalf("tool error %s", body)
		}
		return body
	}
	status, _, body := request(tokenA, sessionA, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	if status != 200 {
		t.Fatal(string(body))
	}
	var listed struct {
		Result struct{ Tools []struct{ Name string } }
	}
	if e := json.Unmarshal(body, &listed); e != nil {
		t.Fatal(e)
	}
	if len(listed.Result.Tools) != 4 {
		t.Fatalf("tools: %s", body)
	}
	call(tokenA, sessionA, "remember", map[string]any{"fact": "My launch codeword is silver otter", "provenance": "hosted integration test", "operation_key": "first-memory"})
	body = call(tokenA, sessionA, "recall", map[string]any{"query": "silver otter", "limit": 10})
	if !bytes.Contains(body, []byte("silver otter")) {
		t.Fatalf("recall: %s", body)
	}
	body = call(tokenB, sessionB, "recall", map[string]any{"query": "silver otter", "limit": 10})
	if bytes.Contains(body, []byte("My launch codeword")) {
		t.Fatalf("tenant leak %s", body)
	}
	status, _, _ = request(tokenB, sessionA, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/list"})
	if status != 401 {
		t.Fatalf("cross-account session status %d", status)
	}
	resp, e := a.PostForm(server.URL+"/credentials", url.Values{})
	if e != nil {
		t.Fatal(e)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("missing CSRF status %d", resp.StatusCode)
	}
	// A real snapshot contains canonical Git data; restore cannot revive credentials.
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	if e = svc.Backup(context.Background(), snapshot); e != nil {
		t.Fatal(e)
	}
	restored := filepath.Join(t.TempDir(), "restored")
	if e = backup.Restore(context.Background(), snapshot, restored); e != nil {
		t.Fatal(e)
	}
	recovered, e := store.Open(filepath.Join(restored, "control.db"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = (&credential.Issuer{Store: recovered}).Verify(context.Background(), tokenA); e == nil {
		t.Fatal("restore revived old credential")
	}
	binding, e := svc.Gateway.Issuer.Verify(context.Background(), tokenA)
	if e != nil {
		t.Fatal(e)
	}
	projection, e := brainstore.LoadMemoryProjection(brainstore.NewSourceStore(filepath.Join(restored, "brains", binding.BrainID)))
	if e != nil {
		t.Fatal(e)
	}
	if len(projection.All()) != 1 {
		t.Fatalf("restored facts %d", len(projection.All()))
	}
	if e = recovered.Close(); e != nil {
		t.Fatal(e)
	}
	// Restart the real store, writer queues, memory handlers and HTTP sessions.
	server.Close()
	if e = svc.Close(); e != nil {
		t.Fatal(e)
	}
	// Derived data is intentionally absent from backups. Opening must rebuild it.
	if e = os.RemoveAll(filepath.Join(dir, "brains", binding.BrainID, ".serenity")); e != nil {
		t.Fatal(e)
	}
	start()
	sessionA = initialize(tokenA)
	body = call(tokenA, sessionA, "recall", map[string]any{"query": "silver otter", "limit": 10})
	if !bytes.Contains(body, []byte("silver otter")) {
		t.Fatalf("restart recall %s", body)
	}
	// Cookie jars are host-scoped and survive a changed local port.
	resp, e = a.Get(server.URL + "/dashboard/settings")
	if e != nil {
		t.Fatal(e)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	brain := regexp.MustCompile(`<option value="([^"]+)"`).FindSubmatch(body)
	if len(brain) != 2 {
		t.Fatalf("brain missing %s", body)
	}
	resp, e = a.PostForm(server.URL+"/credentials/rotate", url.Values{"csrf": {csrf}, "brain_id": {string(brain[1])}})
	if e != nil {
		t.Fatal(e)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("rotate %d %s", resp.StatusCode, body)
	}
	status, _, _ = request(tokenA, sessionA, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/list"})
	if status != 401 {
		t.Fatalf("revoked established session status %d", status)
	}
	newToken := string(regexp.MustCompile(`sk_live_[a-f0-9]{8}_[A-Za-z0-9_-]{43}`).Find(body))
	resp, e = a.PostForm(server.URL+"/account/delete", url.Values{"csrf": {csrf}, "confirm": {"DELETE"}})
	if e != nil {
		t.Fatal(e)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("delete status %d", resp.StatusCode)
	}
	if _, e = svc.Gateway.Issuer.Verify(context.Background(), newToken); e == nil {
		t.Fatal("deleted account credential works")
	}
	_, freshToken, _ := login("a@example.com")
	freshSession := initialize(freshToken)
	fresh := call(freshToken, freshSession, "recall", map[string]any{"query": "silver otter"})
	if bytes.Contains(fresh, []byte("My launch codeword")) {
		t.Fatal("new signup resurrected deleted memory")
	}
	if strings.Contains(string(body), tokenA) {
		t.Fatal("old secret exposed")
	}
}

func TestInviteOnlyRegistrationConfigWiresThroughAssembly(t *testing.T) {
	dir := t.TempDir()
	mail := &sender{}
	db, err := store.Open(filepath.Join(dir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := service.Assemble(service.Config{
		DataDir:          dir,
		PublicOrigin:     "http://127.0.0.1",
		MaxOpen:          2,
		MaxInFlight:      4,
		AccountCap:       10,
		RegistrationMode: contracts.RegistrationInviteOnly,
		InviteAllowlist:  []string{"allowed@example.com"},
	}, true, db, mail, embedding{})
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	request := func(email string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("email="+url.QueryEscape(email)))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.RemoteAddr = "127.0.0.1:1234"
		w := httptest.NewRecorder()
		svc.Handler.ServeHTTP(w, r)
		return w
	}
	if got := request("blocked@example.com"); got.Code != http.StatusBadRequest {
		t.Fatalf("blocked status=%d, want 400", got.Code)
	}
	if mail.link != "" {
		t.Fatal("blocked invite unexpectedly sent a link")
	}
	if got := request("allowed@example.com"); got.Code != http.StatusOK {
		t.Fatalf("allowed status=%d, want 200", got.Code)
	}
	if mail.link == "" {
		t.Fatal("allowed invite did not send a link")
	}
}

func TestOperationKeysAreBrainScopedAndQuotaIsAccountScoped(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := service.Assemble(service.Config{DataDir: dir, PublicOrigin: "http://127.0.0.1", MaxOpen: 2, MaxInFlight: 4, AccountCap: 100}, true, db, &sender{}, embedding{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := svc.Close(); err != nil {
			t.Error(err)
		}
	}()
	a, err := db.CreateAccount(ctx, "two-brains@example.com")
	if err != nil {
		t.Fatal(err)
	}
	p := &provision.Provisioner{Store: db, BrainsRoot: filepath.Join(dir, "brains")}
	first, err := p.Provision(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.Additional(ctx, a.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	badArgs, _ := json.Marshal(map[string]string{"fact": "valid fact", "provenance": "test", "operation_key": strings.Repeat("x", 129)})
	rejected, err := svc.Gateway.CallForAccount(ctx, a.ID, first.ID, "remember", badArgs)
	if err != nil || !rejected.IsError {
		t.Fatalf("oversized operation key %+v %v", rejected, err)
	}
	var reservations int
	if err = db.DB().QueryRowContext(ctx, `SELECT count(*) FROM reservations WHERE account_id=?`, a.ID).Scan(&reservations); err != nil || reservations != 0 {
		t.Fatalf("invalid key persisted: %d %v", reservations, err)
	}
	for _, id := range []string{first.ID, second.ID, first.ID} {
		args := json.RawMessage(`{"fact":"The marker is amber heron","provenance":"quota regression","operation_key":"same-key"}`)
		result, err := svc.Gateway.CallForAccount(ctx, a.ID, id, "remember", args)
		if err != nil || result.IsError {
			t.Fatalf("remember %+v %v", result, err)
		}
	}
	for _, id := range []string{first.ID, second.ID} {
		output, err := exec.Command("git", "-C", filepath.Join(dir, "brains", id), "status", "--porcelain", "--untracked-files=all").CombinedOutput()
		if err != nil || len(bytes.TrimSpace(output)) != 0 {
			t.Fatalf("acknowledged write not committed: %s %v", output, err)
		}
	}
	inventory, err := svc.Gateway.Inventory(ctx, a.ID, filepath.Join(dir, "brains"))
	if err != nil || inventory.Brains != 2 || inventory.Memories != 2 || inventory.StorageBytes <= 0 {
		t.Fatalf("inventory %+v %v", inventory, err)
	}
	var writes int
	if err = db.DB().QueryRowContext(ctx, `SELECT sum(committed) FROM usage_windows WHERE account_id=? AND metric='writes'`, a.ID).Scan(&writes); err != nil || writes != 2 {
		t.Fatalf("writes=%d err=%v", writes, err)
	}
}
