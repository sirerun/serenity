package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/hosted/plans"
	cstore "github.com/sirerun/serenity/internal/store"

	_ "modernc.org/sqlite"
)

// VerifyOptions configure Verify. Expect is required and is never read from the fixture: the caller states what the
// directory is supposed to be, and the verifier compares the disk to that.
type VerifyOptions struct {
	Dir        string
	Workload   string
	Expect     string
	SmokeFacts int // 0 means use the marker's own reduced size when Expect is smoke.
	Workers    int
}

// Check is one comparison of an expected value (derived from the frozen workload and the plan table) to an
// observed value (read from the directory).
type Check struct {
	ID       string `json:"id"`
	Expected any    `json:"expected"`
	Observed any    `json:"observed"`
	Pass     bool   `json:"pass"`
	Detail   string `json:"detail,omitempty"`
}

// BrainObs is what the verifier read from one brain directory.
type BrainObs struct {
	Label                 string           `json:"label"`
	Index                 int              `json:"index"`
	BrainID               string           `json:"brain_id"`
	ExpectedFacts         int              `json:"expected_facts"`
	CanonicalFacts        int              `json:"canonical_facts"`
	CanonicalExpiries     int              `json:"canonical_expiries"`
	ContentMissing        int              `json:"expected_facts_missing"`
	ContentUnexpected     int              `json:"unexpected_facts"`
	ContentMismatch       int              `json:"sha_or_decode_mismatches"`
	ProjectionActive      int              `json:"projection_active_facts"`
	IndexChunks           int              `json:"index_fact_chunks"`
	IndexChunksMissing    int              `json:"index_chunks_missing"`
	IndexChunksExtra      int              `json:"index_chunks_extra"`
	Vectors               int              `json:"index_vectors_under_pin"`
	VectorsMissing        int              `json:"index_vectors_missing"`
	VectorsExtra          int              `json:"index_vectors_extra_or_other_pin"`
	VectorsBadLength      int              `json:"index_vectors_bad_length"`
	ContentSHA256         string           `json:"content_sha256"`
	ConfigPin             string           `json:"config_embedding_pin"`
	GitCommits            int              `json:"git_commits"`
	GitDirtyPaths         int              `json:"git_uncommitted_paths"`
	GitTrackedSources     int              `json:"git_tracked_source_files"`
	StorageBytes          int64            `json:"storage_bytes"`
	StorageByComponent    map[string]int64 `json:"storage_bytes_by_component"`
	Files                 int64            `json:"files"`
	AllocatedBytes        int64            `json:"allocated_bytes"`
	Errors                []string         `json:"errors,omitempty"`
	expectedSHA, foundSHA map[string]struct{}
}

func (b *BrainObs) fail(format string, args ...any) {
	b.Errors = append(b.Errors, fmt.Sprintf(format, args...))
}

// AccountObs is one account as the control database and the brain directories show it.
type AccountObs struct {
	Label             string  `json:"label"`
	PlanID            string  `json:"plan_id"`
	Brains            int     `json:"brains"`
	Memories          int     `json:"memories"`
	MemoryCap         int64   `json:"memory_cap"`
	AtOrOverMemoryCap bool    `json:"at_or_over_memory_cap"`
	StorageBytes      int64   `json:"storage_bytes"`
	StorageQuota      int64   `json:"storage_quota_bytes"`
	StorageShare      float64 `json:"storage_share_of_quota"`
	AtOrOverStorage   bool    `json:"at_or_over_storage_quota"`

	// What the gateway's own Inventory and Entitlement return for the account (see verifyThroughTheGateway).
	GatewayBrains          int64  `json:"gateway_inventory_brains"`
	GatewayMemories        int64  `json:"gateway_inventory_memories"`
	GatewayStorageBytes    int64  `json:"gateway_inventory_storage_bytes"`
	GatewayPlan            string `json:"gateway_entitlement_plan"`
	GatewayRefusesRemember bool   `json:"gateway_limit_inputs_refuse_a_nonreplay_remember"`
}

// Report is the verifier's output. Every number in it was read from the directory.
type Report struct {
	Schema                   string           `json:"schema"`
	Version                  int              `json:"version"`
	FixtureDirName           string           `json:"fixture_dir_name"`
	Expect                   string           `json:"expect"`
	MarkerProfile            string           `json:"marker_profile"`
	Reduced                  bool             `json:"reduced"`
	Pass                     bool             `json:"pass"`
	SatisfiesFullCardinality bool             `json:"satisfies_full_cardinality"`
	EmbedderPin              string           `json:"embedder_pin"`
	EmbedderDim              int              `json:"embedder_dim"`
	CommitEvery              int              `json:"commit_every"`
	FixtureContentSHA256     string           `json:"fixture_content_sha256"`
	ControlDBSHA256          string           `json:"control_db_sha256"`
	MarkerSHA256             string           `json:"marker_sha256"`
	WorkloadSHA256           string           `json:"workload_sha256"`
	Totals                   map[string]int64 `json:"totals"`
	StorageAndHistory        map[string]any   `json:"storage_and_history_gap"`
	Accounts                 []AccountObs     `json:"accounts"`
	Brains                   []*BrainObs      `json:"brains"`
	Checks                   []Check          `json:"checks"`
	Notices                  []string         `json:"notices"`
	NotClaimed               []string         `json:"not_claimed"`
}

func (r *Report) add(id string, expected, observed any, pass bool, detail string) {
	r.Checks = append(r.Checks, Check{ID: id, Expected: expected, Observed: observed, Pass: pass, Detail: detail})
}

func (r *Report) eq(id string, expected, observed any) {
	r.add(id, expected, observed, fmt.Sprint(expected) == fmt.Sprint(observed), "")
}

// Verify reads a prepared fixture without changing it and compares it to the plan derived from the frozen workload and
// the plan table. It opens SQLite files immutable and read-only, runs Git with --no-optional-locks, and never reads a
// credential file's contents. A missing or unfinished marker is a refusal (error); every other difference is a
// failed check.
func Verify(ctx context.Context, o VerifyOptions) (*Report, error) {
	var marker Marker
	raw, err := os.ReadFile(filepath.Join(o.Dir, MarkerFile))
	if err != nil {
		return nil, refuse("%s has no %s: it is not a fixture this tool prepared", filepath.Base(o.Dir), MarkerFile)
	}
	if err = json.Unmarshal(raw, &marker); err != nil || marker.Schema != MarkerSchema || marker.Version != SchemaVersion {
		return nil, refuse("%s in %s is not a %s v%d marker", MarkerFile, filepath.Base(o.Dir), MarkerSchema, SchemaVersion)
	}
	if marker.Status != StatusPrepared {
		return nil, refuse("marker status is %q, not %q: preparation did not finish", marker.Status, StatusPrepared)
	}
	if o.Expect != ProfileFull && o.Expect != ProfileSmoke {
		return nil, fmt.Errorf("fixtureprep: expect must be %q or %q, got %q", ProfileFull, ProfileSmoke, o.Expect)
	}
	wl, err := LoadWorkload(o.Workload)
	if err != nil {
		return nil, err
	}
	smoke := o.SmokeFacts
	if o.Expect == ProfileSmoke && smoke == 0 {
		smoke = marker.SmokeFactsPerBrain
	}
	plan, err := BuildPlan(wl, o.Expect, smoke)
	if err != nil {
		return nil, err
	}
	r := &Report{Schema: "serenity-hosted-load-fixture-verification", Version: 1, FixtureDirName: filepath.Base(o.Dir), Expect: o.Expect, MarkerProfile: marker.Profile, Reduced: plan.Reduced, Notices: Notices, NotClaimed: notClaimed(plan)}
	r.EmbedderPin, r.EmbedderDim, r.CommitEvery = marker.EmbedderPin, marker.EmbedderDim, marker.CommitEvery
	markerSum := sha256.Sum256(raw)
	r.MarkerSHA256, r.WorkloadSHA256 = hex.EncodeToString(markerSum[:]), wl.SHA256
	if db, e := os.ReadFile(filepath.Join(o.Dir, "data", "control.db")); e == nil {
		dbSum := sha256.Sum256(db)
		r.ControlDBSHA256 = hex.EncodeToString(dbSum[:])
	}
	r.eq("marker.profile_equals_expectation", o.Expect, marker.Profile)
	r.eq("marker.workload_sha256_equals_frozen_workload", wl.SHA256, marker.WorkloadSHA256)
	r.eq("marker.embedder_pin_shape", fmt.Sprintf("fixture-hash-embedder-d%d@%s", marker.EmbedderDim, EmbedderVersion), marker.EmbedderPin)

	acctByLabel, _, brainIDs := verifyControlDB(ctx, r, o.Dir, plan)
	verifyBrainDirs(r, o.Dir, brainIDs)

	// One observation per planned brain, in plan order.
	var jobs []*BrainObs
	for _, a := range plan.Accounts {
		for _, b := range a.Brains {
			obs := &BrainObs{Label: a.Label, Index: b.Index, ExpectedFacts: b.Facts}
			if info, ok := acctByLabel[a.Label]; ok && b.Index < len(info.brains) {
				obs.BrainID = info.brains[b.Index]
			}
			jobs = append(jobs, obs)
		}
	}
	workers := max(o.Workers, 1)
	var wg sync.WaitGroup
	slots := make(chan struct{}, workers)
	for _, obs := range jobs {
		slots <- struct{}{}
		wg.Add(1)
		go func(obs *BrainObs) {
			defer func() { <-slots; wg.Done() }()
			observeBrain(ctx, o.Dir, wl, &marker, obs)
		}(obs)
	}
	wg.Wait()
	r.Brains = jobs

	aggregate(r, plan, &marker, jobs, acctByLabel)
	verifyThroughTheGateway(ctx, r, o.Dir, plan, acctByLabel)
	r.Pass = true
	for _, c := range r.Checks {
		r.Pass = r.Pass && c.Pass
	}
	r.SatisfiesFullCardinality = r.Pass && o.Expect == ProfileFull && marker.Profile == ProfileFull && !plan.Reduced &&
		r.Totals["accounts"] == 14 && r.Totals["brains"] == 29 && r.Totals["canonical_memory_facts"] == 90000
	return r, nil
}

type acctInfo struct {
	id     string
	brains []string // default brain first, then the rest in creation order
}

func openImmutable(path string) (*sql.DB, error) {
	if info, err := os.Stat(path + "-wal"); err == nil && info.Size() > 0 {
		return nil, fmt.Errorf("%s has a non-empty write-ahead log: something is running or exited uncleanly", filepath.Base(path))
	}
	u := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro&immutable=1"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// verifyControlDB checks accounts, plans, brains, subscriptions and credential bindings straight from the fixture's
// control database, and the credential files by mode and size only.
func verifyControlDB(ctx context.Context, r *Report, dir string, plan *Plan) (map[string]*acctInfo, map[string]string, map[string]bool) {
	byLabel := map[string]*acctInfo{}
	defaultBrain := map[string]string{}
	brainIDs := map[string]bool{}
	db, err := openImmutable(filepath.Join(dir, "data", "control.db"))
	if err != nil {
		r.add("control_db.readable", "readable", err.Error(), false, "")
		return byLabel, defaultBrain, brainIDs
	}
	defer func() { _ = db.Close() }()
	type acct struct {
		id, email, status, plan string
		version                 int
	}
	rows, err := db.QueryContext(ctx, `SELECT id,email,status,plan_id,plan_version FROM accounts ORDER BY email`)
	if err != nil {
		r.add("control_db.accounts_readable", "readable", err.Error(), false, "")
		return byLabel, defaultBrain, brainIDs
	}
	var accounts []acct
	for rows.Next() {
		var a acct
		if err = rows.Scan(&a.id, &a.email, &a.status, &a.plan, &a.version); err != nil {
			r.add("control_db.accounts_readable", "readable", err.Error(), false, "")
			_ = rows.Close()
			return byLabel, defaultBrain, brainIDs
		}
		accounts = append(accounts, a)
	}
	_ = rows.Close()

	wantPlans := map[string]int{}
	for _, a := range plan.Accounts {
		wantPlans[a.PlanID]++
	}
	gotPlans := map[string]int{}
	byID := map[string]acct{}
	labelOf := map[string]string{}
	stray := 0
	for _, a := range accounts {
		gotPlans[a.plan]++
		byID[a.id] = a
		label, ok := strings.CutSuffix(a.email, "@fixture.invalid")
		if !ok {
			stray++
			continue
		}
		labelOf[a.id] = label
		byLabel[label] = &acctInfo{id: a.id}
	}
	r.eq("control_db.accounts_total", plan.TotalAccounts, len(accounts))
	r.eq("control_db.accounts_by_plan", fmt.Sprint(sortedCounts(wantPlans)), fmt.Sprint(sortedCounts(gotPlans)))
	r.eq("control_db.accounts_not_at_fixture_invalid", 0, stray)
	inactive, badVersion := 0, 0
	for _, a := range accounts {
		if a.status != "active" {
			inactive++
		}
		if a.version != 1 {
			badVersion++
		}
	}
	r.eq("control_db.accounts_not_active", 0, inactive)
	r.eq("control_db.accounts_plan_version_not_1", 0, badVersion)
	var missing []string
	for _, a := range plan.Accounts {
		if _, ok := byLabel[a.Label]; !ok {
			missing = append(missing, a.Label)
		}
	}
	r.add("control_db.planned_labels_present", []string{}, missing, len(missing) == 0, "")
	for _, a := range plan.Accounts {
		if info, ok := byLabel[a.Label]; ok {
			if got := byID[info.id].plan; got != a.PlanID {
				r.add("control_db.account_plan:"+a.Label, a.PlanID, got, false, "")
			}
		}
	}

	brows, err := db.QueryContext(ctx, `SELECT id,account_id,state,is_default,path_key,COALESCE(deleted_at,'') FROM brains ORDER BY created_at,id`)
	if err != nil {
		r.add("control_db.brains_readable", "readable", err.Error(), false, "")
		return byLabel, defaultBrain, brainIDs
	}
	perAcct := map[string][]string{}
	defaults := map[string][]string{}
	notReady, deleted, pathMismatch, total := 0, 0, 0, 0
	for brows.Next() {
		var id, acctID, state, pathKey, deletedAt string
		var isDefault int
		if err = brows.Scan(&id, &acctID, &state, &isDefault, &pathKey, &deletedAt); err != nil {
			r.add("control_db.brains_readable", "readable", err.Error(), false, "")
			_ = brows.Close()
			return byLabel, defaultBrain, brainIDs
		}
		total++
		if state != "ready" {
			notReady++
		}
		if deletedAt != "" {
			deleted++
		}
		if pathKey != id {
			pathMismatch++
		}
		brainIDs[id] = true
		perAcct[acctID] = append(perAcct[acctID], id)
		if isDefault == 1 {
			defaults[acctID] = append(defaults[acctID], id)
		}
	}
	_ = brows.Close()
	r.eq("control_db.brains_total", plan.TotalBrains, total)
	r.eq("control_db.brains_not_ready", 0, notReady)
	r.eq("control_db.brains_deleted", 0, deleted)
	r.eq("control_db.brains_path_key_not_id", 0, pathMismatch)
	wrongBrains, wrongDefaults := []string{}, []string{}
	for _, a := range plan.Accounts {
		info, ok := byLabel[a.Label]
		if !ok {
			continue
		}
		got := perAcct[info.id]
		if len(got) != int(plans.Get(a.PlanID).Brains) || len(got) != len(a.Brains) {
			wrongBrains = append(wrongBrains, fmt.Sprintf("%s=%d", a.Label, len(got)))
		}
		if len(defaults[info.id]) != 1 {
			wrongDefaults = append(wrongDefaults, fmt.Sprintf("%s=%d", a.Label, len(defaults[info.id])))
			continue
		}
		// Default brain first, then the others, so brain index 0 is the default the client's credential is bound to.
		def := defaults[info.id][0]
		defaultBrain[a.Label] = def
		info.brains = append(info.brains, def)
		for _, id := range got {
			if id != def {
				info.brains = append(info.brains, id)
			}
		}
	}
	r.add("control_db.brains_per_account_equal_plan_table", []string{}, wrongBrains, len(wrongBrains) == 0, "each account's brain count must equal its plan's brains")
	r.add("control_db.exactly_one_default_brain_per_account", []string{}, wrongDefaults, len(wrongDefaults) == 0, "")

	// Entitlements: paid accounts need one live subscription in their own plan; free accounts need none.
	srows, err := db.QueryContext(ctx, `SELECT account_id,plan_id,status,current_period_start,current_period_end FROM subscriptions`)
	if err != nil {
		r.add("control_db.subscriptions_readable", "readable", err.Error(), false, "")
		return byLabel, defaultBrain, brainIDs
	}
	subs := map[string][][4]string{}
	for srows.Next() {
		var acctID, planID, status, start, end string
		if err = srows.Scan(&acctID, &planID, &status, &start, &end); err != nil {
			r.add("control_db.subscriptions_readable", "readable", err.Error(), false, "")
			_ = srows.Close()
			return byLabel, defaultBrain, brainIDs
		}
		subs[acctID] = append(subs[acctID], [4]string{planID, status, start, end})
	}
	_ = srows.Close()
	now := time.Now().UTC()
	var badEnt []string
	for _, a := range plan.Accounts {
		info, ok := byLabel[a.Label]
		if !ok {
			continue
		}
		got := subs[info.id]
		if a.PlanID == "free" {
			if len(got) != 0 {
				badEnt = append(badEnt, a.Label+": free account has a subscription")
			}
			continue
		}
		if len(got) != 1 {
			badEnt = append(badEnt, fmt.Sprintf("%s: %d subscriptions", a.Label, len(got)))
			continue
		}
		s := got[0]
		start, e1 := time.Parse(time.RFC3339Nano, s[2])
		end, e2 := time.Parse(time.RFC3339Nano, s[3])
		switch {
		case s[0] != a.PlanID || s[1] != "active":
			badEnt = append(badEnt, fmt.Sprintf("%s: plan %s status %s", a.Label, s[0], s[1]))
		case e1 != nil || e2 != nil || start.After(now) || !end.After(now):
			badEnt = append(badEnt, a.Label+": subscription period does not include the current time")
		}
	}
	r.add("control_db.entitlements_match_plan_and_period", []string{}, badEnt, len(badEnt) == 0, "paid: one active subscription in its own plan whose period includes now; free: none")

	// Credentials: one active credential per account, bound to its default brain, with both memory scopes.
	crows, err := db.QueryContext(ctx, `SELECT account_id,brain_id,scopes,COALESCE(revoked_at,'') FROM client_credentials`)
	if err != nil {
		r.add("control_db.credentials_readable", "readable", err.Error(), false, "")
		return byLabel, defaultBrain, brainIDs
	}
	creds := map[string][][3]string{}
	for crows.Next() {
		var acctID, brainID, scopes, revoked string
		if err = crows.Scan(&acctID, &brainID, &scopes, &revoked); err != nil {
			r.add("control_db.credentials_readable", "readable", err.Error(), false, "")
			_ = crows.Close()
			return byLabel, defaultBrain, brainIDs
		}
		creds[acctID] = append(creds[acctID], [3]string{brainID, scopes, revoked})
	}
	_ = crows.Close()
	var badCred []string
	for _, a := range plan.Accounts {
		info, ok := byLabel[a.Label]
		if !ok {
			continue
		}
		var active [][3]string
		for _, c := range creds[info.id] {
			if c[2] == "" {
				active = append(active, c)
			}
		}
		switch {
		case len(active) != 1:
			badCred = append(badCred, fmt.Sprintf("%s: %d active credentials", a.Label, len(active)))
		case active[0][0] != defaultBrain[a.Label]:
			badCred = append(badCred, a.Label+": credential is not bound to the default brain")
		case !strings.Contains(active[0][1], "memory:read") || !strings.Contains(active[0][1], "memory:write"):
			badCred = append(badCred, a.Label+": credential lacks a memory scope")
		}
	}
	r.add("control_db.one_active_credential_per_account_on_default_brain", []string{}, badCred, len(badCred) == 0, "")

	// Credential files: stat only. The verifier never reads a token.
	var badFiles []string
	if info, err := os.Lstat(filepath.Join(dir, "credentials")); err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		badFiles = append(badFiles, "credentials directory is missing or not mode 0700")
	}
	for _, a := range plan.Accounts {
		info, err := os.Lstat(filepath.Join(dir, "credentials", a.Label+".token"))
		switch {
		case err != nil:
			badFiles = append(badFiles, a.Label+": credential file missing")
		case !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() == 0:
			badFiles = append(badFiles, a.Label+": credential file is not a non-empty regular file of mode 0600")
		}
	}
	r.add("credential_files.private_and_present", []string{}, badFiles, len(badFiles) == 0, "checked by mode and size only; contents are never read")
	return byLabel, defaultBrain, brainIDs
}

func sortedCounts(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, fmt.Sprintf("%s=%d", k, v))
	}
	sort.Strings(out)
	return out
}

// verifyBrainDirs compares the directories under data/brains to the brain ids in the control database.
func verifyBrainDirs(r *Report, dir string, brainIDs map[string]bool) {
	entries, err := os.ReadDir(filepath.Join(dir, "data", "brains"))
	if err != nil {
		r.add("brains_dir.readable", "readable", err.Error(), false, "")
		return
	}
	onDisk := map[string]bool{}
	for _, e := range entries {
		onDisk[e.Name()] = true
	}
	var missing, orphan []string
	for id := range brainIDs {
		if !onDisk[id] {
			missing = append(missing, id)
		}
	}
	for id := range onDisk {
		if !brainIDs[id] {
			orphan = append(orphan, id)
		}
	}
	sort.Strings(missing)
	sort.Strings(orphan)
	r.add("brains_dir.every_database_brain_has_a_directory", []string{}, missing, len(missing) == 0, "")
	r.add("brains_dir.no_directory_without_a_database_brain", []string{}, orphan, len(orphan) == 0, "")
}

// observeBrain reads one brain directory. It records what it finds and never repairs anything.
func observeBrain(ctx context.Context, dir string, wl *Workload, marker *Marker, obs *BrainObs) {
	if obs.BrainID == "" {
		obs.fail("no brain %d for account %s in the control database", obs.Index, obs.Label)
		return
	}
	root := filepath.Join(dir, "data", "brains", obs.BrainID)
	if info, err := os.Lstat(root); err != nil || !info.IsDir() {
		obs.fail("brain directory is missing")
		return
	}
	// The oracle: the exact facts the plan says this brain holds, regenerated from the label, brain and index alone.
	obs.expectedSHA = make(map[string]struct{}, obs.ExpectedFacts)
	for i := 0; i < obs.ExpectedFacts; i++ {
		enc, err := cstore.EncodeMemoryFact(factFor(obs.Label, obs.Index, i, wl.FactTokens.Min, wl.FactTokens.Max).Payload)
		if err != nil {
			obs.fail("regenerate expected fact %d: %v", i, err)
			return
		}
		sum := sha256.Sum256(enc)
		obs.expectedSHA[hex.EncodeToString(sum[:])] = struct{}{}
	}
	obs.foundSHA = map[string]struct{}{}
	observeCanonical(root, obs)
	found := make([]string, 0, len(obs.foundSHA))
	for sha := range obs.foundSHA {
		found = append(found, sha)
	}
	obs.ContentSHA256 = digestOf(found)
	observeProjection(root, obs)
	observeIndex(ctx, root, marker, obs)
	observeConfig(root, obs)
	observeGit(ctx, root, obs)
	observeStorage(root, obs)
}

func observeCanonical(root string, obs *BrainObs) {
	base := filepath.Join(root, "brain", "sources")
	prefixes, err := os.ReadDir(base)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			obs.fail("read sources: %v", err)
		}
		return
	}
	legacy := map[int64]bool{}
	for _, p := range prefixes {
		if !p.IsDir() {
			obs.ContentMismatch++
			continue
		}
		shas, err := os.ReadDir(filepath.Join(base, p.Name()))
		if err != nil {
			obs.fail("read sources/%s: %v", p.Name(), err)
			continue
		}
		for _, s := range shas {
			name := s.Name()
			data, err := os.ReadFile(filepath.Join(base, p.Name(), name, "bytes"))
			if err != nil {
				obs.ContentMismatch++
				continue
			}
			if _, err = os.Stat(filepath.Join(base, p.Name(), name, "meta.yaml")); err != nil {
				obs.ContentMismatch++
			}
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != name || name[:2] != p.Name() {
				obs.ContentMismatch++
				continue
			}
			var probe struct {
				RecordType string `json:"record_type"`
			}
			if json.Unmarshal(data, &probe) != nil {
				obs.ContentMismatch++
				continue
			}
			switch probe.RecordType {
			case cstore.SourceKindMemoryExpiry:
				obs.CanonicalExpiries++
				continue
			case cstore.SourceKindMemoryFact:
			default:
				obs.ContentMismatch++
				continue
			}
			fact, err := cstore.DecodeMemoryFact(data)
			if err != nil || len(fact.Fact) > MaxFactBytes || fact.Provenance != Provenance || legacy[fact.LegacyID] {
				obs.ContentMismatch++
				continue
			}
			legacy[fact.LegacyID] = true
			obs.CanonicalFacts++
			obs.foundSHA[name] = struct{}{}
		}
	}
	for sha := range obs.expectedSHA {
		if _, ok := obs.foundSHA[sha]; !ok {
			obs.ContentMissing++
		}
	}
	for sha := range obs.foundSHA {
		if _, ok := obs.expectedSHA[sha]; !ok {
			obs.ContentUnexpected++
		}
	}
}

// observeProjection loads the brain through the real projection reader, the same one the gateway's inventory uses.
func observeProjection(root string, obs *BrainObs) {
	proj, err := cstore.LoadMemoryProjection(cstore.NewSourceStore(root))
	if err != nil {
		obs.fail("load memory projection: %v", err)
		return
	}
	now := time.Now()
	for _, f := range proj.All() {
		if !f.Expired(now) {
			obs.ProjectionActive++
		}
	}
}

func observeIndex(ctx context.Context, root string, marker *Marker, obs *BrainObs) {
	path := filepath.Join(root, ".serenity", "index.db")
	if _, err := os.Stat(path); err != nil {
		obs.fail("index: index.db is missing")
		return
	}
	db, err := openImmutable(path)
	if err != nil {
		obs.fail("index: %v", err)
		return
	}
	defer func() { _ = db.Close() }()
	chunks := map[string]struct{}{}
	rows, err := db.QueryContext(ctx, `SELECT source_sha256 FROM chunks WHERE kind=?`, cstore.SourceKindMemoryFact)
	if err != nil {
		obs.fail("index chunks: %v", err)
		return
	}
	for rows.Next() {
		var sha string
		if err = rows.Scan(&sha); err != nil {
			obs.fail("index chunks: %v", err)
			break
		}
		chunks[sha] = struct{}{}
	}
	_ = rows.Close()
	obs.IndexChunks = len(chunks)
	for sha := range obs.foundSHA {
		if _, ok := chunks[sha]; !ok {
			obs.IndexChunksMissing++
		}
	}
	for sha := range chunks {
		if _, ok := obs.foundSHA[sha]; !ok {
			obs.IndexChunksExtra++
		}
	}
	vrows, err := db.QueryContext(ctx, `SELECT chunk_ref,model,length(vec) FROM vectors`)
	if err != nil {
		obs.fail("index vectors: %v", err)
		return
	}
	have := map[string]struct{}{}
	for vrows.Next() {
		var ref, model string
		var n int
		if err = vrows.Scan(&ref, &model, &n); err != nil {
			obs.fail("index vectors: %v", err)
			break
		}
		sha, isFact := strings.CutPrefix(ref, "fact:")
		if model != marker.EmbedderPin || !isFact {
			obs.VectorsExtra++
			continue
		}
		if n != marker.EmbedderDim*4 {
			obs.VectorsBadLength++
		}
		if _, ok := obs.foundSHA[sha]; !ok {
			obs.VectorsExtra++
			continue
		}
		have[sha] = struct{}{}
	}
	_ = vrows.Close()
	obs.Vectors = len(have)
	obs.VectorsMissing = len(obs.foundSHA) - len(have)
}

func observeConfig(root string, obs *BrainObs) {
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		obs.fail("config: %v", err)
		return
	}
	obs.ConfigPin = cfg.Models.Embedding
}

func observeGit(ctx context.Context, root string, obs *BrainObs) {
	git := func(args ...string) ([]byte, error) {
		out, err := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks", "-C", root}, args...)...).Output()
		return out, err
	}
	out, err := git("rev-list", "--count", "HEAD")
	if err != nil {
		obs.fail("git rev-list: %v", err)
		return
	}
	obs.GitCommits, err = strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		obs.fail("git rev-list count: %v", err)
		return
	}
	if out, err = git("status", "--porcelain", "-z"); err != nil {
		obs.fail("git status: %v", err)
		return
	}
	obs.GitDirtyPaths = bytes.Count(out, []byte{0})
	if out, err = git("ls-files", "-z", "--", "brain/sources"); err != nil {
		obs.fail("git ls-files: %v", err)
		return
	}
	obs.GitTrackedSources = bytes.Count(out, []byte{0})
}

// observeStorage sums file sizes the way the gateway's inventory does (every non-directory entry under the brain),
// split by the top-level component so the Git history and the derived index are visible next to the canonical facts.
func observeStorage(root string, obs *BrainObs) {
	obs.StorageByComponent = map[string]int64{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		comp := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		if comp != ".git" && comp != ".serenity" && comp != "brain" {
			comp = "other"
		}
		obs.StorageBytes += info.Size()
		obs.StorageByComponent[comp] += info.Size()
		obs.Files++
		if st, ok := info.Sys().(*syscall.Stat_t); ok {
			obs.AllocatedBytes += st.Blocks * 512
		}
		return nil
	})
	if err != nil {
		obs.fail("walk storage: %v", err)
	}
}

// aggregate turns the per-brain observations into account and total checks and the storage and history report.
func aggregate(r *Report, plan *Plan, marker *Marker, brains []*BrainObs, byLabel map[string]*acctInfo) {
	var facts, expiries, chunks, vectors, storage, alloc, files, commits, tracked int64
	var bad []string
	perAcctFacts := map[string]int{}
	perAcctBytes := map[string]int64{}
	comp := map[string]int64{}
	for _, b := range brains {
		facts += int64(b.CanonicalFacts)
		expiries += int64(b.CanonicalExpiries)
		chunks += int64(b.IndexChunks)
		vectors += int64(b.Vectors)
		storage += b.StorageBytes
		alloc += b.AllocatedBytes
		files += b.Files
		commits += int64(b.GitCommits)
		tracked += int64(b.GitTrackedSources)
		perAcctFacts[b.Label] += b.CanonicalFacts
		perAcctBytes[b.Label] += b.StorageBytes
		for k, v := range b.StorageByComponent {
			comp[k] += v
		}
		wantCommits := 0
		if b.ExpectedFacts > 0 {
			wantCommits = (b.ExpectedFacts + marker.CommitEvery - 1) / marker.CommitEvery
		}
		reasons := append([]string(nil), b.Errors...)
		add := func(cond bool, format string, args ...any) {
			if cond {
				reasons = append(reasons, fmt.Sprintf(format, args...))
			}
		}
		add(b.CanonicalFacts != b.ExpectedFacts, "canonical facts %d, want %d", b.CanonicalFacts, b.ExpectedFacts)
		add(b.CanonicalExpiries != 0, "%d memory_expiry sources", b.CanonicalExpiries)
		add(b.ContentMissing != 0 || b.ContentUnexpected != 0 || b.ContentMismatch != 0, "content drift: %d expected facts missing, %d unexpected, %d sha/decode/duplicate-id mismatches", b.ContentMissing, b.ContentUnexpected, b.ContentMismatch)
		add(b.ProjectionActive != b.ExpectedFacts, "projection active facts %d, want %d", b.ProjectionActive, b.ExpectedFacts)
		add(b.IndexChunks != b.ExpectedFacts || b.IndexChunksMissing != 0 || b.IndexChunksExtra != 0, "index chunks %d (missing %d, extra %d), want %d", b.IndexChunks, b.IndexChunksMissing, b.IndexChunksExtra, b.ExpectedFacts)
		add(b.Vectors != b.ExpectedFacts || b.VectorsMissing != 0 || b.VectorsExtra != 0 || b.VectorsBadLength != 0, "vectors under the fixture pin %d (missing %d, extra or other pin %d, bad length %d), want %d", b.Vectors, b.VectorsMissing, b.VectorsExtra, b.VectorsBadLength, b.ExpectedFacts)
		add(b.ConfigPin != marker.EmbedderPin, "config embedding pin %q, want the fixture pin", b.ConfigPin)
		add(b.GitCommits != wantCommits+2, "git commits %d, want %d (provisioning and runtime baselines plus %d batch commits)", b.GitCommits, wantCommits+2, wantCommits)
		add(b.GitDirtyPaths != 0, "%d uncommitted paths", b.GitDirtyPaths)
		add(b.GitTrackedSources != 2*b.ExpectedFacts, "git tracks %d source files, want %d", b.GitTrackedSources, 2*b.ExpectedFacts)
		if len(reasons) > 0 {
			bad = append(bad, fmt.Sprintf("%s/%d: %s", b.Label, b.Index, strings.Join(reasons, "; ")))
		}
	}
	var lines []string
	for _, b := range brains {
		lines = append(lines, fmt.Sprintf("%s/%d %s", b.Label, b.Index, b.ContentSHA256))
	}
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	r.FixtureContentSHA256 = hex.EncodeToString(sum[:])
	r.Totals = map[string]int64{"accounts": int64(len(byLabel)), "brains": int64(len(brains)), "canonical_memory_facts": facts, "memory_expiry_sources": expiries, "index_fact_chunks": chunks, "index_vectors_under_pin": vectors, "git_commits_all_brains": commits, "git_tracked_source_files": tracked}
	r.eq("total.canonical_memory_facts", plan.TotalFacts, facts)
	r.eq("total.index_fact_chunks", plan.TotalFacts, chunks)
	r.eq("total.index_vectors_under_pin", plan.TotalFacts, vectors)
	r.add("brains.every_brain_matches_its_planned_state", []string{}, bad, len(bad) == 0, "canonical content, projection, index rows, vectors, config pin, Git history and tree state, per brain")

	var badAcct []string
	for _, a := range plan.Accounts {
		def := plans.Get(a.PlanID)
		obs := AccountObs{Label: a.Label, PlanID: a.PlanID, Brains: len(a.Brains), Memories: perAcctFacts[a.Label], MemoryCap: def.Memories, StorageBytes: perAcctBytes[a.Label], StorageQuota: def.StorageBytes}
		obs.AtOrOverMemoryCap = int64(obs.Memories) >= def.Memories
		obs.StorageShare = float64(obs.StorageBytes) / float64(def.StorageBytes)
		obs.AtOrOverStorage = obs.StorageBytes >= def.StorageBytes
		r.Accounts = append(r.Accounts, obs)
		if obs.Memories != a.Memories {
			badAcct = append(badAcct, fmt.Sprintf("%s: %d memories, want %d", a.Label, obs.Memories, a.Memories))
		}
		if !plan.Reduced && !obs.AtOrOverMemoryCap {
			badAcct = append(badAcct, fmt.Sprintf("%s: %d memories is below the %d cap", a.Label, obs.Memories, def.Memories))
		}
	}
	r.add("accounts.memories_equal_plan_and_full_profile_is_at_the_plan_cap", []string{}, badAcct, len(badAcct) == 0, "")

	// The storage and history gap is reported as a measurement, separately from the memory-count checks above.
	top := int64(0)
	for _, a := range r.Accounts {
		if int64(a.StorageShare*1e9) > top {
			top = int64(a.StorageShare * 1e9)
		}
	}
	over := 0
	for _, a := range r.Accounts {
		if a.AtOrOverStorage {
			over++
		}
	}
	r.StorageAndHistory = map[string]any{
		"note":                                   "Measured here, separately from the memory count. Sizes follow the gateway's inventory: every non-directory file under each brain, decimal bytes.",
		"storage_bytes_all_brains":               storage,
		"allocated_bytes_all_brains":             alloc,
		"files_all_brains":                       files,
		"storage_bytes_by_component":             comp,
		"largest_account_share_of_storage_quota": float64(top) / 1e9,
		"accounts_at_or_over_storage_quota":      over,
		"git_history_is_batched":                 fmt.Sprintf("Facts are committed in batches of %d, so each brain has two initialization commits (provisioning baseline and runtime configuration) plus ceil(facts/%d) batch commits. Production commits after every acknowledged remember, so its history is longer and larger than this fixture's.", marker.CommitEvery, marker.CommitEvery),
		"fact_text_share_of_quota_at_4096_bytes": "at most 4.096% of every plan's storage quota; the rest is envelope, Git objects and the derived index",
		"at_the_memory_cap":                      "an account whose memories equal its plan cap has every further nonreplay remember refused by the gateway with limit_exceeded (class quota, outcome tool_error); this is read from the cap rule, not observed from a run",
		"storage_saturation":                     "not represented: this fixture fills the memory count, not the storage quota; a storage-saturation sample is a separate design question (decision request, question 5)",
	}
}
