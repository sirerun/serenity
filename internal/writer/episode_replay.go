package writer

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/gitrun"
	"github.com/sirerun/serenity/internal/store"
)

// EpisodeReplayFence installs an admitted journal intent without originals.
// The caller owns maintenance/account/runtime authority. Generation is derived
// from the current canonical projection, never a restored database watermark.
func EpisodeReplayFence(ctx context.Context, q *Queue, s *store.SourceStore, root string, fence store.EpisodeRecord) (store.EpisodeReceipt, error) {
	return episodeFence(ctx, q, s, root, fence, nil, nil, true)
}

// EpisodeJournaledExclude publishes only after intent has been attempted inside
// the serialized queue. An ambiguous journal failure still installs a local
// quarantine fence, but returns failure. That fence is NOT restore protection.
// intent must return success only for an independently durable matching entry.
func EpisodeJournaledExclude(ctx context.Context, q *Queue, s *store.SourceStore, root, brain string, generation uint64, in store.EpisodeInput, recheck func(context.Context) error, intent func(*store.EpisodeRecord) error) (store.EpisodeReceipt, error) {
	if in.Operation != "exclude" || intent == nil {
		return store.EpisodeReceipt{}, store.ErrEpisodeInvalid
	}
	if err := store.ValidateEpisodeInput(in); err != nil {
		return store.EpisodeReceipt{}, err
	}
	fence := store.EpisodeRecord{Schema: store.EpisodeSchema, BrainID: brain, Kind: "exclude", CommandKey: store.EpisodeCommandKey(in.ProducerID, in.CommandID), ConversationKey: store.EpisodeConversationKey(in.ProducerID, in.ConversationID), RequestDigest: store.EpisodeHash(in), RecordedAt: time.Now().UTC()}
	check := func(p *store.EpisodeProjection) error {
		if recheck != nil {
			if err := recheck(ctx); err != nil {
				return err
			}
		}
		if generation != p.Generation {
			return store.ErrEpisodeGeneration
		}
		return nil
	}
	return episodeFence(ctx, q, s, root, fence, check, intent, false)
}

func episodeFence(ctx context.Context, q *Queue, s *store.SourceStore, root string, fence store.EpisodeRecord, check func(*store.EpisodeProjection) error, intent func(*store.EpisodeRecord) error, replay bool) (store.EpisodeReceipt, error) {
	if fence.Schema != store.EpisodeSchema || fence.Kind != "exclude" || !store.ValidEpisodeID(fence.BrainID) || !store.ValidSourceSHA(fence.CommandKey) || !store.ValidSourceSHA(fence.ConversationKey) || !store.ValidSourceSHA(fence.RequestDigest) || fence.RecordedAt.IsZero() || fence.Input != nil || len(fence.Refs) != 0 {
		return store.EpisodeReceipt{}, store.ErrEpisodeInvalid
	}
	load := store.LoadEpisodes
	published := EpisodesPublished
	if replay {
		load = store.LoadEpisodesForReplay
		published = episodeReplayPublished
	}
	var receipt store.EpisodeReceipt
	var journalErr error
	result := q.SubmitAndFlush(ctx, root, Job{Kind: "episode", Render: func() ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		p, err := load(ctx, s)
		if err != nil {
			return nil, err
		}
		if p.BrainID != "" && p.BrainID != fence.BrainID {
			return nil, store.ErrEpisodeConflict
		}
		if check != nil {
			if err = check(p); err != nil {
				return nil, err
			}
		}
		old, exists := p.Records[fence.CommandKey]
		if exists {
			if old.Kind != "exclude" || old.RequestDigest != fence.RequestDigest || old.ConversationKey != fence.ConversationKey {
				return nil, store.ErrEpisodeConflict
			}
			if check == nil && !old.RecordedAt.Equal(fence.RecordedAt) {
				return nil, store.ErrEpisodeConflict
			}
			// An existing fence's time and generation are immutable, including retries.
			fence = old
		} else {
			if p.Fences[fence.ConversationKey] != 0 {
				return nil, store.ErrEpisodeFenced
			}
			fence.Generation = p.Generation + 1
		}
		if intent != nil {
			journalErr = intent(&fence)
		}
		// Even at the reserved fence limit, the external intent must not be lost.
		// Returning failure leaves the brain closed until reconciliation is possible.
		if !exists && len(p.Fences) >= store.EpisodeMaxFences {
			return nil, store.ErrEpisodeLimit
		}
		src, err := s.PutEpisode(fence)
		if err != nil {
			return nil, err
		}
		q.MarkTouched(filepath.Join(s.DirFor(src.SHA256), "bytes"))
		q.MarkTouched(filepath.Join(s.DirFor(src.SHA256), "meta.yaml"))
		p.Records[fence.CommandKey] = fence
		p.SourceIDs[fence.CommandKey] = src.SHA256
		receipt = p.Receipt(fence.CommandKey)
		return nil, nil
	}})
	if err := errors.Join(journalErr, result.Result.Err); err != nil {
		return store.EpisodeReceipt{}, err
	}
	p, err := load(ctx, s)
	if err != nil {
		return store.EpisodeReceipt{}, err
	}
	if err = published(ctx, root, p); err != nil {
		return store.EpisodeReceipt{}, err
	}
	return receipt, nil
}

// Replay can encounter large ordinary inventories. Limit Git output to the
// bounded episode projection instead of listing every ordinary source path.
func episodeReplayPublished(ctx context.Context, root string, p *store.EpisodeProjection) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(p.SourceIDs) == 0 {
		return nil
	}
	paths := make(map[string]bool, len(p.SourceIDs)*2)
	args := []string{"ls-tree", "-r", "--name-only", "HEAD", "--"}
	for _, sha := range p.SourceIDs {
		base := "brain/sources/" + sha[:2] + "/" + sha + "/"
		for _, name := range []string{"bytes", "meta.yaml"} {
			path := base + name
			paths[path] = false
			args = append(args, path)
		}
	}
	runner := gitrun.CanonicalReadOnly(root)
	out, err := runner.Output(ctx, args...)
	if err != nil {
		return err
	}
	for _, path := range strings.Split(string(out), "\n") {
		if _, ok := paths[path]; ok {
			paths[path] = true
		}
	}
	diff := []string{"diff", "--quiet", "HEAD", "--"}
	for path, present := range paths {
		if !present {
			return store.ErrEpisodeUnpublished
		}
		diff = append(diff, path)
	}
	if _, err = runner.Output(ctx, diff...); err != nil {
		return store.ErrEpisodeUnpublished
	}
	return ctx.Err()
}
