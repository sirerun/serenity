package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/plans"
	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/hosted/provision"
	hstore "github.com/sirerun/serenity/internal/hosted/store"
	cstore "github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// Schema names of the two files this tool writes.
const (
	MarkerSchema   = "serenity-hosted-load-fixture"
	ManifestSchema = "serenity-hosted-load-fixture-manifest"
	SchemaVersion  = 1

	StatusPreparing = "preparing"
	StatusPrepared  = "prepared"
)

// fixtureScopes are the scopes of the one credential each account gets (its default brain).
var fixtureScopes = []string{"memory:read", "memory:write"}

// Notices are copied into the marker and the manifest so no reader can mistake the fixture for anything else.
var Notices = []string{
	"FIXTURE ONLY: synthetic accounts, credentials, entitlements and facts in an owned, local, freshly created directory.",
	"INFRASTRUCTURE ONLY: vectors come from a hash embedder. They say nothing about provider quality or retrieval quality.",
	"NOT A QUALIFICATION: this is preparation and an inventory. It is not a cold-open, load, steady-state or storage-saturation result.",
	"Entitlements are synthesized by direct SQL in this fixture's own control database; no billing or provider system was contacted.",
}

// Marker is written first (status preparing) and rewritten last (status prepared). verify refuses anything else.
type Marker struct {
	Schema                   string   `json:"schema"`
	Version                  int      `json:"version"`
	Status                   string   `json:"status"`
	Profile                  string   `json:"profile"`
	Reduced                  bool     `json:"reduced"`
	SmokeFactsPerBrain       int      `json:"smoke_facts_per_brain,omitempty"`
	SatisfiesFullCardinality bool     `json:"satisfies_full_cardinality"`
	EmbedderPin              string   `json:"embedder_pin"`
	EmbedderDim              int      `json:"embedder_dim"`
	WorkloadSHA256           string   `json:"workload_sha256"`
	CommitEvery              int      `json:"commit_every"`
	EntitlementDays          int      `json:"entitlement_days"`
	IntendedEndpoint         string   `json:"intended_endpoint,omitempty"`
	PreparedAt               string   `json:"prepared_at"`
	Notices                  []string `json:"notices"`
}

// BrainRecord is what the preparer says it wrote. verify never reads it as evidence.
type BrainRecord struct {
	Index        int     `json:"index"`
	ID           string  `json:"id"`
	Default      bool    `json:"default"`
	Facts        int     `json:"facts"`
	Commits      int     `json:"batch_commits"`
	WriteSeconds float64 `json:"write_seconds"`
	IndexSeconds float64 `json:"index_seconds"`
}

// AccountRecord is one prepared account. It holds a credential id and a file name, never a token.
type AccountRecord struct {
	Label          string        `json:"label"`
	PlanID         string        `json:"plan_id"`
	AccountID      string        `json:"account_id"`
	CredentialID   string        `json:"credential_id"`
	CredentialFile string        `json:"credential_file"`
	Brains         []BrainRecord `json:"brains"`
}

// Manifest is the preparer's own account of the run, kept for the receipt. The verifier ignores it.
type Manifest struct {
	Schema          string          `json:"schema"`
	Version         int             `json:"version"`
	Plan            *Plan           `json:"plan"`
	Accounts        []AccountRecord `json:"accounts"`
	Timing          map[string]any  `json:"timing"`
	Notices         []string        `json:"notices"`
	NotClaimed      []string        `json:"not_claimed"`
	TrustedByVerify bool            `json:"trusted_by_verify"`
}

// Options configure Prepare. There is no endpoint, key or provider option: the tool has no network code.
type Options struct {
	Out             string
	Workload        string
	Profile         string
	SmokeFacts      int
	Dim             int
	CommitEvery     int
	Workers         int
	EntitlementDays int
	Endpoint        string
	LocalFixture    bool
	Log             io.Writer
}

func (o *Options) logf(format string, args ...any) {
	if o.Log != nil {
		_, _ = fmt.Fprintf(o.Log, format+"\n", args...)
	}
}

// Prepare builds the fixture under a new directory and returns the preparer's manifest. It leaves the marker at
// status "preparing" on any failure and never deletes anything: an interrupted run is removed by its owner.
func Prepare(ctx context.Context, o Options) (*Manifest, error) {
	if !o.LocalFixture {
		return nil, refuse("-local-fixture-only is required: this tool creates synthetic accounts, credentials and entitlements and must only run against a new local directory")
	}
	parent, err := CheckOutputDir(o.Out)
	if err != nil {
		return nil, err
	}
	endpoint, err := CheckLoopbackEndpoint(o.Endpoint)
	if err != nil {
		return nil, err
	}
	switch {
	case o.Dim < 8 || o.Dim > 1536:
		return nil, fmt.Errorf("fixtureprep: -dim must be 8..1536, got %d", o.Dim)
	case o.CommitEvery < 1:
		return nil, fmt.Errorf("fixtureprep: -commit-every must be >= 1, got %d", o.CommitEvery)
	case o.Workers < 1 || o.Workers > 4:
		return nil, fmt.Errorf("fixtureprep: -workers must be 1..4, got %d", o.Workers)
	case o.EntitlementDays < 1 || o.EntitlementDays > 365:
		return nil, fmt.Errorf("fixtureprep: -entitlement-days must be 1..365, got %d", o.EntitlementDays)
	}
	wl, err := LoadWorkload(o.Workload)
	if err != nil {
		return nil, err
	}
	if wl.FactTokens.Min < 1 || wl.FactTokens.Max < wl.FactTokens.Min || maxRenderedBytes(wl.FactTokens.Max) > MaxFactBytes {
		return nil, fmt.Errorf("%w: fact_tokens %d..%d can render %d bytes, over the %d byte fact cap", ErrPlanDrift, wl.FactTokens.Min, wl.FactTokens.Max, maxRenderedBytes(wl.FactTokens.Max), MaxFactBytes)
	}
	plan, err := BuildPlan(wl, o.Profile, o.SmokeFacts)
	if err != nil {
		return nil, err
	}
	emb := hashEmbedder{dim: o.Dim}
	start := time.Now()

	out := filepath.Join(parent, filepath.Base(o.Out))
	if err = os.Mkdir(out, 0o700); err != nil {
		return nil, fmt.Errorf("create fixture directory: %w", err)
	}
	dataDir, brainsRoot, credDir := filepath.Join(out, "data"), filepath.Join(out, "data", "brains"), filepath.Join(out, "credentials")
	for _, dir := range []string{dataDir, brainsRoot, credDir} {
		if err = os.Mkdir(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create %s: %w", dir, err)
		}
	}
	marker := Marker{
		Schema: MarkerSchema, Version: SchemaVersion, Status: StatusPreparing, Profile: plan.Profile, Reduced: plan.Reduced,
		SmokeFactsPerBrain: plan.SmokeFactsPerBrain, SatisfiesFullCardinality: plan.SatisfiesFullCardinality,
		EmbedderPin: emb.ModelVersion(), EmbedderDim: o.Dim, WorkloadSHA256: wl.SHA256, CommitEvery: o.CommitEvery,
		EntitlementDays: o.EntitlementDays, IntendedEndpoint: endpoint, PreparedAt: start.UTC().Format(time.RFC3339), Notices: Notices,
	}
	if err = writeJSON(filepath.Join(out, MarkerFile), marker); err != nil {
		return nil, err
	}

	db, err := hstore.Open(filepath.Join(dataDir, "control.db"))
	if err != nil {
		return nil, fmt.Errorf("open fixture control database: %w", err)
	}
	closeDB := sync.OnceValue(db.Close)
	defer func() { _ = closeDB() }()

	prov := &provision.Provisioner{Store: db, BrainsRoot: brainsRoot}
	issuer := &credential.Issuer{Store: db}
	manifest := &Manifest{Schema: ManifestSchema, Version: SchemaVersion, Plan: plan, Notices: Notices, TrustedByVerify: false, NotClaimed: notClaimed(plan)}

	type job struct {
		acct  *AccountRecord
		spec  AccountSpec
		brain BrainSpec
		rec   *BrainRecord
	}
	var jobs []job
	setup := time.Now()
	manifest.Accounts = make([]AccountRecord, len(plan.Accounts))
	for i, spec := range plan.Accounts {
		def := plans.Get(spec.PlanID)
		account, e := db.CreateAccount(ctx, spec.Label+"@fixture.invalid")
		if e != nil {
			return nil, fmt.Errorf("create account %s: %w", spec.Label, e)
		}
		rec := &manifest.Accounts[i]
		*rec = AccountRecord{Label: spec.Label, PlanID: spec.PlanID, AccountID: account.ID, CredentialFile: filepath.Join("credentials", spec.Label+".token")}
		def0, e := prov.Provision(ctx, account.ID)
		if e != nil {
			return nil, fmt.Errorf("provision %s default brain: %w", spec.Label, e)
		}
		ids := []string{def0.ID}
		for len(ids) < int(def.Brains) {
			b, e := prov.Additional(ctx, account.ID, def.Brains)
			if e != nil {
				return nil, fmt.Errorf("provision %s brain %d: %w", spec.Label, len(ids), e)
			}
			ids = append(ids, b.ID)
		}
		if spec.PlanID != "free" {
			if e = grantEntitlement(ctx, db, spec, account.ID, o.EntitlementDays); e != nil {
				return nil, e
			}
		}
		raw, e := issuer.Issue(ctx, def0.ID, account.ID, fixtureScopes)
		if e != nil {
			return nil, fmt.Errorf("issue %s credential: %w", spec.Label, e)
		}
		if e = writeCredential(filepath.Join(out, rec.CredentialFile), raw); e != nil {
			return nil, e
		}
		cred, e := credentialIDFor(ctx, db, account.ID)
		if e != nil {
			return nil, e
		}
		rec.CredentialID = cred
		rec.Brains = make([]BrainRecord, len(spec.Brains))
		for j, b := range spec.Brains {
			rec.Brains[j] = BrainRecord{Index: b.Index, ID: ids[j], Default: j == 0, Facts: b.Facts}
			jobs = append(jobs, job{acct: rec, spec: spec, brain: b, rec: &rec.Brains[j]})
		}
	}
	setupSeconds := time.Since(setup).Seconds()
	o.logf("accounts=%d brains=%d facts=%d profile=%s reduced=%v setup=%.1fs", plan.TotalAccounts, plan.TotalBrains, plan.TotalFacts, plan.Profile, plan.Reduced, setupSeconds)

	// Largest brains first so the longest jobs do not start last.
	sort.SliceStable(jobs, func(a, b int) bool { return jobs[a].brain.Facts > jobs[b].brain.Facts })
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
		done     int
		slots    = make(chan struct{}, o.Workers)
	)
	buildStart := time.Now()
	for _, j := range jobs {
		slots <- struct{}{}
		wg.Add(1)
		go func(j job) {
			defer func() { <-slots; wg.Done() }()
			mu.Lock()
			failed := firstErr != nil
			mu.Unlock()
			if failed {
				return
			}
			err := buildBrain(ctx, o, wl, emb, brainsRoot, j.spec.Label, j.brain, j.rec)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = fmt.Errorf("brain %s/%d: %w", j.spec.Label, j.brain.Index, err)
			}
			done++
			o.logf("brain %d/%d %s/%d facts=%d write=%.1fs index=%.1fs", done, len(jobs), j.spec.Label, j.brain.Index, j.rec.Facts, j.rec.WriteSeconds, j.rec.IndexSeconds)
		}(j)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, fmt.Errorf("%w (the fixture directory %s is incomplete: marker status stays %q and verify refuses it; remove it yourself)", firstErr, out, StatusPreparing)
	}
	if err = closeDB(); err != nil {
		return nil, fmt.Errorf("close fixture control database: %w", err)
	}
	manifest.Timing = map[string]any{
		"setup_seconds": round(setupSeconds), "build_seconds": round(time.Since(buildStart).Seconds()), "total_seconds": round(time.Since(start).Seconds()),
		"workers": o.Workers, "commit_every": o.CommitEvery,
		"per_fact_ms_wall": round(1000 * time.Since(buildStart).Seconds() / float64(max(plan.TotalFacts, 1))),
	}
	if err = writeJSON(filepath.Join(out, "manifest.json"), manifest); err != nil {
		return nil, err
	}
	marker.Status = StatusPrepared
	if err = writeJSON(filepath.Join(out, MarkerFile), marker); err != nil {
		return nil, err
	}
	return manifest, nil
}

// notClaimed lists what a preparation run does not establish; it is copied into every receipt.
func notClaimed(p *Plan) []string {
	claims := []string{
		"Semantic or retrieval quality: vectors come from a hash embedder, so provider quality stays unqualified.",
		"Cold-open, load, latency or steady-state behavior: nothing here ran a service.",
		"The storage and history gap: facts are written in batch commits, where production commits after every acknowledged write; measured bytes belong to this fixture only.",
		"That any load client can use all 29 brains: the current client reads one credential per account, the default brain.",
	}
	if p.Reduced {
		claims = append([]string{fmt.Sprintf("REDUCED SMOKE: %d facts across %d brains. It does not satisfy the 90,000-memory, 29-brain cardinalities.", p.TotalFacts, p.TotalBrains)}, claims...)
	}
	return claims
}

func round(v float64) float64 { return float64(int64(v*1000+0.5)) / 1000 }

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// writeCredential stores a synthetic token in a private file. The token is written once and never printed or logged.
func writeCredential(path, token string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create credential file: %w", err)
	}
	_, werr := f.WriteString(token)
	cerr := f.Close()
	if err = errors.Join(werr, cerr); err != nil {
		return fmt.Errorf("write credential file: %w", err)
	}
	return os.Chmod(path, 0o600)
}

func credentialIDFor(ctx context.Context, db *hstore.Store, accountID string) (string, error) {
	var id string
	err := db.DB().QueryRowContext(ctx, `SELECT id FROM client_credentials WHERE account_id=? AND revoked_at IS NULL`, accountID).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("read credential id: %w", err)
	}
	return id, nil
}

// grantEntitlement writes the paid entitlement the meter reads: an active subscription with a current period and
// the account's plan. It is direct SQL into this fixture's own new control database. Billing normally writes it from a
// Stripe webhook, and no ordinary interface grants a paid plan without one. The synthetic price and subscription ids
// are not Stripe ids.
func grantEntitlement(ctx context.Context, db *hstore.Store, spec AccountSpec, accountID string, days int) error {
	now := time.Now().UTC()
	return db.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscriptions(id,account_id,price_id,status,current_period_start,current_period_end,cancel_at_period_end,plan_id) VALUES(?,?,?,'active',?,?,0,?)`,
			"fixture-sub-"+spec.Label, accountID, "fixture-price-"+spec.PlanID, hstore.Stamp(now.Add(-time.Hour)), hstore.Stamp(now.Add(time.Duration(days)*24*time.Hour)), spec.PlanID); err != nil {
			return fmt.Errorf("grant %s entitlement: %w", spec.Label, err)
		}
		r, err := tx.ExecContext(ctx, `UPDATE accounts SET plan_id=?,plan_version=1 WHERE id=?`, spec.PlanID, accountID)
		if err != nil {
			return err
		}
		if n, _ := r.RowsAffected(); n != 1 {
			return fmt.Errorf("grant %s entitlement: account row not found", spec.Label)
		}
		return nil
	})
}

// openCloseBrain runs the real hosted pool open path once and closes it: it creates the brain's config (with the fixture
// embedding pin), Git repository and baseline commit, opens the index and runs index.RecoverMemorySearch, which embeds
// every eligible fact that has no vector. The pool is single-use because its Acquire is not safe to call concurrently.
func openCloseBrain(ctx context.Context, brainsRoot string, emb embed.Embedder, id string) error {
	p, err := pool.New(pool.Config{MaxOpen: 1, MaxInFlight: 1, BrainsRoot: brainsRoot, Embedder: emb})
	if err != nil {
		return err
	}
	_, release, err := p.Acquire(ctx, id)
	if err != nil {
		return errors.Join(err, p.Close())
	}
	release()
	return p.Close()
}

func buildBrain(ctx context.Context, o Options, wl *Workload, emb embed.Embedder, brainsRoot, label string, b BrainSpec, rec *BrainRecord) error {
	root := filepath.Join(brainsRoot, rec.ID)
	if err := openCloseBrain(ctx, brainsRoot, emb, rec.ID); err != nil {
		return fmt.Errorf("initialize brain: %w", err)
	}
	t0 := time.Now()
	commits, err := writeFacts(ctx, root, label, b, wl, o.CommitEvery)
	if err != nil {
		return fmt.Errorf("write canonical facts: %w", err)
	}
	rec.Commits = commits
	rec.WriteSeconds = round(time.Since(t0).Seconds())
	t1 := time.Now()
	if err = openCloseBrain(ctx, brainsRoot, emb, rec.ID); err != nil {
		return fmt.Errorf("index brain: %w", err)
	}
	rec.IndexSeconds = round(time.Since(t1).Seconds())
	return nil
}

// writeFacts writes b.Facts canonical memory_fact sources through the real store writer, serialized by the writer queue
// and guarded by the brain's single-writer ownership file, then commits them in batches with the real writer.Flush.
// Production allocates each legacy id and resolves duplicates per write inside writer.MemoryFact.Remember, which
// reloads the whole projection on every call; this path assigns sequential ids and rejects a repeated fact SHA by
// construction. The verifier re-checks both independently.
func writeFacts(ctx context.Context, root, label string, b BrainSpec, wl *Workload, commitEvery int) (commits int, err error) {
	owner, err := writer.AcquireBrain(root)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, owner.Close()) }()
	q := writer.NewQueue(nil)
	defer q.Close()
	ss := cstore.NewSourceStore(root)
	seen := make(map[string]struct{}, b.Facts)
	flush := func() error {
		committed, e := writer.Flush(q, root)
		if committed {
			commits++
		}
		return e
	}
	for i := 0; i < b.Facts; i++ {
		if err = ctx.Err(); err != nil {
			return commits, err
		}
		payload := factFor(label, b.Index, i, wl.FactTokens.Min, wl.FactTokens.Max).Payload
		var sha string
		res := q.Submit(writer.Job{Render: func() ([]byte, error) {
			src, e := ss.WriteMemoryFact(payload)
			if src.SHA256 != "" {
				dir := ss.DirFor(src.SHA256)
				q.MarkTouched(filepath.Join(dir, "bytes"))
				q.MarkTouched(filepath.Join(dir, "meta.yaml"))
				sha = src.SHA256
			}
			return nil, e
		}})
		if res.Err != nil {
			return commits, res.Err
		}
		if _, dup := seen[sha]; dup {
			return commits, fmt.Errorf("fact %d repeats an earlier fact's content", i)
		}
		seen[sha] = struct{}{}
		if (i+1)%commitEvery == 0 {
			if err = flush(); err != nil {
				return commits, err
			}
		}
	}
	return commits, flush()
}

// digestOf is the order-independent digest of a brain's fact SHAs.
func digestOf(shas []string) string {
	sorted := append([]string(nil), shas...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:])
}
