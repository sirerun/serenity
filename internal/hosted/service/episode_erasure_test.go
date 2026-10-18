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
