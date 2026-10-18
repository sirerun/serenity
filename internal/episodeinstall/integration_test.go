//go:build darwin || linux

package episodeinstall

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

func TestInstalledHostedBrainPreservesLexicalRecallAfterOwnerCloseout(t *testing.T) {
	ctx := context.Background()
	req := prepareFixture(t)
	stable := map[string]os.FileInfo{}
	for _, name := range []string{"writer.lock", "canonical-view.lock"} {
		fi, err := os.Stat(filepath.Join(req.Source, ".serenity", name))
		if err != nil {
			t.Fatal(err)
		}
		stable[name] = fi
	}
	ref, err := Prepare(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	owner := transactionOwner(t, ref)
	result, err := Install(ctx, ref, owner, transactionAdmissionFor(t, ref))
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "installed-marker-held" || !result.ResidualCopies {
		t.Fatal("incorrect install boundary")
	}
	// Actual pool acquisition refuses while the maintenance owner/marker remains.
	runtimePool, err := pool.New(pool.Config{BrainsRoot: filepath.Dir(req.Source), MaxOpen: 1, MaxInFlight: 1, Embedder: prepareEmbedding{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := runtimePool.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, release, err := runtimePool.AcquireCanonical(ctx, req.BrainID); err == nil {
		release()
		t.Fatal("pool crossed maintenance boundary")
	}
	// This synthetic fixture owns closeout. Production cold admission/closeout
	// remains unwired; library success never performs these calls itself.
	if err = owner.Finish(ref.Identity); err != nil {
		t.Fatal(err)
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
	runtime, release, err := runtimePool.AcquireCanonical(ctx, req.BrainID)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := store.LoadEpisodes(ctx, runtime.Sources)
	if err != nil {
		release()
		t.Fatal(err)
	}
	sibling := false
	for _, record := range projection.Records {
		raw := store.EpisodeHash(record)
		if raw == "" {
			release()
			t.Fatal("invalid installed projection")
		}
		if record.Input != nil {
			for _, event := range record.Input.Events {
				if strings.Contains(event.Text, "INSTALL-EXCLUDED-SECRET") {
					release()
					t.Fatal("excluded event resurrected")
				}
				if strings.Contains(event.Text, "independent sibling") {
					sibling = true
				}
			}
			if record.Input.Checkpoint != nil && strings.Contains(record.Input.Checkpoint.Text, "INSTALL-DEPENDENT-SECRET") {
				release()
				t.Fatal("dependent checkpoint resurrected")
			}
		}
	}
	if !sibling || len(projection.Fences) != len(req.CurrentFences) {
		release()
		t.Fatal("retained episode or replay fence lost")
	}
	release()
	if err = runtimePool.DropContext(ctx, req.BrainID); err != nil {
		t.Fatal(err)
	}
	view, err := writer.AcquireCanonicalRead(req.Source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := view.Close(); err != nil {
			t.Error(err)
		}
	}()
	report, err := index.OpenReport(filepath.Join(req.Source, ".serenity", "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := report.Close(); err != nil {
			t.Error(err)
		}
	}()
	hits, err := report.SearchFTS(ctx, "ember", 20)
	if err != nil || len(hits) == 0 {
		t.Fatalf("installed lexical recall lost: %v", err)
	}
	for _, hit := range hits {
		if strings.Contains(hit.Text, "INSTALL-EXCLUDED-SECRET") || strings.Contains(hit.Text, "INSTALL-DEPENDENT-SECRET") {
			t.Fatal("excluded content in installed index")
		}
	}
	for _, pin := range req.IndependentSources {
		raw, err := os.ReadFile(filepath.Join(store.NewSourceStore(req.Source).DirFor(pin.SHA256), "bytes"))
		if err != nil || hashBytes(raw) != pin.SHA256 {
			t.Fatal("independent raw source changed")
		}
	}
	for name, before := range stable {
		after, err := os.Stat(filepath.Join(req.Source, ".serenity", name))
		if err != nil || !os.SameFile(before, after) {
			t.Fatal("stable ownership inode replaced")
		}
	}
	objects := prepareFixtureGit(t, req.Source, "cat-file", "--batch-all-objects", "--batch-check=%(objectname)")
	for _, object := range strings.Fields(string(objects)) {
		raw := prepareFixtureGit(t, req.Source, "cat-file", "-p", object)
		if strings.Contains(string(raw), "INSTALL-EXCLUDED-SECRET") || strings.Contains(string(raw), "INSTALL-DEPENDENT-SECRET") {
			t.Fatal("excluded bytes remain in installed Git")
		}
	}
	if _, err = os.Stat(filepath.Join(ref.Directory, "retained", ".git")); err != nil {
		t.Fatal("retained old history unaccounted for")
	}
}
