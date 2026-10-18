package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/episodeerase"
	"github.com/sirerun/serenity/internal/gitrun"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/episodemaintenance"
	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

type preparationSource struct {
	snapshot episodemaintenance.Snapshot
	err      error
}

func (p *preparationSource) Observe(context.Context) (episodemaintenance.Snapshot, error) {
	return p.snapshot, p.err
}
func episodePreparationFixture(t *testing.T) (*Service, EpisodeErasureRequest, *preparationSource, string) {
	t.Helper()
	ctx := context.Background()
	db, cfg, account, brain := episodeReplayServiceFixture(t)
	source := &preparationSource{}
	deps := testLifecycleDependencies(t)
	deps.EpisodeMaintenance = source
	svc, err := AssembleWithDependencies(ctx, cfg, true, db, nil, deletionEmbedding{}, deps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	runtime, release, err := svc.Pool.AcquireCanonical(ctx, brain)
	if err != nil {
		t.Fatal(err)
	}
	root := runtime.Root
	release()
	if err = svc.Pool.Drop(brain); err != nil {
		t.Fatal(err)
	}
	entry := episodeReplayEntry(brain)
	id, err := contracts.ParseEpisodeDeletionIdentity(entry.SubjectID)
	if err != nil {
		t.Fatal(err)
	}
	fence := store.EpisodeRecord{Schema: store.EpisodeSchema, BrainID: brain, Kind: "exclude", CommandKey: id.CommandHash, ConversationKey: id.ConversationHash, RequestDigest: id.RequestDigest, Generation: 1, RecordedAt: entry.RecordedAt}
	if _, err = store.NewSourceStore(root).PutEpisode(fence); err != nil {
		t.Fatal(err)
	}
	episodeReplayGit(t, "-C", root, "add", "--", "brain/sources")
	episodeReplayGit(t, "-C", root, "commit", "-m", "synthetic canonical fence")
	config, err := os.ReadFile(filepath.Join(root, "serenity.yml"))
	if err != nil {
		t.Fatal(err)
	}
	head := preparationHead(t, root)
	entry.Watermark = contracts.DeletionWatermark{Generation: 1, SequenceID: 1, EntryHash: strings.Repeat("a", 64)}
	source.snapshot = episodemaintenance.Snapshot{CutID: strings.Repeat("d", 64), Observation: contracts.JournalObservation{Read: contracts.DeletionRead{Entries: []contracts.DeletionEntry{entry}, To: entry.Watermark}, HistorySHA256: strings.Repeat("b", 64)}, Recheck: func(context.Context) error { return nil }}
	req := EpisodeErasureRequest{AccountID: account, BrainID: brain, OperationID: strings.Repeat("c", 64), ExpectedSourceHead: head, IndependentPaths: []episodeerase.PathPin{{Path: ".gitignore", SHA256: store.EpisodeHash(nil)}, {Path: "serenity.yml", SHA256: store.EpisodeHash(nil)}}}
	req.IndependentPaths[0].SHA256 = preparationHash([]byte(".serenity/\n"))
	req.IndependentPaths[1].SHA256 = preparationHash(config)
	return svc, req, source, root
}
func TestEpisodeErasurePreparationInactiveAndNoAdoption(t *testing.T) {
	svc, req, _, root := episodePreparationFixture(t)
	result, err := svc.PrepareEpisodeErasure(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.ResidualInactive || result.CutID != strings.Repeat("d", 64) || result.Candidate.Activation != "not_activated" || result.Candidate.PhysicalPurge != "unsupported" {
		t.Fatalf("bad inactive result %+v", result)
	}
	if head := preparationHead(t, root); head != req.ExpectedSourceHead {
		t.Fatal("source changed")
	}
	if _, err = svc.PrepareEpisodeErasure(context.Background(), req); err == nil {
		t.Fatal("adopted existing operation")
	}
}
func TestEpisodeErasurePreparationRefusals(t *testing.T) {
	for _, name := range []string{"missing_source", "missing_history", "extra_intent", "duplicate", "bad_order", "bad_hash", "bad_cut", "wrong_brain", "wrong_head", "unavailable", "wrong_account", "writer_contention", "pool_busy", "post_cut_drift", "post_authority_change", "whole_brain", "unsafe_parent"} {
		t.Run(name, func(t *testing.T) {
			svc, req, source, root := episodePreparationFixture(t)
			var release func()
			switch name {
			case "missing_source":
				svc.episodeMaintenance = nil
			case "missing_history":
				source.snapshot.Observation.Read.Entries = nil
			case "extra_intent":
				e := source.snapshot.Observation.Read.Entries[0]
				id, _ := contracts.ParseEpisodeDeletionIdentity(e.SubjectID)
				id.CommandHash = strings.Repeat("f", 64)
				e.SubjectID = id.String()
				e.Watermark.SequenceID = 2
				source.snapshot.Observation.Read.Entries = append(source.snapshot.Observation.Read.Entries, e)
			case "duplicate", "bad_order":
				source.snapshot.Observation.Read.Entries = append(source.snapshot.Observation.Read.Entries, source.snapshot.Observation.Read.Entries[0])
			case "bad_cut":
				source.snapshot.CutID = "not-a-pin"
			case "bad_hash":
				source.snapshot.Observation.Read.Entries[0].Watermark.EntryHash = "bad"
			case "wrong_brain":
				req.BrainID = "foreign"
			case "wrong_account":
				req.AccountID = "foreign"
			case "wrong_head":
				req.ExpectedSourceHead = strings.Repeat("0", 40)
			case "unavailable":
				source.err = errors.New("authority unavailable")
			case "writer_contention":
				owner, err := writer.AcquireBrain(root)
				if err != nil {
					t.Fatal(err)
				}
				release = func() { _ = owner.Close() }
			case "pool_busy":
				_, r, err := svc.Pool.AcquireCanonical(context.Background(), req.BrainID)
				if err != nil {
					t.Fatal(err)
				}
				release = r
			case "post_cut_drift":
				n := 0
				source.snapshot.Recheck = func(context.Context) error {
					n++
					if n == 2 {
						return errors.New("cut changed")
					}
					return nil
				}
			case "post_authority_change":
				n := 0
				source.snapshot.Recheck = func(context.Context) error {
					n++
					if n == 2 {
						_, err := svc.Store.DB().Exec("UPDATE accounts SET status='deleting' WHERE id=?", req.AccountID)
						if err != nil {
							return err
						}
						return errors.New("authority changed")
					}
					return nil
				}
			case "whole_brain":
				e := source.snapshot.Observation.Read.Entries[0]
				e.SubjectType = contracts.DeletionSubjectBrain
				e.SubjectID = req.BrainID
				source.snapshot.Observation.Read.Entries = []contracts.DeletionEntry{e}
			case "unsafe_parent":
				if err := os.Symlink(root, filepath.Join(svc.cfg.DataDir, ".episode-erasure")); err != nil {
					t.Fatal(err)
				}
			}
			if release != nil {
				defer release()
			}
			result, err := svc.PrepareEpisodeErasure(context.Background(), req)
			if err == nil {
				t.Fatal("unsafe accepted")
			}
			if name == "post_cut_drift" || name == "post_authority_change" {
				if !result.ResidualInactive || result.Candidate.CandidateHead == "" {
					t.Fatal("lost residual candidate evidence")
				}
			}
		})
	}
}

func preparationHash(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func preparationHead(t *testing.T, root string) string {
	t.Helper()
	raw, err := gitrun.CanonicalReadOnly(root).Output(context.Background(), "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func TestEpisodeErasurePreparationCancelledMaintenanceWait(t *testing.T) {
	svc, req, _, _ := episodePreparationFixture(t)
	svc.Gateway.Maintenance.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err := svc.PrepareEpisodeErasure(ctx, req)
	svc.Gateway.Maintenance.Unlock()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("maintenance wait: %v", err)
	}
	if _, err := svc.PrepareEpisodeErasure(nil, req); err == nil {
		t.Fatal("nil context accepted")
	}
}

func TestEpisodeErasurePreparationRetainsAttributedFailedStage(t *testing.T) {
	svc, req, _, _ := episodePreparationFixture(t)
	secret := "PRIVATE-STAGE-SENTINEL"
	failed := errors.New("synthetic interrupted preparation")
	result, err := svc.prepareEpisodeErasure(context.Background(), req, func(_ context.Context, b episodeerase.HostedRequest) (episodeerase.Result, error) {
		container := filepath.Dir(b.Destination)
		if filepath.Base(container) != req.OperationID || filepath.Base(b.Destination) != "candidate" {
			t.Fatal("stage is not operation-owned")
		}
		stage := filepath.Join(container, ".episode-erase-stage-retained")
		if err := os.Mkdir(stage, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stage, "bytes"), []byte(secret), 0600); err != nil {
			t.Fatal(err)
		}
		return episodeerase.Result{}, failed
	})
	if !errors.Is(err, failed) || !result.ResidualInactive || result.OperationID != req.OperationID || result.CutID == "" || result.HistorySHA256 == "" || result.Candidate.CandidateHead != "" {
		t.Fatalf("lost conservative residual evidence: %+v %v", result, err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("failure disclosed staged payload")
	}
	container := filepath.Join(svc.cfg.DataDir, ".episode-erasure", req.OperationID)
	if info, err := os.Stat(container); err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("operation container absent or not private")
	}
	if _, err := os.Stat(filepath.Join(container, ".episode-erase-stage-retained", "bytes")); err != nil {
		t.Fatal("unknown stage was silently discarded")
	}
	if _, err := os.Stat(filepath.Join(container, "candidate")); !os.IsNotExist(err) {
		t.Fatal("fixture unexpectedly published final candidate")
	}
	if _, err := svc.PrepareEpisodeErasure(context.Background(), req); err == nil {
		t.Fatal("retried/adopted incomplete operation")
	}
}

func TestEpisodeErasurePreparationCancelsAtRuntimeCommitGate(t *testing.T) {
	svc, req, _, root := episodePreparationFixture(t)
	runtime, releaseUser, err := svc.Pool.AcquireCanonical(context.Background(), req.BrainID)
	if err != nil {
		t.Fatal(err)
	}
	releaseGate, err := runtime.Fence(context.Background(), req.BrainID)
	if err != nil {
		releaseUser()
		t.Fatal(err)
	}
	held := true
	defer func() {
		if held {
			releaseGate()
		}
	}()
	releaseUser()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	started := time.Now()
	result, err := svc.PrepareEpisodeErasure(ctx, req)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > time.Second {
		t.Fatalf("commit gate did not cancel promptly: %v", err)
	}
	if result.ResidualInactive || result.OperationID != "" {
		t.Fatal("cancelled before preparation but claimed copy")
	}
	if _, err := os.Stat(filepath.Join(svc.cfg.DataDir, ".episode-erasure", req.OperationID)); !os.IsNotExist(err) {
		t.Fatal("cancelled flush created operation")
	}
	if !svc.Gateway.Maintenance.TryLock() {
		t.Fatal("maintenance lock leaked")
	}
	svc.Gateway.Maintenance.Unlock()
	if owner, err := writer.AcquireBrain(root); err == nil {
		_ = owner.Close()
		t.Fatal("cancelled flush lost runtime writer ownership")
	}
	if _, release, err := svc.Pool.AcquireCanonical(context.Background(), req.BrainID); !errors.Is(err, pool.ErrRuntimeClosing) {
		if err == nil {
			release()
		}
		t.Fatalf("cancelled target must remain parked: %v", err)
	}
	releaseGate()
	held = false
	if _, err := svc.PrepareEpisodeErasure(context.Background(), req); err != nil {
		t.Fatalf("retry after gate release: %v", err)
	}
}

func TestEpisodeErasurePreparationJoinsActualHTTPArchiveAndJournal(t *testing.T) {
	ctx := context.Background()
	db, cfg, account, brain := episodeReplayServiceFixture(t)
	source := &preparationSource{}
	deps := testLifecycleDependencies(t)
	deps.EpisodeMaintenance = source
	svc, err := AssembleWithDependencies(ctx, cfg, true, db, nil, deletionEmbedding{}, deps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	token, err := svc.Gateway.Issuer.Issue(ctx, brain, account, []string{"memory:read", "memory:write"})
	if err != nil {
		t.Fatal(err)
	}
	original := episodeReplayAppend(t, brain)
	const originalSecret = "JOINED-ORIGINAL-ERASURE-SENTINEL"
	const checkpointSecret = "JOINED-CHECKPOINT-ERASURE-SENTINEL"
	original.Mutation.Events[0].Text = originalSecret
	code, appended, _ := episodeReplayCall(t, svc, token, original)
	if code != 200 || appended.Receipt == nil {
		t.Fatalf("append failed HTTP%d", code)
	}
	checkpoint := episodeReplayAppend(t, brain)
	checkpoint.Operation = "checkpoint"
	checkpoint.Mutation.Operation = "checkpoint"
	checkpoint.Mutation.CommandID = "joined-checkpoint"
	checkpoint.Mutation.ConversationID = "joined-dependent"
	checkpoint.Mutation.Events = nil
	checkpoint.Mutation.Checkpoint = &store.EpisodeCheckpoint{Text: checkpointSecret, Model: "synthetic-fixture", CreatedAt: original.Mutation.Events[0].ObservedAt, Sources: appended.Receipt.EventRefs}
	code, checkpointed, _ := episodeReplayCall(t, svc, token, checkpoint)
	if code != 200 || checkpointed.Receipt == nil {
		t.Fatalf("checkpoint failed HTTP%d", code)
	}
	exclude := episodeReplayAppend(t, brain)
	exclude.Operation = "exclude"
	exclude.Mutation = &store.EpisodeInput{Operation: "exclude", ProducerID: original.Mutation.ProducerID, ConversationID: original.Mutation.ConversationID, CommandID: "joined-exclude"}
	code, excluded, _ := episodeReplayCall(t, svc, token, exclude)
	if code != 200 || excluded.Status != "excluded" {
		t.Fatalf("exclusion failed HTTP%d", code)
	}
	history, err := deps.Journal.ReadThrough(ctx, contracts.DeletionWatermark{})
	if err != nil || len(history.Entries) != 1 {
		t.Fatalf("actual journal unavailable: %v", err)
	}
	source.snapshot = episodemaintenance.Snapshot{CutID: strings.Repeat("d", 64), Observation: contracts.JournalObservation{Read: history, HistorySHA256: strings.Repeat("b", 64)}, Recheck: func(context.Context) error { return nil }}
	root := filepath.Join(cfg.DataDir, "brains", brain)
	head := preparationHead(t, root)
	config, err := os.ReadFile(filepath.Join(root, "serenity.yml"))
	if err != nil {
		t.Fatal(err)
	}
	req := EpisodeErasureRequest{AccountID: account, BrainID: brain, OperationID: strings.Repeat("e", 64), ExpectedSourceHead: head, IndependentPaths: []episodeerase.PathPin{{Path: ".gitignore", SHA256: preparationHash([]byte(".serenity/\n"))}, {Path: "serenity.yml", SHA256: preparationHash(config)}}}
	result, err := svc.PrepareEpisodeErasure(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	removed := map[string]bool{}
	for _, sha := range result.Candidate.RemovedSources {
		removed[sha] = true
	}
	if !removed[appended.Receipt.SourceSHA256] || !removed[checkpointed.Receipt.SourceSHA256] {
		t.Fatal("original/checkpoint closure missing")
	}
	repo := filepath.Join(cfg.DataDir, ".episode-erasure", req.OperationID, "candidate", "repository.git")
	reader := gitrun.CanonicalReadOnly(repo)
	objects, err := reader.Output(ctx, "cat-file", "--batch-all-objects", "--batch-check=%(objectname) %(objecttype)")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(objects)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatal("invalid object inventory")
		}
		raw, err := reader.Output(ctx, "cat-file", fields[1], fields[0])
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), originalSecret) || strings.Contains(string(raw), checkpointSecret) {
			t.Fatal("excluded source or derivative survived candidate history")
		}
	}
	if preparationHead(t, root) != head {
		t.Fatal("preparation changed active canonical history")
	}
	if !svc.cfg.EpisodeArchiveEnabled || len(svc.cfg.EpisodeSyntheticBrains) != 1 || svc.cfg.EpisodeSyntheticBrains[0] != brain {
		t.Fatal("preparation changed capture policy")
	}
	if result.Candidate.Activation != "not_activated" || result.Candidate.PhysicalPurge != "unsupported" || result.ResidualInactive {
		t.Fatal("wrong inactive preparation boundary")
	}
}
