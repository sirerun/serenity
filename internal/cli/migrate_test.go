package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/index"
)

// TestMigrateModelsRequiresModelsFlag and TestMigrateModelsRequiresAPin
// pin the command's own argument validation -- a migrate invocation with
// no migration mode, or a --models invocation naming no new pin, is a
// user error reported immediately, never a silent no-op.
func TestMigrateModelsRequiresModelsFlag(t *testing.T) {
	root := newInitializedRoot(t)
	cmd := newRootCmd()
	cmd.SetArgs([]string{"-C", root, "migrate", "--embedding", "new@v1"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error when --models is not passed")
	}
	if !strings.Contains(err.Error(), "--models") {
		t.Fatalf("error = %q, want it to mention --models", err.Error())
	}
}

func TestMigrateModelsRequiresAPin(t *testing.T) {
	root := newInitializedRoot(t)
	cmd := newRootCmd()
	cmd.SetArgs([]string{"-C", root, "migrate", "--models"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error when neither --embedding nor --extraction is passed")
	}
	if !strings.Contains(err.Error(), "--embedding") || !strings.Contains(err.Error(), "--extraction") {
		t.Fatalf("error = %q, want it to name both --embedding and --extraction", err.Error())
	}
}

// newInitializedRoot scaffolds a bare `serenity init`-ed brain repo, no
// connectors, no models pinned -- the minimal fixture the flag-validation
// tests need before they ever reach runMigrateModels.
func newInitializedRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	var out bytes.Buffer
	if err := runInit(root, &out); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestMigrateModelsEndToEnd is T1.16's acc line, run over the same real
// net/http fixture pattern TestSyncExtractEndToEnd (T1.15) uses: a fixture
// brain is synced and extracted under an initial embedding pin, then
// `migrate --models --embedding <new pin>` moves it to a second pin.
// Asserts, in order, every clause of the acc line:
//   - changing the embedding pin flags chunks pending_reembed (reported
//     count equals the fixture's own chunk count, since the new pin has
//     never embedded anything);
//   - old vectors are not searched under the new pin (SearchVectors for
//     the old pin returns nothing once migration completes; the new pin's
//     vectors are what's actually there);
//   - no claim is rewritten in place (the claims section of the dump is
//     byte-identical before and after the migration -- migrate never
//     touches brain/, so Rebuild re-derives the exact same claims);
//   - after completion, the rebuild-identity test is green under the new
//     pin (wipe .serenity/, resync, byte-identical dump).
func TestMigrateModelsEndToEnd(t *testing.T) {
	requireGit(t)
	ctx := context.Background()

	extServer := fakeExtractionServer(t)
	embServer := fakeEmbeddingsServer(t)
	t.Setenv("OPENAI_BASE_URL", extServer.URL)
	t.Setenv("OPENAI_EMBEDDINGS_BASE_URL", embServer.URL)
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	root := t.TempDir()
	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatal(err)
	}
	configureGitIdentity(t, root)

	dropDir := t.TempDir()
	notePath := filepath.Join(dropDir, "note.txt")
	if err := os.WriteFile(notePath, []byte("Alice works at Acme Corp."), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-10 * time.Second)
	if err := os.Chtimes(notePath, old, old); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(root, config.FileName)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	const oldPin = "test-embed@v1"
	const newPin = "test-embed@v2"
	cfg.Models.Extraction = "test-extract@v1"
	cfg.Models.Embedding = oldPin
	cfg.Connectors = map[string]any{
		"file": map[string]any{"path": dropDir},
	}
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatal(err)
	}

	var syncOut, extractOut bytes.Buffer
	if err := runSync(ctx, root, &syncOut); err != nil {
		t.Fatalf("sync: %v\noutput:\n%s", err, syncOut.String())
	}
	if err := runExtract(ctx, root, &extractOut); err != nil {
		t.Fatalf("extract: %v\noutput:\n%s", err, extractOut.String())
	}

	readEngine := func() (*index.SQLite, map[string]int64, string) {
		t.Helper()
		eng, err := openIndex(root)
		if err != nil {
			t.Fatal(err)
		}
		stats, err := eng.Stats(ctx)
		if err != nil {
			t.Fatal(err)
		}
		dump, err := index.DumpString(ctx, eng)
		if err != nil {
			t.Fatal(err)
		}
		return eng, stats, dump
	}

	eng, statsBefore, dumpBefore := readEngine()
	if statsBefore["claims"] == 0 || statsBefore["vectors"] == 0 {
		t.Fatalf("fixture did not produce claims/vectors before migration: %+v", statsBefore)
	}
	claimsBefore := claimsSection(t, dumpBefore)
	if claimsBefore == "" {
		t.Fatal("fixture produced no claims section to compare against")
	}
	totalChunks := statsBefore["chunks"]
	_ = eng.Close()

	var migrateOut bytes.Buffer
	if err := runMigrateModels(ctx, root, newPin, "", &migrateOut); err != nil {
		t.Fatalf("migrate --models --embedding %s: %v\noutput:\n%s", newPin, err, migrateOut.String())
	}
	report := migrateOut.String()
	if !strings.Contains(report, "pending_reembed") {
		t.Fatalf("migrate output does not mention pending_reembed:\n%s", report)
	}
	wantPending := strconv.FormatInt(totalChunks, 10)
	if !strings.Contains(report, wantPending+" chunk(s) pending_reembed") {
		t.Fatalf("migrate output = %q, want it to report %s chunk(s) pending_reembed (every chunk, since %s has never embedded anything)",
			report, wantPending, newPin)
	}

	cfgAfter, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfgAfter.Models.Embedding != newPin {
		t.Fatalf("serenity.yml models.embedding = %q, want %q", cfgAfter.Models.Embedding, newPin)
	}

	eng2, statsAfter, dumpAfter := readEngine()
	if statsAfter["vectors"] == 0 {
		t.Fatalf("expected vectors after migration, stats: %+v", statsAfter)
	}
	claimsAfter := claimsSection(t, dumpAfter)
	if claimsAfter != claimsBefore {
		t.Fatalf("claims were rewritten by migrate --models\n--- before ---\n%s\n--- after ---\n%s", claimsBefore, claimsAfter)
	}

	oldPinHits, err := eng2.SearchVectors(ctx, oldPin, []float32{0, 0, 0}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(oldPinHits) != 0 {
		t.Fatalf("SearchVectors(%s) returned %d hit(s) after migrating to %s, want 0 -- old vectors must not be searched", oldPin, len(oldPinHits), newPin)
	}
	newPinHits, err := eng2.SearchVectors(ctx, newPin, []float32{0, 0, 0}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(newPinHits) == 0 {
		t.Fatal("SearchVectors(new pin) returned 0 hits after migration -- reembed did not run")
	}
	_ = eng2.Close()

	// Rebuild-identity, now under the new pin: wipe .serenity/ and resync;
	// the dump (claims, chunks, and the new pin's vectors) must reproduce
	// byte for byte, exactly as T1.10/T1.15 require for an unchanged pin --
	// the migration is complete, so the pinned set is unchanged from here.
	if err := os.RemoveAll(filepath.Join(root, ".serenity")); err != nil {
		t.Fatal(err)
	}
	var syncOut2, extractOut2 bytes.Buffer
	if err := runSync(ctx, root, &syncOut2); err != nil {
		t.Fatalf("post-migration sync: %v\noutput:\n%s", err, syncOut2.String())
	}
	if err := runExtract(ctx, root, &extractOut2); err != nil {
		t.Fatalf("post-migration extract: %v\noutput:\n%s", err, extractOut2.String())
	}
	_, _, dumpRebuilt := readEngine()
	if dumpRebuilt != dumpAfter {
		t.Fatalf("rebuild-identity under the new pin failed after wiping .serenity/\n--- after migration ---\n%s\n--- after wipe+resync ---\n%s", dumpAfter, dumpRebuilt)
	}
}

// TestMigrateModelsExtractionOnlyDoesNotTouchVectors covers the
// extraction-pin path: the pin is recorded and the gap is disclosed, but
// no re-embed runs (nothing changed on the embedding side) and no claim
// is rewritten (migrate never calls the claim-writing path at all).
func TestMigrateModelsExtractionOnlyDoesNotTouchVectors(t *testing.T) {
	requireGit(t)
	ctx := context.Background()

	root := t.TempDir()
	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatal(err)
	}
	configureGitIdentity(t, root)

	cfgPath := filepath.Join(root, config.FileName)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Models.Extraction = "test-extract@v1"
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runMigrateModels(ctx, root, "", "test-extract@v2", &out); err != nil {
		t.Fatalf("migrate --models --extraction: %v\noutput:\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "not yet built") {
		t.Fatalf("migrate output does not disclose the re-extraction gap:\n%s", out.String())
	}
	if strings.Contains(out.String(), "pending_reembed") {
		t.Fatalf("extraction-only migration reported pending_reembed, want it untouched:\n%s", out.String())
	}

	cfgAfter, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfgAfter.Models.Extraction != "test-extract@v2" {
		t.Fatalf("serenity.yml models.extraction = %q, want test-extract@v2", cfgAfter.Models.Extraction)
	}

	eng, err := openIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	stats, err := eng.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats["vectors"] != 0 {
		t.Fatalf("extraction-only migration produced %d vector(s), want 0", stats["vectors"])
	}
}

// claimsSection extracts just the "claims\t..." lines from a
// index.DumpString dump, so a comparison isn't tripped up by the vectors
// section changing (which migrate --models is expected to change) while
// still strictly proving the claims section did not.
func claimsSection(t *testing.T, dump string) string {
	t.Helper()
	var b strings.Builder
	for _, line := range strings.Split(dump, "\n") {
		if strings.HasPrefix(line, "claims\t") {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}
