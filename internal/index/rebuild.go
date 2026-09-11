package index

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/extract/chunk"
	"github.com/sirerun/serenity/internal/store"
)

// Rebuild reconstructs the entire derived index from canonical repo bytes
// (RFC §7 preamble: "there are no database backups — the index rebuilds
// from the repo"). Deterministic: files are walked in sorted order and
// every derived row comes from the file contents alone.
//
// Authority rule (§7.2a): for shard-tier families the shard is canonical
// and the fence head row is derived — so fence rows belonging to
// shard-tier families are skipped here and the shard resolution is
// indexed instead. A hand-edited (diverged) fence head therefore never
// leaks into the index; the reconciler (M2) turns such divergence into a
// disposition item.
func Rebuild(ctx context.Context, root string, cfg *config.Config, eng Engine) error {
	srcStore := store.NewSourceStore(root)
	sources, err := srcStore.All()
	if err != nil {
		return err
	}
	memProj, err := store.LoadMemoryProjection(srcStore)
	if err != nil {
		return fmt.Errorf("rebuild: load memory projection: %w", err)
	}
	// A derived summary has no span-level privacy attribution. Omit it when
	// canonical history contains restricted evidence rather than embedding or
	// returning a stale summary that may have incorporated that evidence.
	now := time.Now()
	restricted, err := RestrictedSummaryEntities(root, memProj, now)
	if err != nil {
		return err
	}
	if err := eng.ResetAll(ctx); err != nil {
		return err
	}

	// Entity pages: entities, fence-tier claims, page chunks.
	pages, err := filepath.Glob(filepath.Join(root, "brain", "entities", "*", "*.md"))
	if err != nil {
		return err
	}
	sort.Strings(pages)
	fw := store.NewFenceWriter(root)
	for _, path := range pages {
		p, err := fw.ParseEntity(path)
		if err != nil {
			return fmt.Errorf("rebuild %s: %w", path, err)
		}
		if p.Entity.Slug == "" {
			return fmt.Errorf("rebuild %s: page has no slug", path)
		}
		if err := eng.UpsertEntity(ctx, p.Entity); err != nil {
			return err
		}
		for _, c := range p.Claims {
			if cfg.TierOf(c.Family) == domain.TierShard {
				continue // derived head row; the shard below is canonical
			}
			if c.ObjectKey == "" {
				c.ObjectKey = store.NormalizeKey(c.Object)
			}
			if err := eng.UpsertClaim(ctx, c); err != nil {
				return err
			}
		}
		text := p.Title
		for _, cl := range p.Claims {
			if cl.Visibility == domain.VisibilityPrivate || memProj.SourceIndexOnly(cl.Provenance.SourceSHA256) {
				restricted[p.Entity.Slug] = true
			}
		}
		if !restricted[p.Entity.Slug] {
			text += "\n" + p.Summary
		}
		// Entity-page chunks are derived summaries, not raw Source
		// material -- no SourceSHA256 to carry, "entity_page" as their
		// own Kind bucket for internal/search's per-type cap (T1.11).
		if err := eng.InsertChunk(ctx, "page:"+p.Entity.Slug, p.Entity.Slug, text, "", "entity_page"); err != nil {
			return err
		}
	}

	// Shards: resolved heads are the indexed truth for shard-tier families.
	ss := store.NewShardStore(root)
	slugs, err := ss.Slugs()
	if err != nil {
		return err
	}
	for _, slug := range slugs {
		families, err := ss.Families(slug)
		if err != nil {
			return err
		}
		for _, family := range families {
			heads, err := ss.ResolveHeads(slug, family)
			if err != nil {
				return err
			}
			for _, key := range store.HeadKeys(heads) {
				c := heads[key]
				c.SourceRef = "shard"
				if err := eng.UpsertClaim(ctx, c); err != nil {
					return err
				}
			}
		}
	}

	// Canonical claims may have no matching source text (notably human edits).
	// Their derived chunks are revalidated against canonical state on every read.
	canonical, err := canonicalClaims(root, now, cfg)
	if err != nil {
		return err
	}
	refs := make([]string, 0, len(canonical))
	for ref := range canonical {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		rec := canonical[ref]
		if err := eng.InsertChunk(ctx, ref, rec.slug, rec.text, rec.claim.Provenance.SourceSHA256, rec.kind); err != nil {
			return err
		}
	}

	// Raw sources: index every stored source's own text for full-text
	// search (RFC §10.1's "index" pipeline stage) -- searchable evidence
	// distinct from the entity-page/claim-derived chunks above, available
	// even before extraction has run over it (T1.15's `serenity sync`).
	// Sorted by SHA256 (SourceStore.All's own order) for the same
	// reproducibility guarantee the entity-page glob above gets from its
	// sort.Strings. Binary/non-UTF8 sources (PDFs, images -- no
	// text-extraction pipeline exists yet in this codebase, a disclosed
	// v1 gap) are stored and git-committed by `serenity sync` like any
	// other source; they are simply never chunked into the FTS index here.
	//
	// MEMORY_VERBS memory_fact/memory_expiry sources (T4.20) are a
	// distinct raw-ingress kind and never fall through to the generic
	// chunk.Split branch below: a memory_expiry source is a lifecycle
	// event, never indexed at all (mapping doc: "Exclude lifecycle
	// events"), and a memory_fact source's bytes are a canonical JSON
	// envelope, not prose -- indexing the envelope verbatim would leak
	// its own field names into full-text search and would defeat the
	// point of decoding it first. Only the fact's own text is indexed,
	// as one chunk, tagged with the payload's own entity_slug so recall's
	// query arm and entity() resolve it exactly like any other page hit.
	// A fact already expired by its own TTL as of this rebuild is skipped
	// here too -- an authoritative belt-and-suspenders alongside the
	// query-time canonical filter (search.Options.Eligible /
	// store.MemoryEligible) every read path also applies, since an index
	// can go stale between rebuilds. Both private and world-visible
	// facts ARE indexed here (mapping: "Rebuild indexes decoded active
	// fact text (including local-private facts for CLI local search)");
	// the audience split (MCP remote vs local CLI) is enforced at query
	// time, never by omission from the index.
	for _, src := range sources {
		switch src.Kind {
		case store.SourceKindMemoryExpiry:
			continue
		case store.SourceKindMemoryFact:
			rec, ok := memProj.Get(src.SHA256)
			if !ok || rec.Expired(now) {
				continue
			}
			ref := "fact:" + src.SHA256
			if err := eng.InsertChunk(ctx, ref, rec.Payload.EntitySlug, rec.Payload.Fact, src.SHA256, store.SourceKindMemoryFact); err != nil {
				return err
			}
			continue
		}

		data, _, err := srcStore.Read(src.SHA256)
		if err != nil {
			return fmt.Errorf("rebuild: read source %s: %w", src.SHA256, err)
		}
		if !utf8.Valid(data) {
			continue
		}
		for _, ch := range chunk.Split(string(data), chunk.DefaultConfig) {
			ref := fmt.Sprintf("src:%s:%d-%d", src.SHA256, ch.Span.Start, ch.Span.End)
			if err := eng.InsertChunk(ctx, ref, "", ch.Text, src.SHA256, src.Kind); err != nil {
				return err
			}
		}
	}
	return nil
}

// DumpString renders the deterministic dump as a string (test helper and
// doctor output).
func DumpString(ctx context.Context, eng Engine) (string, error) {
	var b strings.Builder
	if err := eng.Dump(ctx, &b); err != nil {
		return "", err
	}
	return b.String(), nil
}

// Embedder is the subset of internal/embed.Embedder that ReembedMissing
// needs, declared locally so this package never has to import
// internal/embed -- the same asymmetric-dependency shape jobs.go uses for
// connector.JobStore and ledger.go's doc comment describes for
// router.SpendLedger. *embed.RouterEmbedder satisfies this structurally.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	ModelVersion() string
}

// chunksLackingVector returns every egress-eligible indexed chunk with no stored
// vector under pin, in AllChunks' deterministic chunk_ref order. This is
// the read-only half shared by ReembedMissing (which then embeds and
// writes each one) and PendingReembed (which only counts them) -- the
// same absence-of-a-(chunk_ref,model)-row that lets embed.Search fall
// back to FTS for a chunk (RFC §10.1) is exactly what "pending_reembed"
// means for a chunk under a pin: there is no separate status column to
// drift out of sync with the vectors table, only this query.
func chunksLackingVector(ctx context.Context, eng *SQLite, pin string) ([]Hit, error) {
	chunks, err := eng.AllChunks(ctx)
	if err != nil {
		return nil, err
	}
	root, err := indexedRoot(ctx, eng)
	if err != nil {
		return nil, err
	}
	proj, err := store.LoadMemoryProjection(store.NewSourceStore(root))
	if err != nil {
		return nil, fmt.Errorf("reembed: source policy: %w", err)
	}
	now := time.Now()
	eligible, err := RetrievalEligibility(root, proj, true, true, now)
	if err != nil {
		return nil, err
	}
	var missing []Hit
	for _, c := range chunks {
		if !eligible(c) {
			continue
		}
		has, err := eng.HasVector(ctx, c.ChunkRef, pin)
		if err != nil {
			return nil, err
		}
		if !has {
			missing = append(missing, c)
		}
	}
	return missing, nil
}

// PendingReembed reports how many indexed chunks currently lack a vector
// under pin -- the count ReembedMissing(ctx, eng, embedder) would actually
// embed if called with an embedder pinned to pin right now (RFC §10.1
// "staged re-embed", plan T1.16 `serenity migrate --models`). A pin that
// has never embedded anything (a fresh migration target) reports every
// eligible chunk as pending, since none has a vector under it yet. Read-only:
// unlike ReembedMissing, this never calls UpsertVector and is safe to call
// from outside the file-first allowlist (e.g. the CLI, before it decides
// whether to run the migration at all).
func PendingReembed(ctx context.Context, eng *SQLite, pin string) (int, error) {
	missing, err := chunksLackingVector(ctx, eng, pin)
	if err != nil {
		return 0, err
	}
	return len(missing), nil
}

// ReembedMissing fills in every egress-eligible chunk's vector under embedder's
// pin (RFC §10.1's "embed" pipeline stage), skipping chunks that already
// have one under that pin. It lives here, not in the CLI, because
// UpsertVector is an index-write primitive the file-first CI gate
// (internal/gate) restricts to this exact file plus internal/writer/ --
// the same rule Rebuild itself is allowlisted under. Called after Rebuild
// (which wipes and fully reconstructs the vectors table via ResetAll):
// the wipe-and-rebuild invariant requires every vector to be reproducible
// purely from repo bytes plus the pinned model, never carried forward as
// separate state, so a real embedding pass -- a pure function of (pin,
// text), per T1.10's own TestVectorsParticipateInRebuildIdentity -- runs
// fresh after every Rebuild, not only for newly added chunks.
func ReembedMissing(ctx context.Context, eng *SQLite, embedder Embedder) (embedded int, err error) {
	pin := embedder.ModelVersion()
	missing, err := chunksLackingVector(ctx, eng, pin)
	if err != nil {
		return 0, err
	}
	for _, c := range missing {
		vec, err := embedder.Embed(ctx, c.Text)
		if err != nil {
			return embedded, fmt.Errorf("index: reembed chunk %s: %w", c.ChunkRef, err)
		}
		if err := eng.UpsertVector(ctx, c.ChunkRef, pin, vec); err != nil {
			return embedded, err
		}
		embedded++
	}
	return embedded, nil
}

// Refresh rebuilds the file-derived projection while retaining vectors only
// when both the chunk reference and its exact text are unchanged. All model
// pins are kept separately; deleted or edited chunks lose every old vector.
// Runtime tables remain untouched, just as in Rebuild. After interruption the
// projection is disposable and a subsequent refresh/re-embed repairs it.
func Refresh(ctx context.Context, root string, cfg *config.Config, eng *SQLite) error {
	chunks, err := eng.AllChunks(ctx)
	if err != nil {
		return err
	}
	old := make(map[string]string, len(chunks))
	for _, ch := range chunks {
		old[ch.ChunkRef] = ch.Text
	}
	type savedVector struct {
		ref, model string
		blob       []byte
	}
	rows, err := eng.db.QueryContext(ctx, `SELECT chunk_ref, model, vec FROM vectors`)
	if err != nil {
		return fmt.Errorf("refresh vectors: %w", err)
	}
	var saved []savedVector
	for rows.Next() {
		var v savedVector
		if err := rows.Scan(&v.ref, &v.model, &v.blob); err != nil {
			_ = rows.Close()
			return err
		}
		saved = append(saved, v)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := Rebuild(ctx, root, cfg, eng); err != nil {
		return err
	}
	current, err := eng.AllChunks(ctx)
	if err != nil {
		return err
	}
	unchanged := make(map[string]bool, len(current))
	for _, ch := range current {
		text, ok := old[ch.ChunkRef]
		unchanged[ch.ChunkRef] = ok && text == ch.Text
	}
	for _, v := range saved {
		if !unchanged[v.ref] {
			continue
		}
		if _, err := eng.db.ExecContext(ctx, `INSERT INTO vectors(chunk_ref, model, vec) VALUES(?,?,?)`, v.ref, v.model, v.blob); err != nil {
			return fmt.Errorf("refresh restore vector: %w", err)
		}
	}
	return nil
}

// SourceEligibility applies audience/lifecycle policy to source metadata.
// It does not validate ordinary chunk identity or content; indexed read paths
// must use RetrievalEligibility. IndexOnly excludes remote recall and provider
// egress while allowing local-owner access to available source bytes.
func SourceEligibility(proj *store.MemoryProjection, remote, egress bool, now time.Time, restricted ...map[string]bool) func(Hit) bool {
	return func(h Hit) bool {
		// Only RetrievalEligibility can validate this reserved canonical projection.
		if strings.HasPrefix(h.Kind, "gbrain_") || strings.HasPrefix(h.Kind, "canonical_") || strings.HasPrefix(h.ChunkRef, "gbrain-claim:") || strings.HasPrefix(h.ChunkRef, "canonical-claim:") {
			return false
		}
		if h.Kind == "entity_page" {
			for _, subjects := range restricted {
				if subjects[h.EntitySlug] {
					return false
				}
			}
		}
		if proj.IsLifecycle(h.SourceSHA256) || h.Kind == store.SourceKindMemoryExpiry {
			return false
		}
		if strings.HasPrefix(h.Kind, "memory_") {
			if h.Kind != store.SourceKindMemoryFact {
				return false
			}
			if _, ok := proj.Get(h.SourceSHA256); !ok {
				return false
			}
		}
		if (remote || egress) && proj.SourceIndexOnly(h.SourceSHA256) {
			return false
		}
		return store.MemoryEligible(proj, h.SourceSHA256, remote || egress, now)
	}
}

// Production indexes live under <brain>/.serenity. Standalone index fixtures
// use their containing directory and still fail closed for reserved chunks.
func indexedRoot(ctx context.Context, eng *SQLite) (string, error) {
	rows, err := eng.db.QueryContext(ctx, "PRAGMA database_list")
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var seq int
		var name, file string
		if err := rows.Scan(&seq, &name, &file); err != nil {
			return "", err
		}
		if name == "main" {
			root := filepath.Dir(file)
			if filepath.Base(root) == ".serenity" {
				root = filepath.Dir(root)
			}
			return root, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("index: main database path unavailable")
}

// RestrictedSummaryEntities identifies untraceable cached summaries that may
// quote private, index-only or expired source evidence. Canonical parse errors
// fail closed. Retained history remains relevant because cached text can be old.
func RestrictedSummaryEntities(root string, proj *store.MemoryProjection, now time.Time) (map[string]bool, error) {
	restricted := make(map[string]bool)
	for _, rec := range proj.All() {
		if rec.Payload.Visibility == store.MemoryVisibilityPrivate || proj.SourceIndexOnly(rec.SHA256) || rec.Expired(now) {
			restricted[rec.Payload.EntitySlug] = true
		}
	}
	pages, err := filepath.Glob(filepath.Join(root, "brain", "entities", "*", "*.md"))
	if err != nil {
		return nil, err
	}
	fw := store.NewFenceWriter(root)
	for _, path := range pages {
		page, err := fw.ParseEntity(path)
		if err != nil {
			return nil, err
		}
		for _, cl := range page.Claims {
			if cl.Visibility == domain.VisibilityPrivate || proj.SourceIndexOnly(cl.Provenance.SourceSHA256) || !store.MemoryEligible(proj, cl.Provenance.SourceSHA256, true, now) {
				restricted[page.Entity.Slug] = true
			}
		}
	}
	shards := store.NewShardStore(root)
	slugs, err := shards.Slugs()
	if err != nil {
		return nil, err
	}
	for _, slug := range slugs {
		families, err := shards.Families(slug)
		if err != nil {
			return nil, err
		}
		for _, family := range families {
			lines, err := shards.Lines(slug, family)
			if err != nil {
				return nil, err
			}
			for _, cl := range lines {
				if cl.Visibility == domain.VisibilityPrivate || proj.SourceIndexOnly(cl.Provenance.SourceSHA256) || !store.MemoryEligible(proj, cl.Provenance.SourceSHA256, true, now) {
					restricted[slug] = true
				}
			}
		}
	}
	return restricted, nil
}

// RefreshMemoryFact replaces one derived chunk after a canonical memory write.
// It reads the persisted source representation again rather than accepting new
// authoritative text through an index-only path. TTL and visibility remain
// enforced by SourceEligibility at query time.
func RefreshMemoryFact(ctx context.Context, root, sha string, eng *SQLite, now time.Time) error {
	projection, err := store.LoadMemoryProjection(store.NewSourceStore(root))
	if err != nil {
		return err
	}
	rec, ok := projection.Get(sha)
	if !ok {
		return fmt.Errorf("index: memory source not found")
	}
	tx, err := eng.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	ref := "fact:" + sha
	if _, err := tx.ExecContext(ctx, "DELETE FROM chunks WHERE chunk_ref = ?", ref); err != nil {
		return err
	}
	if !rec.Expired(now) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO chunks(chunk_ref, entity_slug, text, source_sha256, kind) VALUES(?,?,?,?,?)`, ref, rec.Payload.EntitySlug, rec.Payload.Fact, sha, store.SourceKindMemoryFact); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RefreshMemoryFactSearch indexes one immutable fact and, when configured,
// embeds only that eligible source. The caller must hold its writer queue for
// the complete call so a withdrawal cannot race the egress decision. Failure
// leaves canonical storage untouched; lexical availability can still succeed.
func RefreshMemoryFactSearch(ctx context.Context, root, sha string, eng *SQLite, embedder Embedder, clock func() time.Time) (state string, expired bool, err error) {
	state = "unavailable"
	now := clock()
	proj, err := store.LoadMemoryProjection(store.NewSourceStore(root))
	if err != nil {
		return state, false, err
	}
	rec, ok := proj.Get(sha)
	if !ok {
		return state, false, fmt.Errorf("index: memory source not found")
	}
	expired = rec.Expired(now)
	if err := RefreshMemoryFact(ctx, root, sha, eng, now); err != nil {
		return state, expired, err
	}
	hit := Hit{ChunkRef: "fact:" + sha, EntitySlug: rec.Payload.EntitySlug, Text: rec.Payload.Fact, SourceSHA256: sha, Kind: store.SourceKindMemoryFact}
	eligible, err := RetrievalEligibility(root, proj, true, true, now)
	if err != nil {
		return state, expired, err
	}
	if !eligible(hit) || rec.Expired(clock()) {
		return "not_eligible", rec.Expired(clock()), nil
	}
	state = "lexical"
	if embedder == nil {
		return state, expired, nil
	}
	pin := embedder.ModelVersion()
	has, err := eng.HasVector(ctx, hit.ChunkRef, pin)
	if err != nil {
		return state, expired, err
	}
	if rec.Expired(clock()) {
		return "not_eligible", true, nil
	}
	if has {
		return "semantic", false, nil
	}
	// Bound egress by remaining TTL as well as the caller's indexing deadline.
	if rec.Payload.ValidUntil != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rec.Payload.ValidUntil.Sub(clock()))
		defer cancel()
	}
	if rec.Expired(clock()) {
		return "not_eligible", true, nil
	}
	if ctx.Err() != nil {
		return state, false, ctx.Err()
	}
	vec, err := embedder.Embed(ctx, hit.Text)
	if rec.Expired(clock()) {
		return "not_eligible", true, nil
	}
	if err != nil {
		return state, expired, err
	}
	if err := eng.UpsertVector(ctx, hit.ChunkRef, pin, vec); err != nil {
		return state, expired, err
	}
	return "semantic", expired, nil
}
