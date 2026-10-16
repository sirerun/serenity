package writer

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/gitrun"
	"github.com/sirerun/serenity/internal/store"
)

// Episode publishes originals and content-free replay/fence metadata through
// the existing serialized queue. The hosted caller also holds Mutations.
// Recheck is a current authority check executed inside the publication section.
func Episode(ctx context.Context, q *Queue, s *store.SourceStore, root, brain string, generation uint64, in store.EpisodeInput, recheck func(context.Context) error) (store.EpisodeReceipt, error) {
	var receipt store.EpisodeReceipt
	if err := store.ValidateEpisodeInput(in); err != nil {
		return receipt, err
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return receipt, err
	}
	if len(raw) > store.EpisodeMaxRecordBytes-4096 {
		return receipt, store.ErrEpisodeLimit
	}
	// Detach mutable slices from the caller before queue admission.
	var detached store.EpisodeInput
	if err = json.Unmarshal(raw, &detached); err != nil {
		return receipt, err
	}
	in = detached
	key := store.EpisodeCommandKey(in.ProducerID, in.CommandID)
	conversation := store.EpisodeConversationKey(in.ProducerID, in.ConversationID)
	digest := store.EpisodeHash(in)
	result := q.SubmitAndFlush(ctx, root, Job{Kind: "episode", Render: func() ([]byte, error) {
		if recheck != nil {
			if err := recheck(ctx); err != nil {
				return nil, err
			}
		}
		p, err := store.LoadEpisodes(ctx, s)
		if err != nil {
			return nil, err
		}
		if p.BrainID != "" && p.BrainID != brain {
			return nil, store.ErrEpisodeConflict
		}
		if generation != p.Generation {
			return nil, store.ErrEpisodeGeneration
		}
		if old, ok := p.Records[key]; ok {
			if old.RequestDigest != digest {
				return nil, store.ErrEpisodeConflict
			}
			if old.Kind != "exclude" && !p.RecordEligible(old) {
				return nil, store.ErrEpisodeFenced
			}
			sha := p.SourceIDs[key]
			q.MarkTouched(filepath.Join(s.DirFor(sha), "bytes"))
			q.MarkTouched(filepath.Join(s.DirFor(sha), "meta.yaml"))
			receipt = p.Receipt(key)
			return nil, nil
		}
		if p.Fences[conversation] != 0 {
			return nil, store.ErrEpisodeFenced
		}
		r := store.EpisodeRecord{Schema: store.EpisodeSchema, BrainID: brain, Kind: in.Operation, CommandKey: key, ConversationKey: conversation, RequestDigest: digest, Generation: p.Generation, RecordedAt: time.Now().UTC()}
		if in.Operation == "exclude" {
			if len(p.Fences) >= store.EpisodeMaxFences {
				return nil, store.ErrEpisodeLimit
			}
			r.Generation++
		} else {
			if p.DataRecords >= store.EpisodeMaxRecords {
				return nil, store.ErrEpisodeLimit
			}
			copied := in
			copied.Events = nil
			r.Input = &copied
			sequences := map[int64]bool{}
			for _, e := range p.Events {
				if store.EpisodeConversationKey(e.ProducerID, e.ConversationID) == conversation {
					sequences[e.Event.Sequence] = true
				}
			}

			for _, e := range in.Events {
				eventKey := store.EpisodeEventKey(in.ProducerID, in.ConversationID, e.ID)
				ref := store.EpisodeRef{EventKey: eventKey, Digest: store.EpisodeHash(e)}
				if old, ok := p.Events[eventKey]; ok {
					if old.Ref.Digest != ref.Digest {
						return nil, store.ErrEpisodeConflict
					}
				} else {
					if sequences[e.Sequence] {
						return nil, store.ErrEpisodeConflict
					}
					sequences[e.Sequence] = true
					r.Input.Events = append(r.Input.Events, e)
				}
				r.Refs = append(r.Refs, ref)
			}
			if in.Checkpoint != nil {
				for _, ref := range in.Checkpoint.Sources {
					e, ok := p.Events[ref.EventKey]
					if !ok || e.Ref.Digest != ref.Digest {
						return nil, store.ErrEpisodeConflict
					}
					if !p.EventEligible(e) {
						return nil, store.ErrEpisodeFenced
					}
				}
			}
			b, err := json.Marshal(r)
			if err != nil {
				return nil, err
			}
			if p.Bytes+int64(len(b)) > store.EpisodeMaxArchiveBytes {
				return nil, store.ErrEpisodeLimit
			}
		}
		src, err := s.PutEpisode(r)
		if err != nil {
			return nil, err
		}
		q.MarkTouched(filepath.Join(s.DirFor(src.SHA256), "bytes"))
		q.MarkTouched(filepath.Join(s.DirFor(src.SHA256), "meta.yaml"))
		p.Records[key] = r
		p.SourceIDs[key] = src.SHA256
		receipt = p.Receipt(key)
		return nil, nil
	}})
	if result.Result.Err != nil {
		return store.EpisodeReceipt{}, result.Result.Err
	}
	// Publication failure is never acknowledged, including ignored source paths.
	p, err := store.LoadEpisodes(ctx, s)
	if err != nil {
		return store.EpisodeReceipt{}, err
	}
	if err = EpisodesPublished(ctx, root, p); err != nil {
		return store.EpisodeReceipt{}, err
	}
	return receipt, nil
}

// EpisodesPublished refuses dirty/unpublished source bytes and metadata. A
// working-tree record is not a receipt until both files are in canonical HEAD.
func EpisodesPublished(ctx context.Context, root string, p *store.EpisodeProjection) error {
	if len(p.SourceIDs) == 0 {
		return nil
	}
	runner := gitrun.CanonicalReadOnly(root)
	out, err := runner.Output(ctx, "ls-tree", "-r", "--name-only", "HEAD", "--", "brain/sources")
	if err != nil {
		return err
	}
	paths := map[string]bool{}
	for _, path := range strings.Split(string(out), "\n") {
		paths[path] = true
	}
	args := []string{"diff", "--quiet", "HEAD", "--"}
	for _, sha := range p.SourceIDs {
		base := "brain/sources/" + sha[:2] + "/" + sha + "/"
		for _, name := range []string{"bytes", "meta.yaml"} {
			path := base + name
			if !paths[path] {
				return store.ErrEpisodeUnpublished
			}
			args = append(args, path)
		}
	}
	if _, err = runner.Output(ctx, args...); err != nil {
		return store.ErrEpisodeUnpublished
	}
	return nil
}
