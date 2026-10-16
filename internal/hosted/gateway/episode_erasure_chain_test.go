package gateway

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/episodeerase"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/deletion"
	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// This is a synthetic caller of the startup replay boundary, not service
// assembly or production recovery admission. The fixture owns an independent
// journal and explicitly trusts its exact seal. ReadThrough alone is NEVER
// admission. The Build source is the complete episode-only pre-hosting Git
// snapshot, not a filtered/reconstructed export of a mixed hosted repository.
// Pool startup adds hosted config/index state; that mixed repository remains
// outside Build's contract and is tested as a refusal below.
type episodeErasureChain struct {
	g                                                                  *Gateway
	brain, token, source, root, journalRoot                            string
	journal                                                            *deletion.Journal
	cut                                                                contracts.DeletionWatermark
	original, dependent, sibling                                       episodeRequest
	originalReceipt, reimportReceipt, dependentReceipt, siblingReceipt store.EpisodeReceipt
	excluded                                                           *store.EpisodeReceipt
}

func chainGit(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", args[0], err, out)
	}
	return out
}

// Fetch the entire single-branch history into a fresh standalone repository;
// no remote refs, alternates, local runtime files or history filtering.
func chainSnapshot(t *testing.T, from, to string) {
	t.Helper()
	if err := os.Mkdir(to, 0700); err != nil {
		t.Fatal(err)
	}
	chainGit(t, to, "init", "--initial-branch=main", "--object-format=sha1")
	chainGit(t, to, "fetch", "--no-tags", from, "refs/heads/main")
	chainGit(t, to, "checkout", "-B", "main", "FETCH_HEAD")
}

func newEpisodeErasureChain(t *testing.T) *episodeErasureChain {
	t.Helper()
	ctx := context.Background()
	g, p, brain, token := episodeHTTPSetup(t)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seed := filepath.Join(parent, "seed")
	if err = os.Mkdir(seed, 0700); err != nil {
		t.Fatal(err)
	}
	chainGit(t, seed, "init", "--initial-branch=main", "--object-format=sha1")
	chainGit(t, seed, "config", "user.name", "Synthetic chain")
	chainGit(t, seed, "config", "user.email", "chain@example.test")
	chainGit(t, seed, "commit", "--allow-empty", "-m", "Synthetic episode-only baseline")
	f := &episodeErasureChain{g: g, brain: brain, token: token, source: filepath.Join(parent, "pre-exclusion"), root: filepath.Join(p.BrainsRoot(), brain), journalRoot: filepath.Join(parent, "journal")}
	f.original = episodeHTTPFixture(t, brain)
	f.original.Mutation.Events[0].Text = "CHAIN-ORIGINAL-SECRET"
	f.sibling = episodeHTTPFixture(t, brain)
	f.sibling.Mutation.CommandID = "unrelated-command"
	f.sibling.Mutation.ConversationID = "unrelated-conversation"
	f.sibling.Mutation.Events[0].Text = "CHAIN-INDEPENDENT-SURVIVOR"
	q := writer.NewQueue(nil)
	defer q.Close()
	sources := store.NewSourceStore(seed)
	put := func(in store.EpisodeInput) store.EpisodeReceipt {
		t.Helper()
		r, err := writer.Episode(ctx, q, sources, seed, brain, 0, in, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	f.originalReceipt = put(*f.original.Mutation)
	reimport := *f.original.Mutation
	reimport.CommandID = "reimport-old-event"
	f.reimportReceipt = put(reimport)
	f.siblingReceipt = put(*f.sibling.Mutation)
	f.dependent = episodeHTTPFixture(t, brain)
	f.dependent.Operation = "checkpoint"
	f.dependent.Mutation.Operation = "checkpoint"
	f.dependent.Mutation.CommandID = "dependent-checkpoint"
	f.dependent.Mutation.ConversationID = "derived-conversation"
	f.dependent.Mutation.Events = nil
	f.dependent.Mutation.Checkpoint = &store.EpisodeCheckpoint{Text: "CHAIN-DERIVED-SECRET", Model: "synthetic", CreatedAt: f.original.Mutation.Events[0].ObservedAt, Sources: f.originalReceipt.EventRefs}
	f.dependentReceipt = put(*f.dependent.Mutation)
	chainSnapshot(t, seed, f.source)
	// Preserve the unused generic hosted fixture instead of altering its history.
	if err = os.Rename(f.root, filepath.Join(parent, "unused-hosted-fixture")); err != nil {
		t.Fatal(err)
	}
	chainSnapshot(t, f.source, f.root)
	f.reopen(t)
	f.journal, err = deletion.NewFilesystemJournal(f.journalRoot, "synthetic-chain", 1, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	g.Journal = f.journal
	exclusion := episodeExcludeRequest(brain, *f.original.Mutation)
	code, res, body := episodeHTTPCall(t, g, token, exclusion)
	if code != 200 || res.Receipt == nil || res.PhysicalPurge != "unsupported" {
		t.Fatalf("exclude: %d %s", code, body)
	}
	f.excluded = res.Receipt
	// Close the runtime, preserve its post-exclusion copy, then restore the exact
	// older Git snapshot. Journal storage is outside both snapshots.
	if err = g.Pool.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(f.root, filepath.Join(parent, "post-exclusion")); err != nil {
		t.Fatal(err)
	}
	chainSnapshot(t, f.source, f.root)
	f.reopen(t)
	return f
}

func (f *episodeErasureChain) reopen(t *testing.T) {
	t.Helper()
	chainGit(t, f.root, "config", "user.name", "Synthetic chain")
	chainGit(t, f.root, "config", "user.email", "chain@example.test")
	p, err := pool.New(pool.Config{MaxOpen: 1, MaxInFlight: 1, BrainsRoot: filepath.Dir(f.root), Embedder: journalTestEmbedder{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	})
	f.g.Pool = p
	f.g.mu.Lock()
	f.g.episodeReplay = nil
	f.g.mu.Unlock()
}

func (f *episodeErasureChain) seal(t *testing.T) {
	t.Helper()
	var err error
	f.cut, err = f.journal.Seal(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
}

// Trusted synthetic admission is the externally pinned seal plus the real
// journal's full-history chain verification, never a visible unsealed tail or
// local fence projection. Production authority/credential fencing is not tested.
func (f *episodeErasureChain) replay(ctx context.Context) ([]store.EpisodeRecord, error) {
	if f.g.Journal == nil || f.cut.IsZero() {
		return nil, contracts.ErrDeletionJournalIncomplete
	}
	history, err := f.g.Journal.ReadThrough(ctx, contracts.DeletionWatermark{})
	if err != nil {
		return nil, err
	}
	if !history.Sealed || history.To != f.cut {
		return nil, contracts.ErrDeletionJournalIncomplete
	}
	if _, err = f.g.ReplayEpisodeDeletions(ctx, history.Entries); err != nil {
		return nil, err
	}
	p, err := store.LoadEpisodesForReplay(ctx, store.NewSourceStore(f.root))
	if err != nil {
		return nil, err
	}
	var fences []store.EpisodeRecord
	seen := map[string]bool{}
	for _, e := range history.Entries {
		id, err := contracts.ParseEpisodeDeletionIdentity(e.SubjectID)
		if err != nil {
			return nil, err
		}
		if id.BrainHash != contracts.EpisodeDeletionBrainHash(f.brain) {
			return nil, store.ErrEpisodeConflict
		}
		r, ok := p.Records[id.CommandHash]
		if !ok || r.Kind != "exclude" || episodeDeletionIdentity(f.brain, r) != id || !r.RecordedAt.Equal(e.RecordedAt) {
			return nil, ErrEpisodeReplayPending
		}
		if !seen[id.CommandHash] {
			fences = append(fences, r)
			seen[id.CommandHash] = true
		}
	}
	if len(fences) == 0 || len(fences) != len(p.Fences) {
		return nil, ErrEpisodeReplayPending
	}
	return fences, nil
}

func TestEpisodeErasureChainSealedRestoreReplayAndInactiveCandidate(t *testing.T) {
	ctx := context.Background()
	f := newEpisodeErasureChain(t)
	f.seal(t)
	reopenedJournal, err := deletion.NewFilesystemJournal(f.journalRoot, "synthetic-recovery-reader", 1, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	f.g.Journal = reopenedJournal
	// Open the restored runtime before injecting a canonical publication failure,
	// so this fails replay rather than hosted config initialization.
	_, release, err := f.g.Pool.AcquireCanonical(ctx, f.brain)
	if err != nil {
		t.Fatal(err)
	}
	release()
	hook := filepath.Join(f.root, ".git", "hooks", "pre-commit")
	if err = os.MkdirAll(filepath.Dir(hook), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if failedFences, replayErr := f.replay(ctx); replayErr == nil || len(failedFences) != 0 {
		t.Fatal("failed replay yielded candidate authority")
	}
	if code, _, body := episodeHTTPCall(t, f.g, f.token, episodeReadRequest(f.brain, "context")); code != 503 {
		t.Fatalf("interrupted replay reopened reads: %d %s", code, body)
	}
	if err = os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	before := chainGit(t, f.source, "rev-parse", "HEAD")
	fences, err := f.replay(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Replay is idempotent across an interrupted caller/restart: discard runtime
	// and in-memory intents and repeat from the same independently admitted cut.
	replayHead := chainGit(t, f.root, "rev-parse", "HEAD")
	if err = f.g.Pool.Close(); err != nil {
		t.Fatal(err)
	}
	f.reopen(t)
	retryFences, err := f.replay(ctx)
	if err != nil || !reflect.DeepEqual(fences, retryFences) || !bytes.Equal(replayHead, chainGit(t, f.root, "rev-parse", "HEAD")) {
		t.Fatalf("replay retry: %v", err)
	}
	for _, receipt := range []store.EpisodeReceipt{f.originalReceipt, f.reimportReceipt, f.dependentReceipt} {
		read := episodeReadRequest(f.brain, "read")
		read.CommandKey = receipt.CommandKey
		if code, _, body := episodeHTTPCall(t, f.g, f.token, read); code != 404 || !strings.Contains(body, "record_unavailable") {
			t.Fatalf("old/derived read: %d %s", code, body)
		}
	}
	read := episodeReadRequest(f.brain, "read")
	read.EventKey = f.originalReceipt.EventRefs[0].EventKey
	if code, _, body := episodeHTTPCall(t, f.g, f.token, read); code != 404 {
		t.Fatalf("old event: %d %s", code, body)
	}
	read.CommandKey = f.siblingReceipt.CommandKey
	read.EventKey = ""
	if code, res, body := episodeHTTPCall(t, f.g, f.token, read); code != 200 || len(res.Events) != 1 || !reflect.DeepEqual(res.Events[0].Event, f.sibling.Mutation.Events[0]) {
		t.Fatalf("unrelated source: %d %s", code, body)
	}
	if code, _, body := episodeHTTPCall(t, f.g, f.token, f.original); code != 409 || !strings.Contains(body, "generation_mismatch") {
		t.Fatalf("stale generation: %d %s", code, body)
	}
	for _, operation := range []string{"context", "search"} {
		query := episodeReadRequest(f.brain, operation)
		query.Query = "CHAIN-"
		if code, res, body := episodeHTTPCall(t, f.g, f.token, query); code != 200 || len(res.Events) != 1 || res.Events[0].Event.Text != f.sibling.Mutation.Events[0].Text || strings.Contains(body, "CHAIN-ORIGINAL-SECRET") || strings.Contains(body, "CHAIN-DERIVED-SECRET") {
			t.Fatalf("restored retrieval: %d %s", code, body)
		}
	}
	generation := uint64(1)
	f.original.Generation = &generation
	if code, _, body := episodeHTTPCall(t, f.g, f.token, f.original); code != 410 || !strings.Contains(body, `"error":"excluded"`) {
		t.Fatalf("stale queue: %d %s", code, body)
	}
	f.dependent.Generation = &generation
	f.dependent.Mutation.CommandID = "new-dependent-checkpoint"
	if code, _, body := episodeHTTPCall(t, f.g, f.token, f.dependent); code != 410 {
		t.Fatalf("dependent checkpoint publication: %d %s", code, body)
	}
	exclusion := episodeExcludeRequest(f.brain, *f.original.Mutation)
	exclusion.Generation = &generation
	if code, res, body := episodeHTTPCall(t, f.g, f.token, exclusion); code != 200 || !reflect.DeepEqual(res.Receipt, f.excluded) || res.PhysicalPurge != "unsupported" {
		t.Fatalf("exclusion retry: %d %s", code, body)
	}
	parent := filepath.Dir(f.source)
	req := episodeerase.Request{Source: f.source, Destination: filepath.Join(parent, "candidate"), BrainID: f.brain, CurrentFences: fences}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = episodeerase.Build(canceled, req); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled build: %v", err)
	}
	if _, err = os.Stat(req.Destination); !os.IsNotExist(err) {
		t.Fatalf("canceled candidate published: %v", err)
	}
	result, err := episodeerase.Build(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Activation != "not_activated" || result.PhysicalPurge != "unsupported" || len(result.RemovedSources) != 3 {
		t.Fatalf("candidate disposition: %+v", result)
	}
	for _, sha := range []string{f.originalReceipt.SourceSHA256, f.reimportReceipt.SourceSHA256, f.dependentReceipt.SourceSHA256} {
		found := false
		for _, removed := range result.RemovedSources {
			found = found || removed == sha
		}
		if !found {
			t.Fatalf("missing removal disposition: %s", sha)
		}
	}
	chainInspectCandidate(t, f, req.Destination)
	if _, err = episodeerase.Build(ctx, req); err == nil {
		t.Fatal("existing candidate adopted")
	}
	req.Destination = filepath.Join(parent, "retry-candidate")
	retry, err := episodeerase.Build(ctx, req)
	if err != nil || retry.RetryID != result.RetryID || retry.CandidateHead != result.CandidateHead {
		t.Fatalf("fresh retry: %+v %v", retry, err)
	}
	if !bytes.Equal(before, chainGit(t, f.source, "rev-parse", "HEAD")) {
		t.Fatal("source history changed")
	}
	for _, sha := range []string{f.originalReceipt.SourceSHA256, f.reimportReceipt.SourceSHA256, f.dependentReceipt.SourceSHA256} {
		if _, err = os.Stat(filepath.Join(store.NewSourceStore(f.source).DirFor(sha), "bytes")); err != nil {
			t.Fatal("source purged", err)
		}
	}
	// No widening to mixed hosted history is hidden in this qualification.
	req.Source = f.root
	req.Destination = filepath.Join(parent, "unsupported-hosted-candidate")
	if _, err = episodeerase.Build(ctx, req); !errors.Is(err, episodeerase.ErrUnsafe) {
		t.Fatalf("mixed hosted source: %v", err)
	}
}

func chainInspectCandidate(t *testing.T, f *episodeErasureChain, destination string) {
	t.Helper()
	repo := filepath.Join(destination, "repository.git")
	// Metadata blobs may legitimately be identical to an independent survivor;
	// removed source paths and content blobs must disappear in every revision.
	oldBlobs := map[string]bool{}
	for _, sha := range []string{f.originalReceipt.SourceSHA256, f.reimportReceipt.SourceSHA256, f.dependentReceipt.SourceSHA256} {
		path := "brain/sources/" + sha[:2] + "/" + sha + "/bytes"
		oldBlobs[strings.TrimSpace(string(chainGit(t, f.source, "rev-parse", "HEAD:"+path)))] = true
	}
	objects := strings.Fields(string(chainGit(t, repo, "cat-file", "--batch-all-objects", "--batch-check=%(objectname)")))
	reachable := map[string]bool{}
	for _, id := range strings.Fields(string(chainGit(t, repo, "rev-list", "--objects", "--all", "--no-object-names"))) {
		reachable[id] = true
	}
	for _, id := range objects {
		if oldBlobs[id] || !reachable[id] {
			t.Fatalf("old or unreachable object: %s", id)
		}
		raw := chainGit(t, repo, "cat-file", "-p", id)
		if bytes.Contains(raw, []byte("CHAIN-ORIGINAL-SECRET")) || bytes.Contains(raw, []byte("CHAIN-DERIVED-SECRET")) {
			t.Fatalf("excluded content in object %s", id)
		}
	}
	siblingBase := "brain/sources/" + f.siblingReceipt.SourceSHA256[:2] + "/" + f.siblingReceipt.SourceSHA256 + "/"
	sourceRevisions := strings.Fields(string(chainGit(t, f.source, "rev-list", "--reverse", "main")))
	candidateRevisions := strings.Fields(string(chainGit(t, repo, "rev-list", "--reverse", "main")))
	if len(sourceRevisions) != len(candidateRevisions) {
		t.Fatal("candidate lost historical revisions")
	}
	for i, rev := range candidateRevisions {
		tree := string(chainGit(t, repo, "ls-tree", "-r", "--name-only", rev))
		if strings.Contains(tree, f.originalReceipt.SourceSHA256) || strings.Contains(tree, f.reimportReceipt.SourceSHA256) || strings.Contains(tree, f.dependentReceipt.SourceSHA256) {
			t.Fatal("historical source retained")
		}
		// Every revision carries current external fences, including those predating
		// the unrelated source; once present, both canonical source files are exact.
		checkout := filepath.Join(t.TempDir(), "revision")
		if err := os.Mkdir(checkout, 0700); err != nil {
			t.Fatal(err)
		}
		for _, path := range strings.Fields(tree) {
			full := filepath.Join(checkout, filepath.FromSlash(path))
			if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, chainGit(t, repo, "show", rev+":"+path), 0600); err != nil {
				t.Fatal(err)
			}
		}
		projection, err := store.LoadEpisodes(context.Background(), store.NewSourceStore(checkout))
		if err != nil || projection.Generation != 1 || projection.Fences[store.EpisodeConversationKey(f.original.Mutation.ProducerID, f.original.Mutation.ConversationID)] != 1 {
			t.Fatalf("historical fence: %v", err)
		}
		sourceTree := string(chainGit(t, f.source, "ls-tree", "-r", "--name-only", sourceRevisions[i]))
		if strings.Contains(tree, siblingBase) != strings.Contains(sourceTree, siblingBase) {
			t.Fatal("unrelated historical source presence changed")
		}
		if strings.Contains(tree, siblingBase) {
			for _, name := range []string{"bytes", "meta.yaml"} {
				if !bytes.Equal(chainGit(t, repo, "show", rev+":"+siblingBase+name), chainGit(t, f.source, "show", "HEAD:"+siblingBase+name)) {
					t.Fatal("unrelated canonical bytes changed")
				}
			}
		}
	}
	if !strings.Contains(string(chainGit(t, repo, "ls-tree", "-r", "--name-only", "HEAD")), siblingBase) {
		t.Fatal("unrelated source absent at candidate head")
	}
}

func TestEpisodeErasureChainRejectsUnadmittedOrConflictingHistory(t *testing.T) {
	for _, mode := range []string{"missing-journal", "unsealed", "wrong-cut", "missing-object", "conflicting", "local-fence-only", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			f := newEpisodeErasureChain(t)
			ctx := context.Background()
			if mode == "conflicting" {
				history, err := f.journal.ReadThrough(ctx, contracts.DeletionWatermark{}) // inspection only, NOT admission
				if err != nil || len(history.Entries) != 1 {
					t.Fatalf("fixture intent: %v", err)
				}
				conflict := history.Entries[0]
				conflict.RecordedAt = conflict.RecordedAt.Add(time.Second)
				if _, err = f.journal.AppendDeletion(ctx, conflict); err != nil {
					t.Fatal(err)
				}
			}
			if mode != "unsealed" {
				f.seal(t)
			} else {
				// Even a nonzero claimed cut cannot promote an unsealed read to authority.
				f.cut = contracts.DeletionWatermark{Generation: 1, SequenceID: 1, EntryHash: strings.Repeat("a", 64)}
			}
			switch mode {
			case "local-fence-only":
				if _, err := f.replay(ctx); err != nil {
					t.Fatal(err)
				}
				empty, err := deletion.NewFilesystemJournal(t.TempDir(), "empty-authority", 1, time.Now)
				if err != nil {
					t.Fatal(err)
				}
				f.cut, err = empty.Seal(ctx, 1)
				if err != nil {
					t.Fatal(err)
				}
				f.g.Journal = empty
				if err = f.g.Pool.Close(); err != nil {
					t.Fatal(err)
				}
				f.reopen(t)
			case "missing-journal":
				f.g.Journal = nil
			case "wrong-cut":
				f.cut.EntryHash = strings.Repeat("f", 64)
			case "missing-object":
				path := filepath.Join(f.journalRoot, filepath.FromSlash(contracts.JournalKey(1, 1)))
				if err := os.Rename(path, filepath.Join(filepath.Dir(f.journalRoot), "withheld-journal-object")); err != nil {
					t.Fatal(err)
				}
			case "canceled":
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = canceled
			}
			fences, err := f.replay(ctx)
			if err == nil || len(fences) != 0 {
				t.Fatalf("unsafe history yielded candidate authority: %v %+v", err, fences)
			}
			if mode == "conflicting" && !errors.Is(err, store.ErrEpisodeConflict) {
				t.Fatalf("conflicting history: %v", err)
			}
			// The owning startup caller must not install handlers or call Build after
			// this failure; the raw gateway API cannot enforce service admission itself.
			if _, err = os.Stat(filepath.Join(filepath.Dir(f.source), "candidate")); !os.IsNotExist(err) {
				t.Fatal("candidate exists after failed admission")
			}
		})
	}
}
