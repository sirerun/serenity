package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/store"
)

func TestSearchMemoryLocalPrivacyAndStaleExpiry(t *testing.T) {
	root := t.TempDir()
	if err := config.Default().Save(filepath.Join(root, config.FileName)); err != nil {
		t.Fatal(err)
	}
	ss := store.NewSourceStore(root)
	now := time.Now().UTC()
	var ids []string
	for i, tc := range []struct {
		fact       string
		visibility store.MemoryVisibility
	}{{"publicreviewquery", store.MemoryVisibilityWorld}, {"privatereviewquery", store.MemoryVisibilityPrivate}} {
		src, err := ss.WriteMemoryFact(store.MemoryFactPayload{FormatVersion: 1, RecordType: store.SourceKindMemoryFact, LegacyID: int64(i + 1), Fact: tc.fact, Provenance: "local policy review", Kind: store.MemoryFactKindFact, Visibility: tc.visibility, CreatedAt: now})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, src.SHA256)
	}
	var out bytes.Buffer
	if err := runSync(context.Background(), root, &out); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"publicreviewquery", "privatereviewquery"} {
		results, _, err := searchResults(context.Background(), root, q, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 || results[0].Text != q {
			t.Fatalf("local search cannot retrieve %s: %+v", q, results)
		}
	}
	if _, err := ss.WriteMemoryExpiry(store.MemoryExpiryPayload{FormatVersion: 1, RecordType: store.SourceKindMemoryExpiry, TargetSHA256: ids[0], ExpiredAt: now, Reason: "review"}); err != nil {
		t.Fatal(err)
	}
	// Deliberately no rebuild: canonical expiry must override stale FTS rows.
	results, _, err := searchResults(context.Background(), root, "publicreviewquery", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("stale expired fact returned: %+v", results)
	}
}

// The configured provider is a local test server. Any request is a failure:
// neither raw memory ingress nor index-only bytes may enter extraction.
func TestExtractDoesNotSendMemoryIngressOrIndexOnlySources(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected extraction", http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)
	t.Setenv("OPENAI_BASE_URL", server.URL)
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	root := t.TempDir()
	cfg := config.Default()
	cfg.Models.Provider = ""
	cfg.Models.Extraction = "test-extract@v1"
	if err := cfg.Save(filepath.Join(root, config.FileName)); err != nil {
		t.Fatal(err)
	}
	ss := store.NewSourceStore(root)
	now := time.Now().UTC()
	for i, v := range []store.MemoryVisibility{store.MemoryVisibilityWorld, store.MemoryVisibilityPrivate} {
		if _, err := ss.WriteMemoryFact(store.MemoryFactPayload{FormatVersion: 1, RecordType: store.SourceKindMemoryFact, LegacyID: int64(i + 1), Fact: "RAW-INGRESS-SHOULD-NOT-BE-CLASSIFIED", Provenance: "review", Kind: store.MemoryFactKindFact, Visibility: v, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ss.Write([]byte("INDEXONLY-SHOULD-NOT-EXTRACT"), domain.Source{Kind: "file", URI: "local-only", OccurredAt: now, IndexOnly: true}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runExtract(context.Background(), root, &out); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatalf("restricted raw source reached provider %d times", calls.Load())
	}
}
