package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

// smokeFactsInTests is the per-brain size of the fixtures the tests prepare: every account and brain, 2 facts each.
const smokeFactsInTests = 2

var (
	fixtureOnce sync.Once
	fixtureRoot string
	fixtureErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if fixtureRoot != "" {
		_ = os.RemoveAll(fixtureRoot) // the tests' own scratch directory, created by sharedFixture.
	}
	os.Exit(code)
}

func testOptions(out string) Options {
	return Options{Out: out, Workload: frozenWorkload, Profile: ProfileSmoke, SmokeFacts: smokeFactsInTests, Dim: 16, CommitEvery: 1, Workers: 1, EntitlementDays: 30, LocalFixture: true}
}

// sharedFixture prepares one reduced smoke fixture per test run. Tests never mutate it; they copy it first.
func sharedFixture(t *testing.T) string {
	t.Helper()
	fixtureOnce.Do(func() {
		root, err := os.MkdirTemp("", "fixtureprep-test-")
		if err != nil {
			fixtureErr = err
			return
		}
		fixtureRoot = root
		_, fixtureErr = Prepare(context.Background(), testOptions(filepath.Join(root, "smoke")))
	})
	if fixtureErr != nil {
		t.Fatalf("prepare the shared smoke fixture: %v", fixtureErr)
	}
	return filepath.Join(fixtureRoot, "smoke")
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
}

// mutable returns a private copy of the shared fixture that a test may damage.
func mutable(t *testing.T) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "copy")
	copyTree(t, sharedFixture(t), dst)
	return dst
}

func verify(t *testing.T, dir, expect string) *Report {
	t.Helper()
	r, err := Verify(context.Background(), VerifyOptions{Dir: dir, Workload: frozenWorkload, Expect: expect, Workers: 2})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	return r
}

func failing(r *Report) map[string]Check {
	out := map[string]Check{}
	for _, c := range r.Checks {
		if !c.Pass {
			out[c.ID] = c
		}
	}
	return out
}

// wantFail asserts the report failed, that the named check is among the failures, and that its observation names substr.
func wantFail(t *testing.T, r *Report, id, substr string) {
	t.Helper()
	if r.Pass {
		t.Fatalf("verification passed but %s should have failed", id)
	}
	if r.SatisfiesFullCardinality {
		t.Fatal("a failed verification must never satisfy the full cardinalities")
	}
	c, ok := failing(r)[id]
	if !ok {
		var ids []string
		for k := range failing(r) {
			ids = append(ids, k)
		}
		t.Fatalf("check %s did not fail; failing checks: %v", id, ids)
	}
	if got := fmt.Sprint(c.Observed); !strings.Contains(got, substr) {
		t.Fatalf("check %s observed %q, want it to contain %q", id, got, substr)
	}
}

// brainRootOf climbs from <brain>/brain/sources/<xx>/<sha>/bytes to <brain>.
func brainRootOf(bytesPath string) string {
	root := bytesPath
	for i := 0; i < 5; i++ {
		root = filepath.Dir(root)
	}
	return root
}

func firstMatch(t *testing.T, dir, pattern string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no match for %s (%v)", pattern, err)
	}
	return matches[0]
}

func mutateDB(t *testing.T, path string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(DELETE)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, s := range stmts {
		if _, err = db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func TestPreparedSmokeFixturePassesAndNeverSatisfiesFull(t *testing.T) {
	r := verify(t, sharedFixture(t), ProfileSmoke)
	if !r.Pass {
		t.Fatalf("a freshly prepared smoke fixture must verify; failing: %v", failing(r))
	}
	if r.SatisfiesFullCardinality || !r.Reduced {
		t.Fatalf("a smoke fixture is reduced and must never satisfy the full cardinalities: %+v", r)
	}
	want := map[string]int64{"accounts": 14, "brains": 29, "canonical_memory_facts": 29 * smokeFactsInTests, "index_fact_chunks": 29 * smokeFactsInTests, "index_vectors_under_pin": 29 * smokeFactsInTests, "memory_expiry_sources": 0}
	for k, v := range want {
		if r.Totals[k] != v {
			t.Errorf("total %s is %d, want %d", k, r.Totals[k], v)
		}
	}
	// Every plan's brain count, read from the database and the directories, and the entitlement paths were all exercised.
	if len(r.Brains) != 29 || len(r.Accounts) != 14 {
		t.Fatalf("report has %d brains and %d accounts", len(r.Brains), len(r.Accounts))
	}
	for _, b := range r.Brains {
		if b.GitCommits != smokeFactsInTests+1 {
			t.Errorf("%s/%d: %d commits, want a baseline plus one per fact (commit-every 1)", b.Label, b.Index, b.GitCommits)
		}
	}
	if !strings.Contains(strings.Join(r.NotClaimed, " "), "REDUCED SMOKE") || !strings.Contains(strings.Join(r.NotClaimed, " "), "unqualified") {
		t.Errorf("the report must state that it is a reduced smoke and that provider quality is unqualified: %v", r.NotClaimed)
	}
	if _, ok := r.StorageAndHistory["git_history_is_batched"]; !ok {
		t.Error("the storage and history gap must be reported separately")
	}
}

func TestSmokePresentedAsFullOrWithTheWrongSizeFails(t *testing.T) {
	dir := sharedFixture(t)
	r := verify(t, dir, ProfileFull)
	wantFail(t, r, "marker.profile_equals_expectation", ProfileSmoke)
	wantFail(t, r, "total.canonical_memory_facts", fmt.Sprint(29*smokeFactsInTests))
	r, err := Verify(context.Background(), VerifyOptions{Dir: dir, Workload: frozenWorkload, Expect: ProfileSmoke, SmokeFacts: smokeFactsInTests + 1})
	if err != nil {
		t.Fatal(err)
	}
	wantFail(t, r, "total.canonical_memory_facts", fmt.Sprint(29*smokeFactsInTests))
}

func TestVerifyDoesNotTrustTheManifest(t *testing.T) {
	dir := mutable(t)
	inflated := map[string]any{"schema": ManifestSchema, "plan": map[string]any{"total_facts": 90000, "total_brains": 29, "satisfies_full_cardinality": true}, "trusted_by_verify": true}
	data, _ := json.Marshal(inflated)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if r := verify(t, dir, ProfileSmoke); !r.Pass || r.SatisfiesFullCardinality {
		t.Fatalf("an inflated manifest must change nothing; failing: %v", failing(r))
	}
	if err := os.Remove(filepath.Join(dir, "manifest.json")); err != nil { // a file this test wrote.
		t.Fatal(err)
	}
	if r := verify(t, dir, ProfileSmoke); !r.Pass {
		t.Fatalf("verification must not need the manifest; failing: %v", failing(r))
	}
}

func fingerprint(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		out[rel] = fmt.Sprintf("%v|%d|%d", info.Mode(), info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestVerifyIsReadOnly(t *testing.T) {
	dir := mutable(t)
	before := fingerprint(t, dir)
	if r := verify(t, dir, ProfileSmoke); !r.Pass {
		t.Fatalf("verify: %v", failing(r))
	}
	after := fingerprint(t, dir)
	for path, want := range before {
		if after[path] != want {
			t.Errorf("verify changed %s: %s -> %s", path, want, after[path])
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			t.Errorf("verify created %s", path)
		}
	}
}

func TestFixtureRecordsNoTokenAndKeepsCredentialsPrivate(t *testing.T) {
	dir := sharedFixture(t)
	for _, name := range []string{MarkerFile, "manifest.json"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "sk_live_") {
			t.Errorf("%s contains a credential token", name)
		}
	}
	info, err := os.Stat(filepath.Join(dir, "credentials", "scale-0.token"))
	if err != nil || info.Mode().Perm() != 0o600 || info.Size() != 60 {
		t.Fatalf("scale-0.token must be a 60-byte 0600 file: %v %v", info, err)
	}
}

func TestPrepareWithWorkersAndBatchedCommitsVerifies(t *testing.T) {
	out := filepath.Join(t.TempDir(), "batched")
	o := testOptions(out)
	o.SmokeFacts, o.CommitEvery, o.Workers = 3, 2, 2
	if _, err := Prepare(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	r := verify(t, out, ProfileSmoke)
	if !r.Pass {
		t.Fatalf("failing: %v", failing(r))
	}
	for _, b := range r.Brains {
		if b.GitCommits != 3 { // baseline plus ceil(3/2) batch commits.
			t.Errorf("%s/%d: %d commits, want 3", b.Label, b.Index, b.GitCommits)
		}
	}
}

func TestPrepareRefusesAnExistingDirectory(t *testing.T) {
	_, err := Prepare(context.Background(), testOptions(sharedFixture(t)))
	mustGuard(t, err, "existing fixture directory")
}

// --- Negative fixtures: every drift below must fail verification, and the untouched copy passes (the red-to-green pair).

func TestNegativeMissingFact(t *testing.T) {
	dir := mutable(t)
	if !verify(t, dir, ProfileSmoke).Pass {
		t.Fatal("the untouched copy must pass")
	}
	if err := os.RemoveAll(filepath.Dir(firstMatch(t, dir, "data/brains/*/brain/sources/*/*/bytes"))); err != nil { // a fact inside this test's own copy.
		t.Fatal(err)
	}
	r := verify(t, dir, ProfileSmoke)
	wantFail(t, r, "total.canonical_memory_facts", fmt.Sprint(29*smokeFactsInTests-1))
	wantFail(t, r, "brains.every_brain_matches_its_planned_state", "canonical facts 1, want 2")
}

func TestNegativeMissingBrainDirectory(t *testing.T) {
	dir := mutable(t)
	if err := os.RemoveAll(firstMatch(t, dir, "data/brains/*")); err != nil { // a brain inside this test's own copy.
		t.Fatal(err)
	}
	r := verify(t, dir, ProfileSmoke)
	wantFail(t, r, "brains_dir.every_database_brain_has_a_directory", "[")
	wantFail(t, r, "brains.every_brain_matches_its_planned_state", "brain directory is missing")
}

func TestNegativeOrphanBrainDirectory(t *testing.T) {
	dir := mutable(t)
	if err := os.Mkdir(filepath.Join(dir, "data", "brains", "ORPHANBRAINORPHANBRAIN0001"), 0o700); err != nil {
		t.Fatal(err)
	}
	wantFail(t, verify(t, dir, ProfileSmoke), "brains_dir.no_directory_without_a_database_brain", "ORPHAN")
}

func TestNegativeExtraFactAndUntrackedFiles(t *testing.T) {
	dir := mutable(t)
	root := brainRootOf(firstMatch(t, dir, "data/brains/*/brain/sources/*/*/bytes"))
	payload := factFor("intruder", 0, 0, 32, 64).Payload
	payload.LegacyID = 9999
	if _, err := store.NewSourceStore(root).WriteMemoryFact(payload); err != nil {
		t.Fatal(err)
	}
	r := verify(t, dir, ProfileSmoke)
	wantFail(t, r, "brains.every_brain_matches_its_planned_state", "canonical facts 3, want 2")
	wantFail(t, r, "brains.every_brain_matches_its_planned_state", "1 unexpected")
	wantFail(t, r, "brains.every_brain_matches_its_planned_state", "uncommitted paths")
}

func TestNegativeTamperedFactBytes(t *testing.T) {
	dir := mutable(t)
	path := firstMatch(t, dir, "data/brains/*/brain/sources/*/*/bytes")
	data, _ := os.ReadFile(path)
	data[len(data)/2] ^= 1
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	wantFail(t, verify(t, dir, ProfileSmoke), "brains.every_brain_matches_its_planned_state", "sha/decode/duplicate-id mismatches")
}

func TestNegativePlantedExpiry(t *testing.T) {
	dir := mutable(t)
	bytesPath := firstMatch(t, dir, "data/brains/*/brain/sources/*/*/bytes")
	sha := filepath.Base(filepath.Dir(bytesPath))
	if _, err := store.NewSourceStore(brainRootOf(bytesPath)).WriteMemoryExpiry(store.MemoryExpiryPayload{FormatVersion: 1, TargetSHA256: sha, Reason: "negative fixture", ExpiredAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	r := verify(t, dir, ProfileSmoke)
	wantFail(t, r, "brains.every_brain_matches_its_planned_state", "memory_expiry sources")
	wantFail(t, r, "brains.every_brain_matches_its_planned_state", "projection active facts 1, want 2")
}

func TestNegativeEntitlementDrift(t *testing.T) {
	for name, stmt := range map[string]string{
		"plan changed":         `UPDATE subscriptions SET plan_id='builder' WHERE account_id=(SELECT id FROM accounts WHERE email='scale-0@fixture.invalid')`,
		"period expired":       `UPDATE subscriptions SET current_period_end='2020-01-01T00:00:00.000000000Z'`,
		"status changed":       `UPDATE subscriptions SET status='canceled' WHERE account_id=(SELECT id FROM accounts WHERE email='builder-1@fixture.invalid')`,
		"subscription removed": `DELETE FROM subscriptions WHERE account_id=(SELECT id FROM accounts WHERE email='builder-2@fixture.invalid')`,
		"free upgraded":        `INSERT INTO subscriptions(id,account_id,price_id,status,current_period_start,current_period_end,cancel_at_period_end,plan_id) SELECT 'x',id,'p','active','2026-01-01T00:00:00.000000000Z','2099-01-01T00:00:00.000000000Z',0,'scale' FROM accounts WHERE email='free-3@fixture.invalid'`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := mutable(t)
			mutateDB(t, filepath.Join(dir, "data", "control.db"), stmt)
			wantFail(t, verify(t, dir, ProfileSmoke), "control_db.entitlements_match_plan_and_period", "[")
		})
	}
}

func TestNegativeAccountAndBrainOwnershipDrift(t *testing.T) {
	t.Run("account plan downgraded", func(t *testing.T) {
		dir := mutable(t)
		mutateDB(t, filepath.Join(dir, "data", "control.db"), `UPDATE accounts SET plan_id='free' WHERE email='scale-0@fixture.invalid'`)
		wantFail(t, verify(t, dir, ProfileSmoke), "control_db.accounts_by_plan", "free=11")
	})
	t.Run("extra account", func(t *testing.T) {
		dir := mutable(t)
		mutateDB(t, filepath.Join(dir, "data", "control.db"), `INSERT INTO accounts(id,email_hash,email,created_at,status,plan_id,plan_version) VALUES('extra','h','extra@fixture.invalid','2026-01-01T00:00:00.000000000Z','active','free',1)`)
		wantFail(t, verify(t, dir, ProfileSmoke), "control_db.accounts_total", "15")
	})
	t.Run("account not on the fixture domain", func(t *testing.T) {
		dir := mutable(t)
		mutateDB(t, filepath.Join(dir, "data", "control.db"), `UPDATE accounts SET email='someone@example.com' WHERE email='free-9@fixture.invalid'`)
		wantFail(t, verify(t, dir, ProfileSmoke), "control_db.accounts_not_at_fixture_invalid", "1")
	})
	t.Run("extra brain", func(t *testing.T) {
		dir := mutable(t)
		mutateDB(t, filepath.Join(dir, "data", "control.db"), `INSERT INTO brains(id,account_id,state,is_default,path_key,created_at) VALUES('EXTRABRAINEXTRABRAINEXTRA1',(SELECT id FROM accounts WHERE email='free-0@fixture.invalid'),'ready',0,'EXTRABRAINEXTRABRAINEXTRA1','2026-01-01T00:00:00.000000000Z')`)
		r := verify(t, dir, ProfileSmoke)
		wantFail(t, r, "control_db.brains_total", "30")
		wantFail(t, r, "control_db.brains_per_account_equal_plan_table", "free-0=2")
	})
	t.Run("default brain removed", func(t *testing.T) {
		dir := mutable(t)
		mutateDB(t, filepath.Join(dir, "data", "control.db"), `UPDATE brains SET is_default=0 WHERE account_id=(SELECT id FROM accounts WHERE email='free-1@fixture.invalid')`)
		wantFail(t, verify(t, dir, ProfileSmoke), "control_db.exactly_one_default_brain_per_account", "free-1=0")
	})
	t.Run("credential revoked", func(t *testing.T) {
		dir := mutable(t)
		mutateDB(t, filepath.Join(dir, "data", "control.db"), `UPDATE client_credentials SET revoked_at='2026-01-02T00:00:00.000000000Z' WHERE account_id=(SELECT id FROM accounts WHERE email='free-2@fixture.invalid')`)
		wantFail(t, verify(t, dir, ProfileSmoke), "control_db.one_active_credential_per_account_on_default_brain", "free-2: 0 active")
	})
	t.Run("credential file missing or loose", func(t *testing.T) {
		dir := mutable(t)
		if err := os.Chmod(filepath.Join(dir, "credentials", "free-4.token"), 0o644); err != nil {
			t.Fatal(err)
		}
		wantFail(t, verify(t, dir, ProfileSmoke), "credential_files.private_and_present", "free-4")
	})
}

func TestNegativeIndexDrift(t *testing.T) {
	t.Run("vector row deleted", func(t *testing.T) {
		dir := mutable(t)
		mutateDB(t, firstMatch(t, dir, "data/brains/*/.serenity/index.db"), `DELETE FROM vectors WHERE rowid IN (SELECT rowid FROM vectors LIMIT 1)`)
		r := verify(t, dir, ProfileSmoke)
		wantFail(t, r, "total.index_vectors_under_pin", fmt.Sprint(29*smokeFactsInTests-1))
		wantFail(t, r, "brains.every_brain_matches_its_planned_state", "missing 1")
	})
	t.Run("chunk row deleted", func(t *testing.T) {
		dir := mutable(t)
		mutateDB(t, firstMatch(t, dir, "data/brains/*/.serenity/index.db"), `DELETE FROM chunks WHERE rowid IN (SELECT rowid FROM chunks LIMIT 1)`)
		wantFail(t, verify(t, dir, ProfileSmoke), "total.index_fact_chunks", fmt.Sprint(29*smokeFactsInTests-1))
	})
	t.Run("vectors under another pin", func(t *testing.T) {
		dir := mutable(t)
		mutateDB(t, firstMatch(t, dir, "data/brains/*/.serenity/index.db"), `UPDATE vectors SET model='other-model@v9'`)
		wantFail(t, verify(t, dir, ProfileSmoke), "brains.every_brain_matches_its_planned_state", "other pin")
	})
	t.Run("vector of the wrong width", func(t *testing.T) {
		dir := mutable(t)
		mutateDB(t, firstMatch(t, dir, "data/brains/*/.serenity/index.db"), `UPDATE vectors SET vec=zeroblob(8) WHERE rowid IN (SELECT rowid FROM vectors LIMIT 1)`)
		wantFail(t, verify(t, dir, ProfileSmoke), "brains.every_brain_matches_its_planned_state", "bad length 1")
	})
	t.Run("index missing", func(t *testing.T) {
		dir := mutable(t)
		if err := os.Remove(firstMatch(t, dir, "data/brains/*/.serenity/index.db")); err != nil { // an index file inside this test's own copy.
			t.Fatal(err)
		}
		wantFail(t, verify(t, dir, ProfileSmoke), "brains.every_brain_matches_its_planned_state", "index:")
	})
}

func TestNegativeConfigAndGitDrift(t *testing.T) {
	t.Run("config pin changed", func(t *testing.T) {
		dir := mutable(t)
		path := firstMatch(t, dir, "data/brains/*/serenity.yml")
		data, _ := os.ReadFile(path)
		if err := os.WriteFile(path, []byte(strings.Replace(string(data), "fixture-hash-embedder-d16@infrastructure-only-v1", "text-embedding-3-small@2024", 1)), 0o644); err != nil {
			t.Fatal(err)
		}
		wantFail(t, verify(t, dir, ProfileSmoke), "brains.every_brain_matches_its_planned_state", "config embedding pin")
	})
	t.Run("history rewritten", func(t *testing.T) {
		dir := mutable(t)
		root := filepath.Dir(firstMatch(t, dir, "data/brains/*/serenity.yml"))
		git(t, root, "commit", "--allow-empty", "-m", "extra")
		wantFail(t, verify(t, dir, ProfileSmoke), "brains.every_brain_matches_its_planned_state", "git commits 4, want 3")
	})
	t.Run("tracked file modified", func(t *testing.T) {
		dir := mutable(t)
		path := firstMatch(t, dir, "data/brains/*/brain/sources/*/*/meta.yaml")
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.WriteString("# drift\n")
		_ = f.Close()
		wantFail(t, verify(t, dir, ProfileSmoke), "brains.every_brain_matches_its_planned_state", "uncommitted paths")
	})
}

func TestNegativeWorkloadAndDatabaseState(t *testing.T) {
	t.Run("dirty write-ahead log", func(t *testing.T) {
		dir := mutable(t)
		if err := os.WriteFile(filepath.Join(dir, "data", "control.db-wal"), []byte("not empty"), 0o600); err != nil {
			t.Fatal(err)
		}
		wantFail(t, verify(t, dir, ProfileSmoke), "control_db.readable", "write-ahead log")
	})
	t.Run("workload drifted since preparation", func(t *testing.T) {
		dir := mutable(t)
		path := filepath.Join(dir, MarkerFile)
		data, _ := os.ReadFile(path)
		var m Marker
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		m.WorkloadSHA256 = strings.Repeat("0", 64)
		out, _ := json.Marshal(m)
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatal(err)
		}
		wantFail(t, verify(t, dir, ProfileSmoke), "marker.workload_sha256_equals_frozen_workload", strings.Repeat("0", 64))
	})
}

func TestVerifyRefusesAMissingOrUnfinishedMarker(t *testing.T) {
	dir := mutable(t)
	path := filepath.Join(dir, MarkerFile)
	data, _ := os.ReadFile(path)
	var m Marker
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	m.Status = StatusPreparing
	out, _ := json.Marshal(m)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Verify(context.Background(), VerifyOptions{Dir: dir, Workload: frozenWorkload, Expect: ProfileSmoke})
	mustGuard(t, err, "unfinished preparation")
	if err = os.Remove(path); err != nil { // the marker inside this test's own copy.
		t.Fatal(err)
	}
	_, err = Verify(context.Background(), VerifyOptions{Dir: dir, Workload: frozenWorkload, Expect: ProfileSmoke})
	mustGuard(t, err, "missing marker")
	if _, err = Verify(context.Background(), VerifyOptions{Dir: t.TempDir(), Workload: frozenWorkload, Expect: ProfileSmoke}); !errors.Is(err, ErrGuard) {
		t.Fatalf("an unrelated directory must be refused, got %v", err)
	}
}
