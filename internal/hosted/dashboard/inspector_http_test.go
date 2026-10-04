package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/gateway"
	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/pool"
	hoststore "github.com/sirerun/serenity/internal/hosted/store"
	brainstore "github.com/sirerun/serenity/internal/store"
)

type inspectorMailCapture struct{ consumeURL string }

func (m *inspectorMailCapture) Send(_ context.Context, _, body string) error {
	m.consumeURL = body
	return nil
}

type inspectorTestEmbedder struct{ calls atomic.Int32 }

func (e *inspectorTestEmbedder) Embed(context.Context, string) ([]float32, error) {
	e.calls.Add(1)
	return nil, errors.New("unexpected inspector embedding call")
}
func (*inspectorTestEmbedder) ModelVersion() string { return "inspector-http-test" }

type inspectorHTTPFixture struct {
	handler         http.Handler
	store           *hoststore.Store
	pool            *pool.Pool
	embed           *inspectorTestEmbedder
	brains          map[string]string
	tokens          map[string]string
	excludedFactIDs []string
}

func newInspectorHTTPFixture(t *testing.T) *inspectorHTTPFixture {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	st, err := hoststore.Open(filepath.Join(root, "hosted.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("close hosted store: %v", err)
		}
	})
	mail := &inspectorMailCapture{}
	identityService := &identity.Service{Store: st, Sender: mail, Origin: "https://serenity.test"}
	login := func(email string) (string, string) {
		t.Helper()
		if err := identityService.RequestLink(ctx, email, "127.0.0.1"); err != nil {
			t.Fatalf("request sign-in link for %s: %v", email, err)
		}
		parsed, err := url.Parse(mail.consumeURL)
		if err != nil {
			t.Fatalf("parse sign-in URL: %v", err)
		}
		sessionToken, err := identityService.Consume(ctx, parsed.Query().Get("token"))
		if err != nil {
			t.Fatalf("consume sign-in link for %s: %v", email, err)
		}
		session, err := identityService.Session(ctx, sessionToken)
		if err != nil {
			t.Fatalf("read session for %s: %v", email, err)
		}
		return session.AccountID, sessionToken
	}
	issuer := &credential.Issuer{Store: st}
	brainsRoot := filepath.Join(root, "brains")
	if err := os.MkdirAll(brainsRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	brains := map[string]string{}
	tokens := map[string]string{}
	for _, user := range []struct{ name, email, brainID string }{
		{name: "owner", email: "owner@serenity.test", brainID: "BrainOwnerABCDEFGHIJKLMNOP"},
		{name: "other", email: "other@serenity.test", brainID: "BrainOtherABCDEFGHIJKLMNOPQ"},
	} {
		accountID, sessionToken := login(user.email)
		if _, err := st.InsertBrain(ctx, accountID, user.brainID, user.brainID, "ready", time.Now().UTC()); err != nil {
			t.Fatalf("insert %s brain: %v", user.name, err)
		}
		brainRoot := filepath.Join(brainsRoot, user.brainID)
		for _, dir := range []string{"brain/sources", "brain/entities", "brain/claims"} {
			if err := os.MkdirAll(filepath.Join(brainRoot, dir), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		brains[user.name] = brainRoot
		tokens[user.name] = sessionToken
	}
	sources := brainstore.NewSourceStore(brains["owner"])
	writeFact := func(legacyID int64, fact string, visibility brainstore.MemoryVisibility, created time.Time, validUntil *time.Time) string {
		t.Helper()
		source, err := sources.WriteMemoryFact(brainstore.MemoryFactPayload{
			FormatVersion: brainstore.MemoryFactFormatVersion,
			LegacyID:      legacyID,
			Fact:          fact,
			Provenance:    "inspector HTTP fixture",
			Kind:          brainstore.MemoryFactKindFact,
			Visibility:    visibility,
			CreatedAt:     created,
			ValidUntil:    validUntil,
		})
		if err != nil {
			t.Fatalf("write fixture fact %q: %v", fact, err)
		}
		return source.SHA256
	}
	for i := 1; i <= 3; i++ {
		created := time.Date(2025, time.January, i, 12, 0, 0, 0, time.UTC)
		writeFact(int64(i), fmt.Sprintf("sentinel fact %d", i), brainstore.MemoryVisibilityWorld, created, nil)
	}
	now := time.Now().UTC()
	expiredAt := now.Add(-24 * time.Hour)
	privateID := writeFact(4, "private fixture must not appear", brainstore.MemoryVisibilityPrivate, now.Add(-48*time.Hour), nil)
	expiredID := writeFact(5, "expired fixture must not appear", brainstore.MemoryVisibilityWorld, now.Add(-48*time.Hour), &expiredAt)
	deletedID := writeFact(6, "forgotten fixture must not appear", brainstore.MemoryVisibilityWorld, now.Add(-48*time.Hour), nil)
	if _, err := sources.WriteMemoryExpiry(brainstore.MemoryExpiryPayload{
		FormatVersion: brainstore.MemoryFactFormatVersion,
		TargetSHA256:  deletedID,
		ExpiredAt:     now,
	}); err != nil {
		t.Fatalf("write fixture forget event: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(brains["owner"], ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(brains["owner"], ".git", "index"), []byte("fixture git index stays unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	emb := &inspectorTestEmbedder{}
	poolInstance, err := pool.New(pool.Config{MaxOpen: 2, MaxInFlight: 2, BrainsRoot: brainsRoot, Embedder: emb})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := poolInstance.Close(); err != nil {
			t.Errorf("close inspector test pool: %v", err)
		}
	})
	g := &gateway.Gateway{Issuer: issuer, Pool: poolInstance}
	d := &Dashboard{Identity: identityService, Issuer: issuer, Gateway: g, Dev: true}
	return &inspectorHTTPFixture{handler: d.Handler(), store: st, pool: poolInstance, embed: emb, brains: brains, tokens: tokens, excludedFactIDs: []string{privateID, expiredID, deletedID}}
}

func (f *inspectorHTTPFixture) get(t *testing.T, user, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(&http.Cookie{Name: "serenity_session", Value: f.tokens[user]})
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, req)
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	return response
}

func TestInspectorAuthenticatedHTTPRoutesReadOwnedGraphAndDetail(t *testing.T) {
	f := newInspectorHTTPFixture(t)
	before := inspectorTreeDigest(t, f.brains["owner"])
	brainsResponse := f.get(t, "owner", "/api/inspector/v1/brains")
	if brainsResponse.Code != http.StatusOK {
		t.Fatalf("brains status=%d body=%s", brainsResponse.Code, brainsResponse.Body.String())
	}
	var listed inspectorBrainList
	if err := json.Unmarshal(brainsResponse.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Brains) != 1 || listed.Brains[0].Name != "Project "+listed.Brains[0].ID[:8] {
		t.Fatalf("brains response = %#v", listed)
	}

	brainID := listed.Brains[0].ID
	pageResponse := f.get(t, "owner", "/api/inspector/v1/brains/"+brainID+"/graph?limit=2&q=sentinel")
	if pageResponse.Code != http.StatusOK {
		t.Fatalf("graph status=%d body=%s", pageResponse.Code, pageResponse.Body.String())
	}
	var page inspectorGraphPage
	if err := json.Unmarshal(pageResponse.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Version != 1 || page.BrainID != brainID || !page.PrivateExcluded || len(page.Nodes) != 2 || page.TotalMatching != 3 || page.NextCursor == "" {
		t.Fatalf("graph page = %#v", page)
	}
	selectedID := page.Nodes[0].ID
	detailResponse := f.get(t, "owner", "/api/inspector/v1/brains/"+brainID+"/nodes/"+url.PathEscape(selectedID))
	if detailResponse.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", detailResponse.Code, detailResponse.Body.String())
	}
	var detail inspectorNodeDetail
	if err := json.Unmarshal(detailResponse.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Node.ID != selectedID || len(detail.RelatedNodes) == 0 || len(detail.Edges) == 0 {
		t.Fatalf("detail = %#v", detail)
	}
	if after := inspectorTreeDigest(t, f.brains["owner"]); after != before {
		t.Fatalf("inspector GET changed brain files/index: before=%s after=%s", before, after)
	}
	if got := f.embed.calls.Load(); got != 0 {
		t.Fatalf("inspector GET made %d embedding/model calls", got)
	}
}

func TestInspectorHTTPOmitsPrivateExpiredAndForgottenFacts(t *testing.T) {
	f := newInspectorHTTPFixture(t)
	brainID := "BrainOwnerABCDEFGHIJKLMNOP"
	response := f.get(t, "owner", "/api/inspector/v1/brains/"+brainID+"/graph")
	if response.Code != http.StatusOK {
		t.Fatalf("graph status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, forbidden := range []string{"private fixture must not appear", "expired fixture must not appear", "forgotten fixture must not appear"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("private or erased fact leaked in graph response: %q", forbidden)
		}
	}
	if !strings.Contains(body, "sentinel fact 1") {
		t.Fatalf("eligible public fixture absent from graph response: %s", body)
	}
	for _, id := range f.excludedFactIDs {
		guessed := f.get(t, "owner", "/api/inspector/v1/brains/"+brainID+"/nodes/fact:"+id)
		if guessed.Code != http.StatusNotFound {
			t.Errorf("excluded fact %s status=%d body=%s", id, guessed.Code, guessed.Body.String())
		}
	}
}

func TestInspectorHTTPRoutesHideOtherOwnersAndGuessedSources(t *testing.T) {
	f := newInspectorHTTPFixture(t)
	otherBrainID := "BrainOtherABCDEFGHIJKLMNOPQ"
	for _, path := range []string{
		"/api/inspector/v1/brains/" + otherBrainID + "/graph",
		"/api/inspector/v1/brains/" + otherBrainID + "/nodes/fact:0000000000000000000000000000000000000000000000000000000000000000",
	} {
		response := f.get(t, "owner", path)
		if response.Code != http.StatusNotFound {
			t.Errorf("owner request to foreign brain path %q status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	guessed := f.get(t, "owner", "/api/inspector/v1/brains/BrainOwnerABCDEFGHIJKLMNOP/nodes/source:"+strings.Repeat("0", 64))
	if guessed.Code != http.StatusNotFound {
		t.Fatalf("guessed source status=%d body=%s", guessed.Code, guessed.Body.String())
	}
}

func TestInspectorHTTPCursorIsFilterBoundAndPagesAreBounded(t *testing.T) {
	f := newInspectorHTTPFixture(t)
	brainID := "BrainOwnerABCDEFGHIJKLMNOP"
	firstResponse := f.get(t, "owner", "/api/inspector/v1/brains/"+brainID+"/graph?limit=2&q=sentinel")
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("first page status=%d body=%s", firstResponse.Code, firstResponse.Body.String())
	}
	var first inspectorGraphPage
	if err := json.Unmarshal(firstResponse.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Nodes) != 2 || first.TotalMatching != 3 || first.NextCursor == "" {
		t.Fatalf("first page not bounded as expected: %#v", first)
	}
	secondResponse := f.get(t, "owner", "/api/inspector/v1/brains/"+brainID+"/graph?limit=2&q=sentinel&cursor="+url.QueryEscape(first.NextCursor))
	if secondResponse.Code != http.StatusOK {
		t.Fatalf("second page status=%d body=%s", secondResponse.Code, secondResponse.Body.String())
	}
	var second inspectorGraphPage
	if err := json.Unmarshal(secondResponse.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if len(second.Nodes) != 1 || second.NextCursor != "" || second.Nodes[0].ID == first.Nodes[0].ID || second.Nodes[0].ID == first.Nodes[1].ID {
		t.Fatalf("second page not a distinct bounded remainder: %#v", second)
	}
	for _, query := range []string{
		"limit=2&q=other&cursor=" + url.QueryEscape(first.NextCursor),
		"limit=3&q=sentinel&cursor=" + url.QueryEscape(first.NextCursor),
	} {
		response := f.get(t, "owner", "/api/inspector/v1/brains/"+brainID+"/graph?"+query)
		if response.Code != http.StatusBadRequest {
			t.Errorf("filter-bound cursor query %q status=%d body=%s", query, response.Code, response.Body.String())
		}
		var body inspectorErrorEnvelope
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Error.Code != "invalid_cursor" {
			t.Errorf("filter-bound cursor error = %#v, unmarshal err=%v", body, err)
		}
	}
}

func TestInspectorHTTPEmptyBrainReturnsEmptyArrays(t *testing.T) {
	f := newInspectorHTTPFixture(t)
	response := f.get(t, "other", "/api/inspector/v1/brains/BrainOtherABCDEFGHIJKLMNOPQ/graph")
	if response.Code != http.StatusOK {
		t.Fatalf("empty graph status=%d body=%s", response.Code, response.Body.String())
	}
	var page inspectorGraphPage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Nodes == nil || page.Edges == nil || page.TotalMatching != 0 || page.NextCursor != "" {
		t.Fatalf("empty graph response = %#v", page)
	}
}

func inspectorTreeDigest(t *testing.T, root string) string {
	t.Helper()
	var names []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			names = append(names, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprintf(h, "%s\x00", name); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		h.Write([]byte(hex.EncodeToString(digest[:])))
	}
	return hex.EncodeToString(h.Sum(nil))
}
