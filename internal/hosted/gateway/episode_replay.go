package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

var ErrEpisodeReplayPending = errors.New("hosted: episode exclusion requires journal reconciliation")

type episodeReplayIntent struct {
	entry     contracts.DeletionEntry
	confirmed bool
	published bool
}

func episodeDeletionIdentity(brain string, r store.EpisodeRecord) contracts.EpisodeDeletionIdentity {
	return contracts.EpisodeDeletionIdentity{BrainHash: contracts.EpisodeDeletionBrainHash(brain), ConversationHash: r.ConversationKey, CommandHash: r.CommandKey, RequestDigest: r.RequestDigest}
}

func (g *Gateway) setEpisodeIntent(brain, key string, state episodeReplayIntent) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.episodeReplay == nil {
		g.episodeReplay = make(map[string]map[string]episodeReplayIntent)
	}
	if g.episodeReplay[brain] == nil {
		g.episodeReplay[brain] = make(map[string]episodeReplayIntent)
	}
	g.episodeReplay[brain][key] = state
}

// appendEpisodeIntent runs inside the serialized writer while its caller holds
// maintenance, account and runtime authority. Failed attempts retain their exact
// timestamp/identity for retry. No unsealed tail read is used to clear failures.
func (g *Gateway) appendEpisodeIntent(ctx context.Context, brain string, r *store.EpisodeRecord) error {
	id := episodeDeletionIdentity(brain, *r)
	e := contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectEpisodeConversation, SubjectID: id.String(), Outcome: contracts.DeletionIntentRequested, RecordedAt: r.RecordedAt}
	g.mu.Lock()
	old, exists := g.episodeReplay[brain][r.CommandKey]
	g.mu.Unlock()
	if exists {
		if old.entry.SubjectID != e.SubjectID {
			return store.ErrEpisodeConflict
		}
		e.RecordedAt = old.entry.RecordedAt
		r.RecordedAt = e.RecordedAt
		if old.confirmed {
			return nil
		}
	}
	g.setEpisodeIntent(brain, r.CommandKey, episodeReplayIntent{entry: e})
	if err := g.requireDeletionJournal(); err != nil {
		return err
	}
	appended, err := g.Journal.AppendDeletion(ctx, e)
	if err != nil {
		return err
	}
	if err = validateEpisodeJournalEntry(appended); err != nil {
		return err
	}
	if appended.SubjectType != e.SubjectType || appended.SubjectID != e.SubjectID || appended.Outcome != e.Outcome || !appended.RecordedAt.Equal(e.RecordedAt) {
		return ErrEpisodeReplayPending
	}
	g.setEpisodeIntent(brain, r.CommandKey, episodeReplayIntent{entry: appended, confirmed: true})
	return ctx.Err()
}

func validateEpisodeJournalEntry(e contracts.DeletionEntry) error {
	if err := e.Validate(); err != nil {
		return err
	}
	b, err := hex.DecodeString(e.Watermark.EntryHash)
	if err != nil || len(b) != 32 || hex.EncodeToString(b) != e.Watermark.EntryHash || e.Watermark.Generation < 1 || e.Watermark.SequenceID < 1 || e.RecordedAt.IsZero() {
		return contracts.ErrDeletionJournalIncomplete
	}
	return nil
}

// checkEpisodeReplay requires both directions: every local fence has journal
// evidence and every known intent has a canonical fence. An identical exclusion
// may repair only its own pending boundary. Unrelated reads/writes stay closed.
func (g *Gateway) checkEpisodeReplay(brain string, p *store.EpisodeProjection, retry *store.EpisodeInput) error {
	if err := g.requireDeletionJournal(); err != nil {
		return err
	}
	retryMatches := func(key, conversation, digest string) bool {
		return retry != nil && retry.Operation == "exclude" && store.EpisodeCommandKey(retry.ProducerID, retry.CommandID) == key && store.EpisodeConversationKey(retry.ProducerID, retry.ConversationID) == conversation && store.EpisodeHash(*retry) == digest
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	intents := g.episodeReplay[brain]
	for key, r := range p.Records {
		if r.Kind != "exclude" {
			continue
		}
		state, ok := intents[key]
		if !ok || !state.confirmed || state.entry.SubjectID != episodeDeletionIdentity(brain, r).String() || !state.entry.RecordedAt.Equal(r.RecordedAt) {
			if !retryMatches(key, r.ConversationKey, r.RequestDigest) {
				return ErrEpisodeReplayPending
			}
		}
	}
	for key, state := range intents {
		id, err := contracts.ParseEpisodeDeletionIdentity(state.entry.SubjectID)
		if err != nil {
			return err
		}
		r, ok := p.Records[key]
		if !state.confirmed || !ok || r.Kind != "exclude" || episodeDeletionIdentity(brain, r) != id || !r.RecordedAt.Equal(state.entry.RecordedAt) {
			if !retryMatches(key, id.ConversationHash, id.RequestDigest) {
				return ErrEpisodeReplayPending
			}
		}
	}
	return nil
}

// episodeRetryGeneration lets a failed exclusion retry repair its own canonical
// quarantine without needing a fences read (reads are closed while pending).
// Successful retries retain the existing protocol's current-generation check.
func (g *Gateway) episodeRetryGeneration(brain string, p *store.EpisodeProjection, in store.EpisodeInput, requested uint64) uint64 {
	key := store.EpisodeCommandKey(in.ProducerID, in.CommandID)
	r, exists := p.Records[key]
	if !exists || r.Kind != "exclude" || r.RequestDigest != store.EpisodeHash(in) {
		return requested
	}
	g.mu.Lock()
	state, known := g.episodeReplay[brain][key]
	g.mu.Unlock()
	if !known || !state.published {
		return p.Generation
	}
	return requested
}

// ReplayEpisodeDeletions must be called only with the complete history already
// admitted by the service's ordinary startup authority or sealed recovery path,
// before installing handlers. It returns legacy entries for ReplayDeletions.
// A local projection or highest visible unsealed tail is not such admission.
func (g *Gateway) ReplayEpisodeDeletions(ctx context.Context, entries []contracts.DeletionEntry) ([]contracts.DeletionEntry, error) {
	if err := g.requireDeletionJournal(); err != nil {
		return nil, err
	}
	var legacy, episodes []contracts.DeletionEntry
	deletedBrains := map[string]bool{}
	deletedAccounts := map[string]bool{}
	var previous contracts.DeletionWatermark
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := validateEpisodeJournalEntry(e); err != nil {
			return nil, err
		}
		wm := e.Watermark
		if !previous.IsZero() && (wm.Generation < previous.Generation || wm.Generation == previous.Generation && wm.SequenceID <= previous.SequenceID) {
			return nil, contracts.ErrDeletionJournalIncomplete
		}
		previous = wm
		if e.SubjectType == contracts.DeletionSubjectEpisodeConversation {
			episodes = append(episodes, e)
		} else {
			legacy = append(legacy, e)
			if e.SubjectType == contracts.DeletionSubjectBrain {
				deletedBrains[contracts.EpisodeDeletionBrainHash(e.SubjectID)] = true
			}
			if e.SubjectType == contracts.DeletionSubjectAccount {
				deletedAccounts[e.SubjectID] = true
			}
		}
	}
	type brainRow struct{ id, account, state string }
	rows, err := g.Issuer.Store.DB().QueryContext(ctx, `SELECT id,account_id,state FROM brains ORDER BY id`)
	if err != nil {
		return nil, err
	}
	brains := map[string]brainRow{}
	var ordered []brainRow
	for rows.Next() {
		var b brainRow
		if err = rows.Scan(&b.id, &b.account, &b.state); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if !store.ValidEpisodeID(b.id) {
			_ = rows.Close()
			return nil, store.ErrEpisodeInvalid
		}
		hash := contracts.EpisodeDeletionBrainHash(b.id)
		if _, exists := brains[hash]; exists {
			_ = rows.Close()
			return nil, store.ErrEpisodeConflict
		}
		brains[hash] = b
		ordered = append(ordered, b)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	byBrain := map[string][]contracts.DeletionEntry{}
	seen := map[string]contracts.DeletionEntry{}
	for _, e := range episodes {
		id, _ := contracts.ParseEpisodeDeletionIdentity(e.SubjectID) // validated above
		b, exists := brains[id.BrainHash]
		if deletedBrains[id.BrainHash] || exists && deletedAccounts[b.account] {
			continue
		} // legacy replay owns the stronger whole-brain exclusion
		if !exists || b.state == "deleted" {
			return nil, fmt.Errorf("%w: episode brain is not resolved by admitted history", ErrDeletionSubjectStateUnknown)
		}
		key := id.BrainHash + ":" + id.CommandHash
		if old, ok := seen[key]; ok {
			if old.SubjectID != e.SubjectID || !old.RecordedAt.Equal(e.RecordedAt) {
				return nil, store.ErrEpisodeConflict
			}
			continue
		}
		seen[key] = e
		byBrain[b.id] = append(byBrain[b.id], e)
	}
	g.Maintenance.RLock()
	defer g.Maintenance.RUnlock()
	for _, b := range ordered {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if deletedBrains[contracts.EpisodeDeletionBrainHash(b.id)] || deletedAccounts[b.account] {
			continue
		}
		// Inspect the source namespace without initializing a runtime for unrelated
		// empty/provisioning brains. This also detects foundation-only local fences.
		p, err := store.LoadEpisodes(ctx, store.NewSourceStore(filepath.Join(g.Pool.BrainsRoot(), b.id)))
		if err != nil {
			return nil, err
		}
		if p.BrainID != "" && p.BrainID != b.id {
			return nil, store.ErrEpisodeConflict
		}
		if len(byBrain[b.id]) == 0 && len(p.Fences) == 0 {
			continue
		}
		if b.state == "deleted" {
			return nil, ErrEpisodeReplayPending
		}
		for _, e := range byBrain[b.id] {
			id, _ := contracts.ParseEpisodeDeletionIdentity(e.SubjectID)
			g.setEpisodeIntent(b.id, id.CommandHash, episodeReplayIntent{entry: e, confirmed: true, published: true})
		}
		// Refuse unjournaled local fences BEFORE writing more canonical state.
		for _, r := range p.Records {
			if r.Kind != "exclude" {
				continue
			}
			e, ok := seen[contracts.EpisodeDeletionBrainHash(b.id)+":"+r.CommandKey]
			if !ok || e.SubjectID != episodeDeletionIdentity(b.id, r).String() || !e.RecordedAt.Equal(r.RecordedAt) {
				return nil, ErrEpisodeReplayPending
			}
		}
		if err = g.replayEpisodeBrain(ctx, b.account, b.id, byBrain[b.id]); err != nil {
			return nil, err
		}
	}
	return legacy, ctx.Err()
}

func (g *Gateway) replayEpisodeBrain(ctx context.Context, account, brain string, entries []contracts.DeletionEntry) error {
	hash := sha256.Sum256([]byte(account))
	lock := &g.accountLocks[int(hash[0])%len(g.accountLocks)]
	lock.Lock()
	defer lock.Unlock()
	runtime, release, err := g.Pool.AcquireCanonical(ctx, brain)
	if err != nil {
		return err
	}
	defer release()
	runtime.Mutations.Lock()
	defer runtime.Mutations.Unlock()
	for _, e := range entries {
		id, _ := contracts.ParseEpisodeDeletionIdentity(e.SubjectID)
		fence := store.EpisodeRecord{Schema: store.EpisodeSchema, BrainID: brain, Kind: "exclude", CommandKey: id.CommandHash, ConversationKey: id.ConversationHash, RequestDigest: id.RequestDigest, RecordedAt: e.RecordedAt}
		if _, err = writer.EpisodeReplayFence(ctx, runtime.Queue, runtime.Sources, runtime.Root, fence); err != nil {
			return err
		}
	}
	p, err := store.LoadEpisodes(ctx, runtime.Sources)
	if err != nil {
		return err
	}
	return g.checkEpisodeReplay(brain, p, nil)
}

func (g *Gateway) episodeExclusionPublished(brain, key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	state := g.episodeReplay[brain][key]
	state.published = true
	g.episodeReplay[brain][key] = state
}
