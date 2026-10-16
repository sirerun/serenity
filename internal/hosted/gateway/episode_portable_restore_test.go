package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/episodeerase"
	"github.com/sirerun/serenity/internal/hosted/backup"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/deletion"
	"github.com/sirerun/serenity/internal/hosted/meter"
	hoststore "github.com/sirerun/serenity/internal/hosted/store"
	"github.com/sirerun/serenity/internal/store"
)

// T1.27's bounded synthetic portable boundary: actual manifest-v2 Create and
// Restore, then caller-trusted sealed-history replay while accounts stay frozen.
// This is not hosted assembly, provider admission, managed restore or activation.
// Reuse the chain's explicit seal check/replay helper, not its manual Git copy.
type episodePortableRestore struct {
	*episodeErasureChain
	parent, snapshot string
	binding          credential.Binding
	manifest         contracts.ManifestV2
	artifacts        map[string][32]byte
	oldHead          string
}

func newEpisodePortableRestore(t *testing.T) *episodePortableRestore {
	t.Helper()
	ctx := context.Background()
	g, p, brain, token := episodeHTTPSetup(t)
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	dataDir, err := filepath.EvalSymlinks(filepath.Dir(p.BrainsRoot()))
	if err != nil {
		t.Fatal(err)
	}
	f := &episodePortableRestore{
		episodeErasureChain: &episodeErasureChain{g: g, brain: brain, token: token,
			root: filepath.Join(dataDir, "brains", brain), journalRoot: filepath.Join(parent, "external-journal")},
		parent: parent, snapshot: filepath.Join(parent, "portable-before-exclusion"),
		artifacts: make(map[string][32]byte),
	}
	f.binding, err = g.Issuer.Verify(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	f.journal, err = deletion.NewFilesystemJournal(f.journalRoot, "synthetic-portable", 1, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	g.Journal = f.journal
	f.original = episodeHTTPFixture(t, brain)
	f.original.Mutation.Events[0].Text = "PORTABLE-ORIGINAL-SECRET"
	f.sibling = episodeHTTPFixture(t, brain)
	f.sibling.Mutation.CommandID = "portable-independent-command"
	f.sibling.Mutation.ConversationID = "portable-independent-conversation"
	f.sibling.Mutation.Events[0].Text = "PORTABLE-INDEPENDENT-SURVIVOR"
	put := func(req episodeRequest) store.EpisodeReceipt {
		t.Helper()
		code, res, body := episodeHTTPCall(t, g, token, req)
		if code != 200 || res.Receipt == nil {
			t.Fatalf("portable source publication: %d %s", code, body)
		}
		return *res.Receipt
	}
	f.originalReceipt = put(f.original)
	f.siblingReceipt = put(f.sibling)
	f.dependent = episodeHTTPFixture(t, brain)
	f.dependent.Operation = "checkpoint"
	f.dependent.Mutation.Operation = "checkpoint"
	f.dependent.Mutation.CommandID = "portable-dependent-checkpoint"
	f.dependent.Mutation.ConversationID = "portable-derived-conversation"
	f.dependent.Mutation.Events = nil
	f.dependent.Mutation.Checkpoint = &store.EpisodeCheckpoint{Text: "PORTABLE-DERIVED-SECRET", Model: "synthetic",
		CreatedAt: f.original.Mutation.Events[0].ObservedAt, Sources: f.originalReceipt.EventRefs}
	f.dependentReceipt = put(f.dependent)
	// The complete hosted repository (including config, ignore file and other
	// branch) is bundled. No episode-only reconstruction or history filtering.
	f.oldHead = strings.TrimSpace(string(chainGit(t, f.root, "rev-parse", "HEAD")))
	if err = p.Close(); err != nil {
		t.Fatal(err)
	}
	if err = g.Issuer.Store.Close(); err != nil {
		t.Fatal(err)
	}
	if err = backup.Create(ctx, dataDir, f.snapshot, "synthetic-portable-t127", f.journal); err != nil {
		t.Fatalf("public backup.Create: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(f.snapshot, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &f.manifest); err != nil {
		t.Fatal(err)
	}
	if err = f.manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	if f.manifest.Version != 2 || f.manifest.ControlDB.RelativePath != "control.db" ||
		len(f.manifest.Brains) != 1 || f.manifest.Brains[0].ID != brain || !f.manifest.JournalWatermark.IsZero() {
		t.Fatalf("pre-exclusion portable inventory: %+v", f.manifest)
	}
	for _, path := range []string{"manifest.json", f.manifest.ControlDB.RelativePath, f.manifest.Brains[0].RelativePath} {
		b, err := os.ReadFile(filepath.Join(f.snapshot, path))
		if err != nil {
			t.Fatal(err)
		}
		f.artifacts[path] = sha256.Sum256(b)
	}
	// Reopen the live source only to append the later exclusion through the
	// existing gateway into the exact independent journal supplied to Create.
	db, err := hoststore.Open(filepath.Join(dataDir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	g.Issuer.Store, g.Meter = db, &meter.Meter{Store: db}
	f.reopen(t)
	code, res, body := episodeHTTPCall(t, g, token, episodeExcludeRequest(brain, *f.original.Mutation))
	if code != 200 || res.Receipt == nil || res.PhysicalPurge != "unsupported" {
		t.Fatalf("external post-snapshot exclusion: %d %s", code, body)
	}
	f.excluded = res.Receipt
	if err = g.Pool.Close(); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *episodePortableRestore) restore(t *testing.T) {
	t.Helper()
	destination := filepath.Join(f.parent, "fresh-restored")
	if err := backup.Restore(context.Background(), f.snapshot, destination); err != nil {
		t.Fatalf("public backup.Restore: %v", err)
	}
	db, err := hoststore.Open(filepath.Join(destination, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	journal, err := deletion.NewFilesystemJournal(f.journalRoot, "synthetic-portable-reader", 1, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	f.g = &Gateway{Issuer: &credential.Issuer{Store: db}, Meter: &meter.Meter{Store: db}, Journal: journal,
		EpisodeArchiveEnabled: true, EpisodeSyntheticBrains: []string{f.brain}}
	f.root = filepath.Join(destination, "brains", f.brain)
	if got := strings.TrimSpace(string(chainGit(t, f.root, "rev-parse", "HEAD"))); got != f.oldHead {
		t.Fatalf("restored head = %s, want pre-exclusion %s", got, f.oldHead)
	}
	f.reopen(t)
	f.requireFrozen(t)
}

func (f *episodePortableRestore) requireFrozen(t *testing.T) {
	t.Helper()
	var total, pending, credentials, revoked int
	db := f.g.Issuer.Store.DB()
	if err := db.QueryRow(`SELECT count(*),count(CASE WHEN status='restore_pending' THEN 1 END) FROM accounts`).Scan(&total, &pending); err != nil {
		t.Fatal(err)
	}
	if total != 1 || pending != total {
		t.Fatalf("restored accounts: total=%d pending=%d", total, pending)
	}
	if err := db.QueryRow(`SELECT count(*),count(revoked_at) FROM client_credentials`).Scan(&credentials, &revoked); err != nil {
		t.Fatal(err)
	}
	if credentials == 0 || revoked != credentials {
		t.Fatalf("restored credentials: total=%d revoked=%d", credentials, revoked)
	}
	if _, err := f.g.Issuer.Verify(context.Background(), f.token); !errors.Is(err, credential.ErrRevoked) {
		t.Fatalf("old credential not revoked: %v", err)
	}
}

func TestEpisodePortableRestoreSealedReplayStaysFrozen(t *testing.T) {
	ctx := context.Background()
	f := newEpisodePortableRestore(t)
	f.seal(t) // Explicit fixture-owned trusted cut, separate from old manifest.
	f.restore(t)
	fences, err := f.replay(ctx)
	if err != nil || len(fences) != 1 {
		t.Fatalf("caller-trusted sealed replay: %v", err)
	}
	if fences[0].CommandKey != f.excluded.CommandKey || fences[0].RequestDigest != f.excluded.Digest || !fences[0].RecordedAt.Equal(f.excluded.RecordedAt) {
		t.Fatal("replay did not preserve the post-snapshot exclusion identity")
	}
	head := chainGit(t, f.root, "rev-parse", "HEAD")
	if retry, err := f.replay(ctx); err != nil || !reflect.DeepEqual(fences, retry) || !bytes.Equal(head, chainGit(t, f.root, "rev-parse", "HEAD")) {
		t.Fatalf("portable replay retry: %v", err)
	}
	// These are synthetic canonical reads AFTER replay, not admitted HTTP reads.
	// Restore-pending accounts are never activated, including for assertions.
	p, err := store.LoadEpisodes(ctx, store.NewSourceStore(f.root))
	if err != nil {
		t.Fatal(err)
	}
	if p.Generation != 1 || p.Fences[store.EpisodeConversationKey(f.original.Mutation.ProducerID, f.original.Mutation.ConversationID)] != 1 {
		t.Fatal("external exclusion absent from restored projection")
	}
	for _, receipt := range []store.EpisodeReceipt{f.originalReceipt, f.dependentReceipt} {
		r, exists := p.Records[receipt.CommandKey]
		if !exists || p.RecordEligible(r) {
			t.Fatalf("restored original/dependent eligible or missing: %s", receipt.CommandKey)
		}
		// Logical exclusion leaves canonical bytes and pre-exclusion Git objects.
		path := "brain/sources/" + receipt.SourceSHA256[:2] + "/" + receipt.SourceSHA256 + "/bytes"
		retained, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(path)))
		if err != nil || !bytes.Equal(retained, chainGit(t, f.root, "show", f.oldHead+":"+path)) {
			t.Fatalf("historical bytes physically erased or changed: %v", err)
		}
	}
	events := p.SortedEvents()
	sibling, exists := p.Records[f.siblingReceipt.CommandKey]
	if len(events) != 1 || !reflect.DeepEqual(events[0].Event, f.sibling.Mutation.Events[0]) || !exists || !p.RecordEligible(sibling) {
		t.Fatal("canonical retrieval did not retain only the independent survivor")
	}
	for _, req := range []episodeRequest{episodeReadRequest(f.brain, "context"), f.original, f.dependent} {
		if code, _, body := episodeHTTPCall(t, f.g, f.token, req); code != 401 {
			t.Fatalf("old credential admitted after restore: %d %s", code, body)
		}
	}
	// An existing auth-fixture seam lets a synthetic binding reach the actual
	// account check. It does NOT reactivate the account or issue a new credential.
	// Pending stale publications are refused there, before queue admission; this
	// does not claim generation/fence errors were reached behind the frozen gate.
	verifies := 0
	f.g.Issuer.OAuthVerify = func(_ context.Context, token string) (credential.Binding, error) {
		if token != "synthetic-portable-binding" {
			return credential.Binding{}, credential.ErrInvalidCredential
		}
		verifies++
		return f.binding, nil
	}
	for _, req := range []episodeRequest{episodeReadRequest(f.brain, "context"), f.original, f.dependent} {
		before := verifies
		if code, _, body := episodeHTTPCall(t, f.g, "synthetic-portable-binding", req); code != 401 || verifies-before != 2 {
			t.Fatalf("frozen account admitted pending publication: %d %s (verifies=%d)", code, body, verifies-before)
		}
	}
	f.g.Issuer.OAuthVerify = nil
	if !bytes.Equal(head, chainGit(t, f.root, "rev-parse", "HEAD")) {
		t.Fatal("refused publication changed canonical history")
	}
	f.requireFrozen(t)
	// The unfiltered hosted repository is still outside the erasure builder's
	// episode-only contract. Portable restore must not be promoted to purge.
	if err = f.g.Pool.Close(); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(f.parent, "unsupported-hosted-candidate")
	if _, err = episodeerase.Build(ctx, episodeerase.Request{Source: f.root, Destination: candidate, BrainID: f.brain, CurrentFences: fences}); !errors.Is(err, episodeerase.ErrUnsafe) {
		t.Fatalf("mixed hosted builder refusal: %v", err)
	}
	if _, err = os.Stat(candidate); !os.IsNotExist(err) {
		t.Fatalf("refused candidate published: %v", err)
	}
	for path, want := range f.artifacts {
		raw, err := os.ReadFile(filepath.Join(f.snapshot, path))
		if err != nil || sha256.Sum256(raw) != want {
			t.Fatalf("preserved portable artifact changed: %s: %v", path, err)
		}
	}
}

func TestEpisodePortableRestoreRefusesUntrustedHistory(t *testing.T) {
	for _, mode := range []string{"missing-journal", "missing-object", "wrong-cut", "unsealed", "conflicting"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			f := newEpisodePortableRestore(t)
			if mode == "conflicting" {
				history, err := f.journal.ReadThrough(ctx, contracts.DeletionWatermark{}) // inspection, not admission
				if err != nil || len(history.Entries) != 1 {
					t.Fatalf("external exclusion fixture: %v", err)
				}
				conflict := history.Entries[0]
				conflict.RecordedAt = conflict.RecordedAt.Add(time.Second)
				if _, err = f.journal.AppendDeletion(ctx, conflict); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "unsealed" {
				f.cut = contracts.DeletionWatermark{Generation: 1, SequenceID: 1, EntryHash: strings.Repeat("a", 64)}
			} else {
				f.seal(t)
			}
			f.restore(t)
			switch mode {
			case "missing-journal":
				f.g.Journal = nil
			case "missing-object":
				if err := os.Rename(filepath.Join(f.journalRoot, filepath.FromSlash(contracts.JournalKey(1, 1))), filepath.Join(f.parent, "withheld-journal-object")); err != nil {
					t.Fatal(err)
				}
			case "wrong-cut":
				f.cut.EntryHash = strings.Repeat("f", 64)
			}
			before := chainGit(t, f.root, "rev-parse", "HEAD")
			fences, err := f.replay(ctx)
			if err == nil || len(fences) != 0 {
				t.Fatalf("untrusted history admitted: %v", err)
			}
			if mode == "conflicting" && !errors.Is(err, store.ErrEpisodeConflict) {
				t.Fatalf("conflicting replay: %v", err)
			}
			if !bytes.Equal(before, chainGit(t, f.root, "rev-parse", "HEAD")) {
				t.Fatal("failed admission/replay changed restored history")
			}
			// No canonical read or builder call follows failed synthetic admission.
			// ReadThrough alone never authorizes provider/service admission.
			f.requireFrozen(t)
		})
	}
}
