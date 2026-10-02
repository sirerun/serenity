package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/privatefs"
	"github.com/sirerun/serenity/internal/hosted/store"
)

const (
	privateFixtureEnv  = "SERENITY_RECOVERY_TEST_TMPDIR"
	privateFixtureRoot = "/Volumes/SerenityPrivateFixture20261001/tmp"
)

func manifestDigest(t *testing.T, snapshot string) (string, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(snapshot, manifestFile))
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), data
}

func inspectionOptions(t *testing.T, snapshot string) (InspectionOptions, string) {
	t.Helper()
	digest, _ := manifestDigest(t, snapshot)
	scratch := filepath.Join(privateTempDir(t), "inspection scratch ?#")
	if err := os.Mkdir(scratch, 0700); err != nil {
		t.Fatal(err)
	}
	return InspectionOptions{
		ScratchRoot: scratch, ExpectedManifestSHA256: digest,
		MaxDeclaredBytes: 1 << 30, MaxAccounts: 100, MaxBrains: 100,
	}, scratch
}

func privateTempDir(t *testing.T) string {
	t.Helper()
	explicitRoot, explicit := os.LookupEnv(privateFixtureEnv)
	if !explicit && runtime.GOOS != "darwin" {
		dir := t.TempDir()
		if err := privatefs.ValidateDirectory(context.Background(), dir); err != nil {
			t.Fatalf("default test temp directory is not private: %v", err)
		}
		return dir
	}
	root := explicitRoot
	if !explicit {
		root = privateFixtureRoot
	}
	if explicit && (root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root) {
		t.Fatalf("%s must be a nonempty absolute clean directory", privateFixtureEnv)
	}
	if err := privatefs.ValidateDirectory(context.Background(), root); err != nil {
		if explicit {
			t.Fatalf("explicit %s is not private: %v", privateFixtureEnv, err)
		}
		t.Skipf("Darwin ownership-enabled fixture unavailable: %v", err)
	}
	dir, err := os.MkdirTemp(root, "t23-50-inspect-")
	if err != nil {
		if explicit {
			t.Fatalf("create explicit %s fixture: %v", privateFixtureEnv, err)
		}
		t.Skipf("Darwin ownership-enabled fixture unavailable: %v", err)
	}
	if err := privatefs.ValidateDirectory(context.Background(), dir); err != nil {
		_ = os.RemoveAll(dir)
		if explicit {
			t.Fatalf("created fixture under explicit %s is not private: %v", privateFixtureEnv, err)
		}
		t.Skipf("Darwin ownership-enabled fixture unavailable: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestSnapshotExplicitFixtureFailsClosed(t *testing.T) {
	if os.Getenv("SERENITY_TEMP_DIR_PROBE") == "1" {
		_ = privateTempDir(t)
		return
	}
	invalid := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(invalid, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSnapshotExplicitFixtureFailsClosed$")
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		privateFixtureEnv + "=" + invalid,
		"SERENITY_TEMP_DIR_PROBE=1",
	}
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "explicit "+privateFixtureEnv+" is not private") || strings.Contains(string(output), "SKIP") {
		t.Fatalf("invalid explicit fixture result err=%v output=%q; want fatal non-skip", err, output)
	}
}

func freshPrivateSnapshot(t *testing.T, brainCount int) (dataDir, snapshot string, brainIDs []string) {
	t.Helper()
	dataDir, brainIDs = newTestDataDir(t, brainCount)
	snapshot = filepath.Join(privateTempDir(t), "snapshot")
	if err := Create(context.Background(), dataDir, snapshot, "test-build-sha", fakeJournal{watermark: testWatermark()}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return dataDir, snapshot, brainIDs
}

func inspectionIsZero(got SnapshotInspection) bool {
	return got.ManifestSHA256 == "" && got.Source == (contracts.SourceRef{}) &&
		got.JournalWatermark == (contracts.DeletionWatermark{}) && got.Accounts == nil &&
		got.Brains == nil && got.VerifiedArtifactCount == 0 && got.DeclaredArtifactBytes == 0
}

func TestInspectSnapshotVerifiesCreateFixtureWithoutPublishingOrMutatingSource(t *testing.T) {
	_, snapshot, brainIDs := freshPrivateSnapshot(t, 2)
	manifestSHA, _ := manifestDigest(t, snapshot)
	options, scratchRoot := inspectionOptions(t, snapshot)
	before, err := os.ReadDir(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var beforeFiles = make(map[string][32]byte, len(before))
	for _, entry := range before {
		data, readErr := os.ReadFile(filepath.Join(snapshot, entry.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		beforeFiles[entry.Name()] = sha256.Sum256(data)
	}

	got, err := InspectSnapshot(context.Background(), snapshot, options)
	if err != nil {
		t.Fatalf("InspectSnapshot: %v", err)
	}
	if got.ManifestSHA256 != manifestSHA || got.ManifestSHA256 == got.Source.BuildSHA {
		t.Fatalf("manifest and source identities were conflated: %+v", got)
	}
	if got.Source.BuildSHA != "test-build-sha" || got.JournalWatermark != testWatermark() {
		t.Fatalf("source/watermark = %+v / %+v", got.Source, got.JournalWatermark)
	}
	if len(got.Accounts) != 1 || got.Accounts[0].Status != "active" || !safeID(got.Accounts[0].ID) {
		t.Fatalf("accounts = %+v", got.Accounts)
	}
	if len(got.Brains) != len(brainIDs) || got.VerifiedArtifactCount != len(brainIDs)+1 || got.DeclaredArtifactBytes <= 0 {
		t.Fatalf("brain/artifact inventory = %d/%d/%d bytes", len(got.Brains), got.VerifiedArtifactCount, got.DeclaredArtifactBytes)
	}
	for i, id := range brainIDs {
		if got.Brains[i].ID != id || got.Brains[i].Empty || !hasHeadRef(got.Brains[i].Heads, "HEAD") {
			t.Fatalf("brains[%d] = %+v", i, got.Brains[i])
		}
	}
	if entries, err := os.ReadDir(scratchRoot); err != nil || len(entries) != 0 {
		t.Fatalf("scratch root not empty after inspection: entries=%v err=%v", entries, err)
	}
	if _, err := os.Stat(filepath.Join(snapshot, "restore")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspection published a restore destination: %v", err)
	}
	after, err := os.ReadDir(snapshot)
	if err != nil || len(after) != len(beforeFiles) {
		t.Fatalf("snapshot entries changed: before=%d after=%d err=%v", len(beforeFiles), len(after), err)
	}
	for _, entry := range after {
		data, readErr := os.ReadFile(filepath.Join(snapshot, entry.Name()))
		if readErr != nil || sha256.Sum256(data) != beforeFiles[entry.Name()] {
			t.Fatalf("snapshot artifact %s changed: %v", entry.Name(), readErr)
		}
	}
}

func TestInspectSnapshotRejectsWrongDigestBeforeArtifactAccessOrScratchCreation(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 0)
	options, scratchRoot := inspectionOptions(t, snapshot)
	options.ExpectedManifestSHA256 = strings.Repeat("0", 64)
	if err := os.Remove(filepath.Join(snapshot, controlDBName)); err != nil {
		t.Fatal(err)
	}
	got, err := InspectSnapshot(context.Background(), snapshot, options)
	if err == nil || !strings.Contains(err.Error(), "approved digest") || !inspectionIsZero(got) {
		t.Fatalf("inspection = %+v, err=%v; want pre-artifact digest refusal and zero result", got, err)
	}
	if entries, readErr := os.ReadDir(scratchRoot); readErr != nil || len(entries) != 0 {
		t.Fatalf("scratch was created before digest approval: entries=%v err=%v", entries, readErr)
	}
}

func TestInspectSnapshotRejectsTamperedArtifactAndCleansScratch(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 0)
	options, scratchRoot := inspectionOptions(t, snapshot)
	control := filepath.Join(snapshot, controlDBName)
	if err := os.WriteFile(control, []byte("not the approved database"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := InspectSnapshot(context.Background(), snapshot, options)
	if err == nil || !inspectionIsZero(got) {
		t.Fatalf("inspection = %+v, err=%v; want tamper refusal and zero result", got, err)
	}
	if entries, readErr := os.ReadDir(scratchRoot); readErr != nil || len(entries) != 0 {
		t.Fatalf("scratch leaked after tamper refusal: entries=%v err=%v", entries, readErr)
	}
}

func TestInspectSnapshotRefusesBadInventoryAndLimits(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 0)
	options, scratchRoot := inspectionOptions(t, snapshot)
	options.MaxAccounts = 1
	// Add a second account after backup. The control DB still passes its own
	// integrity/schema checks, but the inventory now exceeds this explicit cap.
	db, err := store.Open(filepath.Join(snapshot, controlDBName))
	if err != nil {
		t.Fatal(err)
	}
	_, createErr := db.CreateAccount(context.Background(), "extra-"+store.ID()+"@example.com")
	closeErr := db.Close()
	if createErr != nil || closeErr != nil {
		t.Fatalf("add tampering account: %v / %v", createErr, closeErr)
	}
	manifest := readManifestFile(t, snapshot)
	controlBytes, err := os.ReadFile(filepath.Join(snapshot, controlDBName))
	if err != nil {
		t.Fatal(err)
	}
	manifest.ControlDB.LengthBytes = int64(len(controlBytes))
	controlHash := sha256.Sum256(controlBytes)
	manifest.ControlDB.SHA256 = hex.EncodeToString(controlHash[:])
	writeManifestFile(t, snapshot, manifest)
	options.ExpectedManifestSHA256, _ = manifestDigest(t, snapshot)
	got, err := InspectSnapshot(context.Background(), snapshot, options)
	if err == nil || !inspectionIsZero(got) || !strings.Contains(err.Error(), "inventory exceeds") {
		t.Fatalf("inspection = %+v, err=%v; want capped inventory refusal", got, err)
	}
	if entries, readErr := os.ReadDir(scratchRoot); readErr != nil || len(entries) != 0 {
		t.Fatalf("scratch leaked after inventory refusal: entries=%v err=%v", entries, readErr)
	}
}

func TestInspectSnapshotPreservesDeletionStatusesAsMetadata(t *testing.T) {
	dataDir := t.TempDir()
	db, err := store.Open(filepath.Join(dataDir, controlDBName))
	if err != nil {
		t.Fatal(err)
	}
	want := []SnapshotAccount{}
	for _, status := range []string{"active", "deleted", "deleting", "restore_pending"} {
		account, createErr := db.CreateAccount(context.Background(), "status-"+status+"-"+store.ID()+"@example.com")
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, err = db.DB().Exec(`UPDATE accounts SET status=? WHERE id=?`, status, account.ID); err != nil {
			t.Fatal(err)
		}
		want = append(want, SnapshotAccount{ID: account.ID, Status: status})
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(privateTempDir(t), "snapshot")
	if err = Create(context.Background(), dataDir, snapshot, "metadata-build", fakeJournal{watermark: testWatermark()}); err != nil {
		t.Fatal(err)
	}
	options, scratchRoot := inspectionOptions(t, snapshot)
	got, err := InspectSnapshot(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(want, func(i, j int) bool { return want[i].ID < want[j].ID })
	if !slices.Equal(got.Accounts, want) {
		t.Fatalf("accounts = %+v, want original statuses %+v", got.Accounts, want)
	}
	if entries, err := os.ReadDir(scratchRoot); err != nil || len(entries) != 0 {
		t.Fatalf("scratch not empty: %v %v", entries, err)
	}
}

func TestInspectSnapshotEnforcesBrainAndDeclaredByteCapsBeforeScratch(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 2)
	options, scratchRoot := inspectionOptions(t, snapshot)
	options.MaxDeclaredBytes = 1
	if got, err := InspectSnapshot(context.Background(), snapshot, options); err == nil || !inspectionIsZero(got) || !strings.Contains(err.Error(), "declared artifact bytes") {
		t.Fatalf("over-byte-limit inspection = %+v, %v", got, err)
	}
	if entries, err := os.ReadDir(scratchRoot); err != nil || len(entries) != 0 {
		t.Fatalf("scratch created before declared-byte check: %v %v", entries, err)
	}
	options, scratchRoot = inspectionOptions(t, snapshot)
	options.MaxBrains = 1
	if got, err := InspectSnapshot(context.Background(), snapshot, options); err == nil || !inspectionIsZero(got) || !strings.Contains(err.Error(), "brain inventory exceeds") {
		t.Fatalf("over-brain-limit inspection = %+v, %v", got, err)
	}
	if entries, err := os.ReadDir(scratchRoot); err != nil || len(entries) != 0 {
		t.Fatalf("scratch created before brain-count check: %v %v", entries, err)
	}
}

func TestReadOnlySnapshotDatabaseUsesEscapedImmutableQueryOnlyURI(t *testing.T) {
	dataDir, snapshot, _ := freshPrivateSnapshot(t, 0)
	options, scratchRoot := inspectionOptions(t, snapshot)
	if _, err := InspectSnapshot(context.Background(), snapshot, options); err != nil {
		t.Fatal(err)
	}
	control := filepath.Join(dataDir, "control.db")
	before, err := os.ReadFile(control)
	if err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(scratchRoot, "control ?# db")
	if err := os.WriteFile(copyPath, before, 0600); err != nil {
		t.Fatal(err)
	}
	readOnly, err := openSnapshotControlDB(context.Background(), copyPath, store.SchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = readOnly.ExecContext(context.Background(), `UPDATE accounts SET status='deleted'`); err == nil {
		_ = readOnly.Close()
		t.Fatal("immutable/query-only SQLite connection accepted a mutation")
	}
	if err = readOnly.Close(); err != nil {
		t.Fatal(err)
	}
	if err := inspectRestoredControlDB(context.Background(), filepath.Join(scratchRoot, "does not exist ?#"), store.SchemaVersion); err == nil {
		t.Fatal("read-only inspector accepted a missing database")
	}
	// A simple URI roundtrip check ensures reserved path characters are encoded
	// into the file URI path rather than interpreted as query/fragment syntax.
	uri := sqliteReadOnlyURI(copyPath)
	if strings.Contains(uri, "control ?# db") || !strings.Contains(uri, "%3F") || !strings.Contains(uri, "%23") {
		t.Fatalf("SQLite file URI did not escape path: %q", uri)
	}
	after, err := os.ReadFile(control)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("source database changed: %v", err)
	}
	entries, err := os.ReadDir(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), "-wal") || strings.HasSuffix(entry.Name(), "-shm") {
			t.Fatalf("inspection created SQLite sidecar in source: %s", entry.Name())
		}
	}
}

func TestSnapshotHelpersHonorCancellationBeforeOpeningInputs(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 0)
	root, err := os.OpenRoot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	manifest := readManifestFile(t, snapshot)
	if err := os.Remove(filepath.Join(snapshot, manifestFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := readManifestBytes(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled manifest read opened missing input: %v", err)
	}
	dest := filepath.Join(privateTempDir(t), "must-not-exist")
	if err := os.Remove(filepath.Join(snapshot, controlDBName)); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyAndCopyContext(ctx, root, manifest.ControlDB, dest); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled artifact copy opened missing input: %v", err)
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled artifact copy created destination: %v", err)
	}
}

func TestInspectSnapshotRejectsInvalidPrivateRootsAndCancelledContext(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 0)
	options, _ := inspectionOptions(t, snapshot)
	publicScratch := filepath.Join(privateTempDir(t), "public")
	if err := os.Mkdir(publicScratch, 0755); err != nil {
		t.Fatal(err)
	}
	options.ScratchRoot = publicScratch
	if got, err := InspectSnapshot(context.Background(), snapshot, options); err == nil || !inspectionIsZero(got) {
		t.Fatalf("public scratch root accepted: %+v %v", got, err)
	}
	options, _ = inspectionOptions(t, snapshot)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := InspectSnapshot(cancelled, snapshot, options); !errors.Is(err, context.Canceled) || !inspectionIsZero(got) {
		t.Fatalf("cancelled inspection = %+v, err=%v", got, err)
	}
}

func TestInspectSnapshotRejectsFIFOsWithoutBlocking(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 0)
	root, err := os.OpenRoot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := syscall.Mkfifo(filepath.Join(snapshot, "inspection-fifo"), 0600); err != nil {
		t.Skipf("FIFO fixture unavailable: %v", err)
	}
	defer func() { _ = os.Remove(filepath.Join(snapshot, "inspection-fifo")) }()
	result := make(chan error, 1)
	go func() {
		file, openErr := openSnapshotRegular(root, "inspection-fifo")
		if file != nil {
			_ = file.Close()
		}
		result <- openErr
	}()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("FIFO was accepted as a snapshot regular file")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("opening FIFO blocked instead of refusing it")
	}
}

func TestInspectionCleanupRefusesReplacedScratchPath(t *testing.T) {
	parent := privateTempDir(t)
	name := "owned-scratch"
	if err := os.Mkdir(filepath.Join(parent, name), 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Errorf("close scratch parent: %v", err)
		}
	}()
	created, err := root.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Rename(name, "renamed-original"); err != nil {
		t.Fatal(err)
	}
	if err := root.Mkdir(name, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(parent, name, "caller-data")
	if err := os.WriteFile(marker, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := removeInspectionScratch(root, name, created); err == nil {
		t.Fatal("cleanup accepted a replaced scratch path")
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "preserve" {
		t.Fatalf("replacement data = %q, err=%v", got, err)
	}
}

type countingReader struct {
	reader io.Reader
	read   int
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.read += n
	return n, err
}

func TestCopyExpectedArtifactBoundsGrowthProbeAndChecksDigest(t *testing.T) {
	ctx := context.Background()
	data := []byte("approved")
	digest := sha256.Sum256(data)
	reader := &countingReader{reader: bytes.NewReader(append(append([]byte(nil), data...), bytes.Repeat([]byte("x"), 1<<20)...))}
	var copied bytes.Buffer
	if _, err := copyExpectedArtifact(ctx, reader, &copied, int64(len(data)), hex.EncodeToString(digest[:])); err == nil || !strings.Contains(err.Error(), "grew beyond") {
		t.Fatalf("growth probe error = %v", err)
	}
	if reader.read != len(data)+1 {
		t.Fatalf("read %d bytes to detect growth, want expected size plus one", reader.read)
	}
	if copied.Len() != len(data) {
		t.Fatalf("copied %d bytes, want only expected %d", copied.Len(), len(data))
	}

	reader = &countingReader{reader: bytes.NewReader(data)}
	copied.Reset()
	if _, err := copyExpectedArtifact(ctx, reader, &copied, int64(len(data)), strings.Repeat("0", 64)); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("checksum error = %v", err)
	}
}

type cancelOnRead struct {
	cancel context.CancelFunc
	data   []byte
	done   bool
}

func (r *cancelOnRead) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	n := copy(p, r.data)
	r.cancel()
	return n, nil
}

func TestCopyExpectedArtifactHonorsCancellationDuringCopy(t *testing.T) {
	data := []byte("approved")
	digest := sha256.Sum256(data)
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelOnRead{cancel: cancel, data: data}
	var dst bytes.Buffer
	if _, err := copyExpectedArtifact(ctx, reader, &dst, int64(len(data)), hex.EncodeToString(digest[:])); !errors.Is(err, context.Canceled) {
		t.Fatalf("copy error = %v, want cancellation", err)
	}
}
