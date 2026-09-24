package backup

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/store"
)

// fakeJournal is a test-only DeletionJournal fixture. It never fabricates
// emptiness silently: ReadThrough reports exactly the watermark it was
// constructed with, standing in for "the journal, when actually asked,
// reports this position" -- including the legitimate all-zero "actually
// empty" case.
type fakeJournal struct {
	watermark contracts.DeletionWatermark
	readErr   error
}

func (f fakeJournal) AppendDeletion(context.Context, contracts.DeletionEntry) (contracts.DeletionEntry, error) {
	return contracts.DeletionEntry{}, errors.New("fakeJournal: AppendDeletion not implemented by this fixture")
}
func (f fakeJournal) ReadThrough(_ context.Context, _ contracts.DeletionWatermark) (contracts.DeletionRead, error) {
	if f.readErr != nil {
		return contracts.DeletionRead{}, f.readErr
	}
	return contracts.DeletionRead{To: f.watermark}, nil
}
func (f fakeJournal) Seal(context.Context, int64) (contracts.DeletionWatermark, error) {
	return contracts.DeletionWatermark{}, errors.New("fakeJournal: Seal not implemented by this fixture")
}

func testWatermark() contracts.DeletionWatermark {
	return contracts.DeletionWatermark{Generation: 1, SequenceID: 3, EntryHash: strings.Repeat("a", 64)}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// newReadyBrainDir creates a brain directory with real canonical content: an
// initialized .git repository with one committed file, matching what
// pool.Acquire produces the first time a brain is actually opened.
func newReadyBrainDir(t *testing.T, dataDir, id string) {
	t.Helper()
	root := filepath.Join(dataDir, "brains", id)
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--quiet", "--initial-branch=main")
	runGit(t, root, "config", "user.name", "Serenity Hosted")
	runGit(t, root, "config", "user.email", "hosted@serenity.sire.run")
	if err := os.WriteFile(filepath.Join(root, "brain.txt"), []byte("hello\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "--quiet", "-m", "Initialize hosted brain")
}

// newTestDataDir builds a fresh hosted data directory with one account and
// brainCount ready brains, each with real canonical content, and returns
// their IDs in ascending order (the order Create is expected to emit them).
func newTestDataDir(t *testing.T, brainCount int) (dataDir string, brainIDs []string) {
	t.Helper()
	dataDir = t.TempDir()
	db, err := store.Open(filepath.Join(dataDir, controlDBName))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	acct, err := db.CreateAccount(ctx, "user-"+store.ID()+"@example.com")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < brainCount; i++ {
		id := store.ID()
		// store.InsertBrain always defaults is_default to 1, and only one
		// default-active brain per account is allowed; insert directly so
		// multiple brains can coexist on the one test account (mirroring
		// provision.Additional's is_default=0 for every brain past the first).
		isDefault := 0
		if i == 0 {
			isDefault = 1
		}
		if _, err = db.DB().ExecContext(ctx, `INSERT INTO brains(id,account_id,path_key,state,is_default,created_at) VALUES(?,?,?,'ready',?,?)`, id, acct.ID, id, isDefault, store.Stamp(time.Now())); err != nil {
			t.Fatal(err)
		}
		newReadyBrainDir(t, dataDir, id)
		brainIDs = append(brainIDs, id)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(brainIDs)
	return dataDir, brainIDs
}

func liveSchemaVersion(t *testing.T, dataDir string) int {
	t.Helper()
	db, err := store.Open(filepath.Join(dataDir, controlDBName))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var v int
	if err = db.DB().QueryRow(`SELECT max(version) FROM schema_migrations`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func readManifestFile(t *testing.T, dir string) contracts.ManifestV2 {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var m contracts.ManifestV2
	if err = json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func writeManifestFile(t *testing.T, dir string, m contracts.ManifestV2) {
	t.Helper()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, manifestFile), data, 0600); err != nil {
		t.Fatal(err)
	}
}

// freshSnapshot runs a real Create and returns the published snapshot
// directory alongside the source dataDir and sorted brain IDs, for tests
// that tamper with the published artifacts afterward.
func freshSnapshot(t *testing.T, brainCount int) (dataDir, snapshot string, brainIDs []string) {
	t.Helper()
	dataDir, brainIDs = newTestDataDir(t, brainCount)
	snapshot = filepath.Join(t.TempDir(), "snapshot")
	if err := Create(context.Background(), dataDir, snapshot, "test-build-sha", fakeJournal{watermark: testWatermark()}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return dataDir, snapshot, brainIDs
}

func TestCreateRestoreRoundTrip(t *testing.T) {
	dataDir, snapshot, brainIDs := freshSnapshot(t, 2)

	manifest := readManifestFile(t, snapshot)
	if manifest.Version != 2 {
		t.Fatalf("manifest version = %d, want 2", manifest.Version)
	}
	if manifest.Source.BuildSHA != "test-build-sha" {
		t.Fatalf("build sha = %q", manifest.Source.BuildSHA)
	}
	if want := liveSchemaVersion(t, dataDir); manifest.Source.SchemaVersion != want {
		t.Fatalf("schema version = %d, want %d", manifest.Source.SchemaVersion, want)
	}
	if manifest.JournalWatermark != testWatermark() {
		t.Fatalf("journal watermark = %+v, want %+v", manifest.JournalWatermark, testWatermark())
	}
	if len(manifest.Brains) != len(brainIDs) {
		t.Fatalf("manifest brains = %d, want %d", len(manifest.Brains), len(brainIDs))
	}
	for i, id := range brainIDs {
		b := manifest.Brains[i]
		if b.ID != id || b.Empty {
			t.Fatalf("brains[%d] = %+v, want id %s non-empty", i, b, id)
		}
		if !hasHeadRef(b.Heads, "refs/heads/main") || !hasHeadRef(b.Heads, "HEAD") {
			t.Fatalf("brains[%d].Heads = %+v, want HEAD and refs/heads/main", i, b.Heads)
		}
	}

	restored := filepath.Join(t.TempDir(), "restored")
	if err := Restore(context.Background(), snapshot, restored); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	db, err := store.Open(filepath.Join(restored, controlDBName))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var status, planID string
	if err = db.DB().QueryRow(`SELECT status,plan_id FROM accounts`).Scan(&status, &planID); err != nil {
		t.Fatal(err)
	}
	if status != "restore_pending" || planID != "free" {
		t.Fatalf("account status=%q plan=%q, want restore_pending/free", status, planID)
	}
	for _, id := range brainIDs {
		content, err := os.ReadFile(filepath.Join(restored, "brains", id, "brain.txt"))
		if err != nil {
			t.Fatalf("brain %s: %v", id, err)
		}
		if string(content) != "hello\n" {
			t.Fatalf("brain %s content = %q", id, content)
		}
		heads, err := localHeads(context.Background(), filepath.Join(restored, "brains", id))
		if err != nil {
			t.Fatal(err)
		}
		if !hasHeadRef(heads, "refs/heads/main") || !hasHeadRef(heads, "HEAD") {
			t.Fatalf("restored brain %s heads = %+v", id, heads)
		}
	}
}

// TestRoundTripPreservesEveryBranchAndTag proves the restore path restores
// every ref a bundle carries, not only the default branch a plain `git
// clone` checks out locally. A brain's canonical repository is never
// expected to hold more than one branch today (pool.Acquire only ever
// creates "main"), but ManifestV2.BundleHead's own Ref field is documented
// to allow any "refs/..." name, and a restore that silently dropped
// secondary branches into remote-tracking refs instead of real local
// branches would be exactly the "incomplete valid-backup" case reported
// only as a quiet success. This exercises that general case directly rather
// than only ever testing the single-branch shape the product happens to
// produce today.
func TestRoundTripPreservesEveryBranchAndTag(t *testing.T) {
	dataDir, brainIDs := newTestDataDir(t, 1)
	id := brainIDs[0]
	root := filepath.Join(dataDir, "brains", id)
	runGit(t, root, "checkout", "-b", "feature", "--quiet")
	if err := os.WriteFile(filepath.Join(root, "feature.txt"), []byte("feature work\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "--quiet", "-m", "feature work")
	runGit(t, root, "checkout", "main", "--quiet")
	runGit(t, root, "tag", "v1")

	snapshot := filepath.Join(t.TempDir(), "snapshot")
	if err := Create(context.Background(), dataDir, snapshot, "test-build-sha", fakeJournal{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	manifest := readManifestFile(t, snapshot)
	for _, want := range []string{"HEAD", "refs/heads/main", "refs/heads/feature", "refs/tags/v1"} {
		if !hasHeadRef(manifest.Brains[0].Heads, want) {
			t.Fatalf("manifest heads = %+v, missing %s", manifest.Brains[0].Heads, want)
		}
	}

	restored := filepath.Join(t.TempDir(), "restored")
	if err := Restore(context.Background(), snapshot, restored); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	restoredRoot := filepath.Join(restored, "brains", id)
	for _, want := range []string{"HEAD", "refs/heads/main", "refs/heads/feature", "refs/tags/v1"} {
		if !hasHeadRef(mustLocalHeads(t, restoredRoot), want) {
			t.Fatalf("restored heads = %+v, missing %s", mustLocalHeads(t, restoredRoot), want)
		}
	}
	featureContent, err := exec.Command("git", "-C", restoredRoot, "show", "refs/heads/feature:feature.txt").Output()
	if err != nil {
		t.Fatalf("restored feature branch content: %v", err)
	}
	if string(featureContent) != "feature work\n" {
		t.Fatalf("restored feature branch content = %q", featureContent)
	}
}

func mustLocalHeads(t *testing.T, dir string) []contracts.BundleHead {
	t.Helper()
	heads, err := localHeads(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return heads
}

func TestCreateFlushesDirtyCanonicalStateBeforeBundling(t *testing.T) {
	dataDir, brainIDs := newTestDataDir(t, 1)
	id := brainIDs[0]
	root := filepath.Join(dataDir, "brains", id)
	if err := os.WriteFile(filepath.Join(root, "brain.txt"), []byte("uncommitted change\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Deliberately left uncommitted: `git bundle create --all` alone would
	// silently exclude this. Create must flush it into a commit instead.
	status, err := exec.Command("git", "-C", root, "status", "--porcelain").Output()
	if err != nil || len(strings.TrimSpace(string(status))) == 0 {
		t.Fatalf("test setup: expected a dirty working tree, got %q err=%v", status, err)
	}

	snapshot := filepath.Join(t.TempDir(), "snapshot")
	if err = Create(context.Background(), dataDir, snapshot, "test-build-sha", fakeJournal{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	restored := filepath.Join(t.TempDir(), "restored")
	if err = Restore(context.Background(), snapshot, restored); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(restored, "brains", id, "brain.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "uncommitted change\n" {
		t.Fatalf("restored content = %q, want the flushed dirty write preserved", content)
	}
	if remaining, e := exec.Command("git", "-C", root, "status", "--porcelain").Output(); e != nil || len(strings.TrimSpace(string(remaining))) != 0 {
		t.Fatalf("source working tree should be clean after flush, got %q err=%v", remaining, e)
	}
}

func TestCreateRejectsReadyBrainMissingGit(t *testing.T) {
	dataDir, brainIDs := newTestDataDir(t, 1)
	id := brainIDs[0]
	root := filepath.Join(dataDir, "brains", id)
	if err := os.RemoveAll(filepath.Join(root, ".git")); err != nil {
		t.Fatal(err)
	}
	// The directory still holds other real content (brain.txt survives),
	// which must not be treated as "never touched" just because .git is gone.
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	err := Create(context.Background(), dataDir, snapshot, "test-build-sha", fakeJournal{})
	if err == nil {
		t.Fatal("expected Create to fail for a ready brain missing its Git repository")
	}
	if !strings.Contains(err.Error(), "no canonical git repository") {
		t.Fatalf("error = %v, want a corruption message naming the missing repository", err)
	}
	if _, statErr := os.Stat(snapshot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("destination must not exist after a failed Create, stat err = %v", statErr)
	}
}

func TestCreateRejectsReadyBrainWithEmptyDirectoryToo(t *testing.T) {
	// An emptied directory is not proof a brain was never used: it must fail
	// exactly like a brain with other surviving content and no .git.
	dataDir, brainIDs := newTestDataDir(t, 1)
	id := brainIDs[0]
	root := filepath.Join(dataDir, "brains", id)
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	err := Create(context.Background(), dataDir, snapshot, "test-build-sha", fakeJournal{})
	if err == nil {
		t.Fatal("expected Create to fail for a ready brain with an empty directory and no Git repository")
	}
	if _, statErr := os.Stat(snapshot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("destination must not exist after a failed Create, stat err = %v", statErr)
	}
}

func TestCreateRequiresJournal(t *testing.T) {
	dataDir, _ := newTestDataDir(t, 0)
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	if err := Create(context.Background(), dataDir, snapshot, "sha", nil); err == nil {
		t.Fatal("expected Create to fail closed with a nil journal")
	}
	if _, statErr := os.Stat(snapshot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("destination must not exist when Create fails closed on a missing journal")
	}
}

// TestCreateFailsClosedWithoutBuildIdentity proves Create's fail-closed
// behavior deterministically: a probe binary built with -buildvcs=false (no
// embedded VCS revision at all) and given an explicit empty build-sha must
// refuse Create outright and publish nothing. This test binary's own VCS
// metadata (this environment's go test always embeds one) cannot be used to
// exercise this path, so it drives a purpose-built subprocess instead of
// skipping when the fallback happens to succeed.
func TestCreateFailsClosedWithoutBuildIdentity(t *testing.T) {
	out := filepath.Join(t.TempDir(), "backupprobe-novcs")
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", out, "./testdata/backupprobe")
	cmd.Dir = "."
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build no-vcs probe: %v: %s", err, output)
	}
	base := t.TempDir()
	probe := exec.Command(out, base, "")
	stdout, err := probe.Output()
	if err != nil {
		t.Fatalf("probe should exit 0 after refusing Create, got %v", err)
	}
	if got := string(stdout); got != "create-error\n" {
		t.Fatalf("probe output = %q, want %q (Create must refuse without any build identity)", got, "create-error\n")
	}
	if _, statErr := os.Stat(filepath.Join(base, "snapshot")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("destination must not exist after Create refuses for lack of build identity, stat err = %v", statErr)
	}
}

func TestCreateRefusesExistingDestination(t *testing.T) {
	dataDir, _ := newTestDataDir(t, 0)
	dest := t.TempDir() // already exists
	err := Create(context.Background(), dataDir, dest, "sha", fakeJournal{})
	if err == nil {
		t.Fatal("expected Create to refuse an existing destination")
	}
}

func TestRestoreRefusesExistingDestination(t *testing.T) {
	_, snapshot, _ := freshSnapshot(t, 0)
	dest := t.TempDir() // already exists
	if err := Restore(context.Background(), snapshot, dest); err == nil {
		t.Fatal("expected Restore to refuse an existing destination")
	}
}

func TestRestoreRejectsLegacyManifest(t *testing.T) {
	_, snapshot, _ := freshSnapshot(t, 0)
	if err := os.WriteFile(filepath.Join(snapshot, manifestFile), []byte(`{"version":1,"created_at":"2020-01-01T00:00:00Z","brains":[]}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "restored")
	err := Restore(context.Background(), snapshot, restored)
	if !errors.Is(err, ErrLegacySnapshot) {
		t.Fatalf("err = %v, want ErrLegacySnapshot", err)
	}
	if _, statErr := os.Stat(restored); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("destination must not exist after refusing a legacy manifest")
	}
}

func TestRestoreRejectsTruncatedManifest(t *testing.T) {
	_, snapshot, _ := freshSnapshot(t, 0)
	if err := os.WriteFile(filepath.Join(snapshot, manifestFile), []byte(`{"version":2,`), 0600); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "restored")
	if err := Restore(context.Background(), snapshot, restored); err == nil {
		t.Fatal("expected Restore to reject a truncated manifest")
	}
}

func TestRestoreRejectsTamperedControlDBChecksum(t *testing.T) {
	_, snapshot, _ := freshSnapshot(t, 1)
	corrupt(t, filepath.Join(snapshot, controlDBName))
	restored := filepath.Join(t.TempDir(), "restored")
	err := Restore(context.Background(), snapshot, restored)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v, want a control database checksum mismatch", err)
	}
	if _, statErr := os.Stat(restored); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("destination must not exist after a checksum failure")
	}
}

func TestRestoreRejectsTruncatedControlDB(t *testing.T) {
	_, snapshot, _ := freshSnapshot(t, 1)
	truncate(t, filepath.Join(snapshot, controlDBName))
	restored := filepath.Join(t.TempDir(), "restored")
	if err := Restore(context.Background(), snapshot, restored); err == nil {
		t.Fatal("expected Restore to reject a truncated control database")
	}
	if _, statErr := os.Stat(restored); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("destination must not exist after a truncation failure")
	}
}

func TestRestoreRejectsTamperedBundle(t *testing.T) {
	_, snapshot, brainIDs := freshSnapshot(t, 1)
	corrupt(t, filepath.Join(snapshot, brainIDs[0]+".bundle"))
	restored := filepath.Join(t.TempDir(), "restored")
	err := Restore(context.Background(), snapshot, restored)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v, want a bundle checksum mismatch", err)
	}
	if _, statErr := os.Stat(restored); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("destination must not exist after a bundle checksum failure")
	}
}

func TestRestoreRejectsTamperedBundleHeadInManifest(t *testing.T) {
	_, snapshot, brainIDs := freshSnapshot(t, 1)
	m := readManifestFile(t, snapshot)
	// Recompute the checksum over the untouched bundle bytes so only the
	// recorded head, not the artifact reference, is wrong.
	sum, size, err := sha256File(filepath.Join(snapshot, brainIDs[0]+".bundle"))
	if err != nil {
		t.Fatal(err)
	}
	m.Brains[0].SHA256 = sum
	m.Brains[0].LengthBytes = size
	m.Brains[0].Heads[0].ObjectID = strings.Repeat("f", 40)
	writeManifestFile(t, snapshot, m)
	restored := filepath.Join(t.TempDir(), "restored")
	err = Restore(context.Background(), snapshot, restored)
	if err == nil || !strings.Contains(err.Error(), "heads do not match") {
		t.Fatalf("err = %v, want a bundle-heads mismatch", err)
	}
	if _, statErr := os.Stat(restored); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("destination must not exist after a heads mismatch")
	}
}

func TestRestoreRejectsUnknownBrainInManifest(t *testing.T) {
	_, snapshot, brainIDs := freshSnapshot(t, 2)
	m := readManifestFile(t, snapshot)
	fakeID := store.ID()
	// Point the renamed entry at the same real bundle bytes so only the
	// inventory cross-check, not the checksum, is exercised. Re-sort by ID
	// afterward so the manifest's own ascending-order structural check
	// (ManifestV2.Validate) still passes and only the inventory
	// cross-reference against the database is exercised.
	m.Brains[0].ID = fakeID
	sort.Slice(m.Brains, func(i, j int) bool { return m.Brains[i].ID < m.Brains[j].ID })
	writeManifestFile(t, snapshot, m)
	restored := filepath.Join(t.TempDir(), "restored")
	err := Restore(context.Background(), snapshot, restored)
	if !errors.Is(err, ErrManifestInventoryMismatch) {
		t.Fatalf("err = %v, want ErrManifestInventoryMismatch", err)
	}
	_ = brainIDs
	if _, statErr := os.Stat(restored); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("destination must not exist after an inventory mismatch")
	}
}

func TestRestoreRejectsOmittedBrainInManifest(t *testing.T) {
	_, snapshot, _ := freshSnapshot(t, 2)
	m := readManifestFile(t, snapshot)
	m.Brains = m.Brains[:1]
	writeManifestFile(t, snapshot, m)
	restored := filepath.Join(t.TempDir(), "restored")
	err := Restore(context.Background(), snapshot, restored)
	if !errors.Is(err, ErrManifestInventoryMismatch) {
		t.Fatalf("err = %v, want ErrManifestInventoryMismatch", err)
	}
	if _, statErr := os.Stat(restored); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("destination must not exist after an inventory mismatch")
	}
}

func TestRestoreRejectsSchemaMismatch(t *testing.T) {
	_, snapshot, _ := freshSnapshot(t, 0)
	m := readManifestFile(t, snapshot)
	m.Source.SchemaVersion++ // control.db bytes are untouched; only the claim is wrong
	writeManifestFile(t, snapshot, m)
	restored := filepath.Join(t.TempDir(), "restored")
	err := Restore(context.Background(), snapshot, restored)
	if !errors.Is(err, ErrSchemaMismatch) {
		t.Fatalf("err = %v, want ErrSchemaMismatch", err)
	}
	if _, statErr := os.Stat(restored); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("destination must not exist after a schema mismatch")
	}
}

func TestRestoreRejectsSymlinkedArtifact(t *testing.T) {
	_, snapshot, _ := freshSnapshot(t, 0)
	real := filepath.Join(snapshot, controlDBName)
	elsewhere := filepath.Join(t.TempDir(), "elsewhere.db")
	data, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(elsewhere, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(real); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(elsewhere, real); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "restored")
	err = Restore(context.Background(), snapshot, restored)
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("err = %v, want a symlinked-artifact refusal", err)
	}
	if _, statErr := os.Stat(restored); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("destination must not exist after a symlinked-artifact refusal")
	}
}

func TestRestoreRejectsSymlinkedManifest(t *testing.T) {
	_, snapshot, _ := freshSnapshot(t, 0)
	real := filepath.Join(snapshot, manifestFile)
	elsewhere := filepath.Join(t.TempDir(), "elsewhere.json")
	data, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(elsewhere, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(real); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(elsewhere, real); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "restored")
	err = Restore(context.Background(), snapshot, restored)
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("err = %v, want a symlinked-manifest refusal", err)
	}
	if _, statErr := os.Stat(restored); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("destination must not exist after a symlinked-manifest refusal")
	}
}

func TestRestoreRevokesCredentialsAndOAuthState(t *testing.T) {
	dataDir, snapshot, brainIDs := freshSnapshot(t, 1)
	db, err := store.Open(filepath.Join(dataDir, controlDBName))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var accountID string
	if err = db.DB().QueryRowContext(ctx, `SELECT id FROM accounts`).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	if err = db.InsertCredential(ctx, store.ClientCredential{
		ID: store.ID(), BrainID: brainIDs[0], AccountID: accountID,
		Prefix: store.ID(), Verifier: store.Hash("secret"), Scopes: []string{"memory:read"},
		Generation: 1, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	// This snapshot predates the credential; a second, later snapshot is what
	// a real backup after credential issuance would look like. Take one now.
	snapshot2 := filepath.Join(t.TempDir(), "snapshot2")
	if err = Create(ctx, dataDir, snapshot2, "test-build-sha", fakeJournal{}); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "restored")
	if err = Restore(ctx, snapshot2, restored); err != nil {
		t.Fatal(err)
	}
	restoredDB, err := store.Open(filepath.Join(restored, controlDBName))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restoredDB.Close() }()
	var revokedAt *string
	if err = restoredDB.DB().QueryRow(`SELECT revoked_at FROM client_credentials`).Scan(&revokedAt); err != nil {
		t.Fatal(err)
	}
	if revokedAt == nil {
		t.Fatal("restored credential must be revoked")
	}
	_ = snapshot
}

func hasHeadRef(heads []contracts.BundleHead, ref string) bool {
	for _, h := range heads {
		if h.Ref == ref {
			return true
		}
	}
	return false
}

func corrupt(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatalf("cannot corrupt empty file %s", path)
	}
	data[len(data)/2] ^= 0xFF
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func truncate(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 2 {
		t.Fatalf("cannot truncate short file %s", path)
	}
	if err = os.WriteFile(path, data[:len(data)/2], 0600); err != nil {
		t.Fatal(err)
	}
}
