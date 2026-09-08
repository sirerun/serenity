package importbudget_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/connector"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	budget "github.com/sirerun/serenity/internal/eval/importbudget"
	"github.com/sirerun/serenity/internal/eval/messages"
	"github.com/sirerun/serenity/internal/extract"
	"github.com/sirerun/serenity/internal/extract/chunk"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/ingest"
	"github.com/sirerun/serenity/internal/reconcile"
	"github.com/sirerun/serenity/internal/router"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

const cacheVersion = "synthetic-golden-v1-vector512-" + extract.PromptVersion
const modelPin = "synthetic-extraction@v1"

type noLiveModel struct{ calls int }

func (m *noLiveModel) Complete(context.Context, router.TaskClass, router.Prompt, router.Budget) (router.Result, error) {
	m.calls++
	return router.Result{}, fmt.Errorf("benchmark cache miss: live calls forbidden")
}

type countedCache struct {
	extract.Cache
	hits int
}

func (c *countedCache) Get(ctx context.Context, k extract.CacheKey) (extract.CachedOutput, bool, error) {
	v, ok, err := c.Cache.Get(ctx, k)
	if ok {
		c.hits++
	}
	return v, ok, err
}

type cachedVectors struct{ values map[string][]float32 }

func (c cachedVectors) ModelVersion() string { return "synthetic-embedding512@v1" }
func (c cachedVectors) Embed(_ context.Context, text string) ([]float32, error) {
	v, ok := c.values[text]
	if !ok {
		return nil, fmt.Errorf("embedding cache miss")
	}
	return v, nil
}
func hash(raw []byte) string { s := sha256.Sum256(raw); return hex.EncodeToString(s[:]) }
func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestImportBudgetSmoke(t *testing.T) { runImport(t, 25, "") }
func TestImportBudget10K(t *testing.T) {
	output := os.Getenv("SERENITY_IMPORT_BUDGET_OUTPUT")
	if output == "" {
		t.Skip("10K performance run requires SERENITY_IMPORT_BUDGET_OUTPUT; smoke run executes separately")
	}
	runImport(t, 10000, output)
}

func runImport(t *testing.T, count int, output string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Hour)
	defer cancel()
	workspace := t.TempDir()
	corpus := filepath.Join(workspace, "corpus")
	manifest, err := messages.Generate(ctx, corpus, messages.Options{Count: count, Seed: 1, MessagesPerFile: 1000})
	if err != nil {
		t.Fatal(err)
	}
	rawManifest, err := os.ReadFile(filepath.Join(corpus, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := mailbox(t, corpus, manifest)
	// Populate immutable golden cache files before measurement. This is synthetic
	// candidate fixture data, not evidence of live-model extraction quality.
	warm, _, err := c.Poll(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(warm) != count {
		t.Fatal("incomplete cache source set")
	}
	cache := &countedCache{Cache: extract.NewFileCache(filepath.Join(workspace, "cache"))}
	preference := regexp.MustCompile(`Synthetic Person ([0-9]{3}) prefers (.+) for Project ([0-9]{2})\.`)
	for _, item := range warm {
		chunks := chunk.Split(string(item.Bytes), chunk.DefaultConfig)
		if len(chunks) != 1 {
			t.Fatalf("fixture changed chunk count: %d", len(chunks))
		}
		match := preference.FindStringSubmatch(chunks[0].Text)
		if len(match) != 4 {
			t.Fatal("fixture has no golden preference")
		}
		candidate := extract.Candidate{Subject: "person-" + match[1], Predicate: "prefers", Object: strings.ReplaceAll(match[2], " ", "-") + "-project-" + match[3], Confidence: 0.9}
		if err := cache.Put(ctx, extract.CacheKey{ChunkSHA256: hash([]byte(chunks[0].Text)), ModelVersion: modelPin, PromptVersion: extract.PromptVersion}, extract.CachedOutput{Accepted: []extract.Candidate{candidate}}); err != nil {
			t.Fatal(err)
		}
	}
	root := filepath.Join(workspace, "brain")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-q")
	git(t, root, "config", "user.name", "Benchmark")
	git(t, root, "config", "user.email", "benchmark@example.invalid")
	cfg := config.Default()
	// Match the production layout so embedding resolves canonical eligibility
	// against this brain, rather than the unrelated benchmark workspace.
	if err := os.MkdirAll(filepath.Join(root, ".serenity"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".serenity/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	eng, err := index.Open(filepath.Join(root, ".serenity", "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	q := writer.NewQueue(nil)
	defer q.Close()
	q.MarkTouched(filepath.Join(root, ".gitignore"))
	ss := store.NewSourceStore(root)
	report := budget.Report{Version: budget.Version, Revision: git(t, ".", "rev-parse", "HEAD"), Environment: fmt.Sprintf("%s/%s-%s-procs%d", runtime.GOOS, runtime.GOARCH, runtime.Version(), runtime.GOMAXPROCS(0)), CorpusSHA256: hash(rawManifest), CacheVersion: cacheVersion, Messages: count}
	if env := os.Getenv("SERENITY_IMPORT_BUDGET_ENVIRONMENT"); env != "" {
		report.Environment = env
	}
	timed := func(name string, items int, fn func()) {
		start := time.Now()
		fn()
		seconds := time.Since(start).Seconds()
		report.Stages = append(report.Stages, budget.Stage{Name: name, Items: items, Seconds: seconds})
		report.TotalSeconds += seconds
		t.Logf("stage=%s items=%d seconds=%.3f", name, items, seconds)
	}
	var items []connector.RawItem
	timed("poll", count, func() {
		var err error
		items, _, err = connector.Run(ctx, eng, c, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != count {
			t.Fatalf("polled %d/%d", len(items), count)
		}
	})
	sources := make([]domain.Source, 0, count)
	timed("store_sources", count, func() {
		for _, item := range items {
			src, err := c.ToSource(item)
			if err != nil {
				t.Fatal(err)
			}
			src, err = ss.Write(item.Bytes, src)
			if err != nil {
				t.Fatal(err)
			}
			sources = append(sources, src)
			dir := ss.DirFor(src.SHA256)
			q.MarkTouched(filepath.Join(dir, "meta.yaml"))
			q.MarkTouched(filepath.Join(dir, "bytes"))
		}
		if _, err := writer.Flush(q, root); err != nil {
			t.Fatal(err)
		}
	})
	live := &noLiveModel{}
	extractor := extract.New(live, modelPin, cfg.FamilyNames(), cache)
	var observations []domain.Observation
	timed("chunk_extract", count, func() {
		for _, src := range sources {
			raw, _, err := ss.Read(src.SHA256)
			if err != nil {
				t.Fatal(err)
			}
			result, err := extractor.Extract(ctx, src.SHA256, src.IndexOnly, chunk.Split(string(raw), chunk.DefaultConfig), router.Budget{})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Ready) != 1 || len(result.Distill) != 0 || result.Rejected != 0 {
				t.Fatalf("incomplete extraction: %+v", result)
			}
			observations = append(observations, result.Ready...)
		}
	})
	engine := reconcile.NewEngine(disposition.NewStore(eng))
	active := map[string][]domain.Claim{}
	reconciled := 0
	timed("reconcile", count, func() {
		for _, obs := range observations {
			claim := ingest.ClaimFromObservation(obs)
			if _, _, err := engine.Process(ctx, claim, active[claim.SubjectSlug], time.Now()); err != nil {
				t.Fatal(err)
			}
			active[claim.SubjectSlug] = append(active[claim.SubjectSlug], claim)
			reconciled++
		}
	})
	iw := ingest.New(q, store.NewFenceWriter(root), store.NewShardStore(root), cfg)
	timed("write_claims", count, func() {
		stats, err := iw.Write(observations)
		if err != nil {
			t.Fatal(err)
		}
		if stats.Written != count || stats.Skipped != 0 {
			t.Fatalf("claim writes: %+v", stats)
		}
		if _, err := writer.Flush(q, root); err != nil {
			t.Fatal(err)
		}
	})
	timed("index", count, func() {
		if err := index.Rebuild(ctx, root, cfg, eng); err != nil {
			t.Fatal(err)
		}
	})
	// Precomputed synthetic vectors isolate model latency from index/write cost.
	// Preparation is excluded, just like extraction-cache population above.
	chunks, err := eng.AllChunks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	vectors := cachedVectors{values: map[string][]float32{}}
	for _, c := range chunks {
		sum := sha256.Sum256([]byte(c.Text))
		v := make([]float32, 512)
		for i := range v {
			v[i] = float32(int(sum[i%len(sum)])-128) / 128
		}
		vectors.values[c.Text] = v
	}
	timed("embed", len(chunks), func() {
		report.Vectors, err = index.ReembedMissing(ctx, eng, vectors)
		if err != nil {
			t.Fatal(err)
		}
	})
	stats, err := eng.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	report.Claims = int(stats["claims"])
	report.CacheHits = cache.hits
	report.ModelCalls = live.calls
	if reconciled != count || stats["vectors"] != int64(report.Vectors) || report.Vectors != len(chunks) {
		t.Fatalf("incomplete reconcile/index/vector path: reconciled=%d/%d persisted_vectors=%d written_vectors=%d eligible_chunks=%d", reconciled, count, stats["vectors"], report.Vectors, len(chunks))
	}
	stored, err := ss.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != count {
		t.Fatalf("stored %d/%d sources", len(stored), count)
	}
	if dirty := git(t, root, "status", "--porcelain"); dirty != "" {
		t.Fatalf("uncommitted canonical work: %s", dirty)
	}
	if err := budget.Check(report, nil); err != nil {
		t.Fatal(err)
	}
	if output != "" {
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			t.Fatal(err)
		}
		raw, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(output, append(raw, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("messages=%d claims=%d vectors=%d cache_hits=%d model_calls=%d measured_seconds=%.3f", report.Messages, report.Claims, report.Vectors, report.CacheHits, report.ModelCalls, report.TotalSeconds)
}
