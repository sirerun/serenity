package index

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

type memoryCallbackEmbedder struct {
	call func(context.Context, string) ([]float32, error)
}

func (*memoryCallbackEmbedder) ModelVersion() string { return "memory-boundary@v1" }
func (e *memoryCallbackEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return e.call(ctx, text)
}

func memorySearchSource(t *testing.T, root string, until *time.Time) string {
	t.Helper()
	src, err := store.NewSourceStore(root).WriteMemoryFact(store.MemoryFactPayload{FormatVersion: 1, RecordType: store.SourceKindMemoryFact, LegacyID: 1, Fact: "egress marker", Provenance: "test", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidUntil: until})
	if err != nil {
		t.Fatal(err)
	}
	return src.SHA256
}

func TestMemorySearchRechecksTTLBeforeAndAfterEmbedding(t *testing.T) {
	for _, during := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_egress", true: "during_provider"}[during], func(t *testing.T) {
			root, eng := sourcePolicyIndex(t)
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			until := now.Add(time.Minute)
			sha := memorySearchSource(t, root, &until)
			clockCalls, calls := 0, 0
			clock := func() time.Time {
				clockCalls++
				if !during && clockCalls > 1 {
					return until
				}
				return now
			}
			e := &memoryCallbackEmbedder{call: func(context.Context, string) ([]float32, error) { calls++; now = until; return []float32{1, 0}, nil }}
			state, expired, err := RefreshMemoryFactSearch(context.Background(), root, sha, eng, e, clock)
			if err != nil || state != "not_eligible" || !expired {
				t.Fatalf("state=%s expired=%v err=%v", state, expired, err)
			}
			if !during && calls != 0 || during && calls != 1 {
				t.Fatalf("provider calls=%d", calls)
			}
			has, err := eng.HasVector(context.Background(), "fact:"+sha, e.ModelVersion())
			if err != nil || has {
				t.Fatal("expired source got a ready vector")
			}
		})
	}
}

func TestMemorySearchProviderCancellationKeepsLexicalState(t *testing.T) {
	root, eng := sourcePolicyIndex(t)
	sha := memorySearchSource(t, root, nil)
	called := false
	e := &memoryCallbackEmbedder{call: func(ctx context.Context, _ string) ([]float32, error) {
		called = true
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	state, expired, err := RefreshMemoryFactSearch(ctx, root, sha, eng, e, time.Now)
	if !called || state != "lexical" || expired || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("state=%s expired=%v called=%v err=%v", state, expired, called, err)
	}
}

func TestMemorySearchIndexOnlyRefusesProvider(t *testing.T) {
	root, eng := sourcePolicyIndex(t)
	sha := memorySearchSource(t, root, nil)
	meta := filepath.Join(store.NewSourceStore(root).DirFor(sha), "meta.yaml")
	file, err := os.OpenFile(meta, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("index_only: true\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	e := &sourceRecordingEmbedder{}
	state, _, err := RefreshMemoryFactSearch(context.Background(), root, sha, eng, e, time.Now)
	if err != nil || state != "not_eligible" || len(e.texts) != 0 {
		t.Fatal("index-only source entered provider", state, err)
	}
}
