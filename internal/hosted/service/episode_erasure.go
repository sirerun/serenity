package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/episodeerase"
	"github.com/sirerun/serenity/internal/gitrun"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

var ErrEpisodePreparationUnavailable = errors.New("episode preparation unavailable or unreconciled")

// EpisodeErasureRequest is an internal trusted-maintenance request, never an
// HTTP or model operation. Pins assert independently qualified retention.
// No caller-supplied fences, destination or authority callback is accepted.
type EpisodeErasureRequest struct {
	AccountID          string
	BrainID            string
	OperationID        string
	ExpectedSourceHead string
	IndependentPaths   []episodeerase.PathPin
	IndependentSources []episodeerase.SourcePin
}

// EpisodeErasurePreparation is inactive source evidence only. On a post-build
// error ResidualInactive identifies the owned copy requiring reconciliation;
// it must never be treated as prepared success, activation or physical purge.
// ResidualInactive=false means a qualified prepared copy, which STILL remains a
// controlled sensitive copy; it never means that a copy was removed or purged.
type EpisodeErasurePreparation struct {
	OperationID      string
	CutID            string
	HistorySHA256    string
	Candidate        episodeerase.Result
	ResidualInactive bool
}

// PrepareEpisodeErasure owns one maintenance interval and stable writer lock.
// It refuses unreconciled history rather than replaying or modifying deletion.
func (s *Service) PrepareEpisodeErasure(ctx context.Context, req EpisodeErasureRequest) (out EpisodeErasurePreparation, err error) {
	if isNilDependency(ctx) || s == nil || s.Gateway == nil || s.Pool == nil || s.Store == nil || isNilDependency(s.episodeMaintenance) || !store.ValidEpisodeID(req.AccountID) || !store.ValidEpisodeID(req.BrainID) || !store.ValidSourceSHA(req.OperationID) || !sourceHeadValid(req.ExpectedSourceHead) {
		return out, ErrEpisodePreparationUnavailable
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	wait := time.NewTicker(10 * time.Millisecond)
	for !s.Gateway.Maintenance.TryLock() {
		select {
		case <-ctx.Done():
			wait.Stop()
			return out, ctx.Err()
		case <-wait.C:
		}
	}
	wait.Stop()
	defer s.Gateway.Maintenance.Unlock()
	if err = s.checkAuthority(ctx); err != nil {
		return out, err
	}
	root, err := s.episodePreparationBrain(ctx, req)
	if err != nil {
		return out, err
	}
	if err = s.Pool.FlushAll(); err != nil {
		return out, err
	}
	if err = s.Pool.Drop(req.BrainID); err != nil {
		return out, err
	}
	owner, err := writer.AcquireBrain(root)
	if err != nil {
		return out, err
	}
	defer func() {
		closeErr := owner.Close()
		if closeErr != nil && out.Candidate.CandidateHead != "" {
			out.ResidualInactive = true
		}
		err = errors.Join(err, closeErr)
	}()
	checkHead := func() error {
		head, e := gitrun.CanonicalReadOnly(root).Output(ctx, "rev-parse", "--verify", "HEAD")
		if e != nil || strings.TrimSpace(string(head)) != req.ExpectedSourceHead {
			return ErrEpisodePreparationUnavailable
		}
		return nil
	}
	if err = checkHead(); err != nil {
		return out, err
	}
	snapshot, err := s.episodeMaintenance.Observe(ctx)
	if err != nil {
		return out, err
	}
	if !store.ValidSourceSHA(snapshot.CutID) || !store.ValidSourceSHA(snapshot.Observation.HistorySHA256) || snapshot.Recheck == nil {
		return out, ErrEpisodePreparationUnavailable
	}
	projection, err := store.LoadEpisodes(ctx, store.NewSourceStore(root))
	if err != nil {
		return out, err
	}
	fences, err := matchedEpisodeFences(req, projection, snapshot.Observation)
	if err != nil {
		return out, err
	}
	recheck := func() error {
		if e := s.checkAuthority(ctx); e != nil {
			return e
		}
		current, e := s.episodePreparationBrain(ctx, req)
		if e != nil || current != root {
			return ErrEpisodePreparationUnavailable
		}
		if e = checkHead(); e != nil {
			return e
		}
		return snapshot.Recheck(ctx)
	}
	if err = recheck(); err != nil {
		return out, err
	}
	parent, err := episodePreparationDirectory(s.cfg.DataDir)
	if err != nil {
		return out, err
	}
	destination := filepath.Join(parent, req.OperationID)
	if _, e := os.Lstat(destination); !errors.Is(e, os.ErrNotExist) {
		return out, ErrEpisodePreparationUnavailable
	}
	result, err := episodeerase.BuildHosted(ctx, episodeerase.HostedRequest{Request: episodeerase.Request{Source: root, Destination: destination, BrainID: req.BrainID, CurrentFences: fences}, IndependentPaths: req.IndependentPaths, IndependentSources: req.IndependentSources})
	if err != nil {
		if _, statErr := os.Lstat(destination); statErr == nil {
			out = EpisodeErasurePreparation{OperationID: req.OperationID, CutID: snapshot.CutID, HistorySHA256: snapshot.Observation.HistorySHA256, ResidualInactive: true}
		}
		return out, err
	}
	out = EpisodeErasurePreparation{OperationID: req.OperationID, CutID: snapshot.CutID, HistorySHA256: snapshot.Observation.HistorySHA256, Candidate: result, ResidualInactive: true}
	if err = recheck(); err != nil {
		return out, fmt.Errorf("inactive candidate retained for reconciliation: %w", err)
	}
	out.ResidualInactive = false
	return out, nil
}
func sourceHeadValid(head string) bool {
	if len(head) != 40 {
		return false
	}
	for _, c := range head {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
func (s *Service) episodePreparationBrain(ctx context.Context, req EpisodeErasureRequest) (string, error) {
	var state string
	if err := s.Store.DB().QueryRowContext(ctx, "SELECT status FROM accounts WHERE id=?", req.AccountID).Scan(&state); err != nil || state != "active" {
		return "", ErrEpisodePreparationUnavailable
	}
	brain, err := s.Store.BrainByID(ctx, req.AccountID, req.BrainID)
	if err != nil || brain.State != "ready" || brain.PathKey != brain.ID {
		return "", ErrEpisodePreparationUnavailable
	}
	root := filepath.Join(s.cfg.DataDir, "brains", brain.PathKey)
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || resolved != root {
		return "", ErrEpisodePreparationUnavailable
	}
	return root, nil
}
func episodePreparationDirectory(data string) (string, error) {
	absolute, err := filepath.Abs(data)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil || resolved != absolute {
		return "", ErrEpisodePreparationUnavailable
	}
	parent := filepath.Join(absolute, ".episode-erasure")
	if err = os.Mkdir(parent, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
		return "", ErrEpisodePreparationUnavailable
	}
	// Persist ownership directory before any inactive copy publication.
	d, err := os.Open(absolute)
	if err != nil {
		return "", err
	}
	err = errors.Join(d.Sync(), d.Close())
	if err != nil {
		return "", err
	}
	return parent, nil
}
func matchedEpisodeFences(req EpisodeErasureRequest, p *store.EpisodeProjection, o contracts.JournalObservation) ([]store.EpisodeRecord, error) {
	if p.BrainID != "" && p.BrainID != req.BrainID {
		return nil, ErrEpisodePreparationUnavailable
	}
	external := map[string]contracts.DeletionEntry{}
	last := contracts.DeletionWatermark{}
	for _, e := range o.Read.Entries {
		if e.Validate() != nil || e.RecordedAt.IsZero() || e.Watermark.Generation < 1 || e.Watermark.SequenceID < 1 || !store.ValidSourceSHA(e.Watermark.EntryHash) {
			return nil, ErrEpisodePreparationUnavailable
		}
		if e.Watermark.Generation < last.Generation || e.Watermark.Generation == last.Generation && e.Watermark.SequenceID <= last.SequenceID {
			return nil, ErrEpisodePreparationUnavailable
		}
		last = e.Watermark
		if e.SubjectType == contracts.DeletionSubjectAccount && e.SubjectID == req.AccountID || e.SubjectType == contracts.DeletionSubjectBrain && e.SubjectID == req.BrainID {
			return nil, ErrEpisodePreparationUnavailable
		}
		if e.SubjectType != contracts.DeletionSubjectEpisodeConversation {
			continue
		}
		id, err := contracts.ParseEpisodeDeletionIdentity(e.SubjectID)
		if err != nil {
			return nil, err
		}
		if id.BrainHash != contracts.EpisodeDeletionBrainHash(req.BrainID) {
			continue
		}
		if _, exists := external[id.CommandHash]; exists {
			return nil, ErrEpisodePreparationUnavailable
		}
		external[id.CommandHash] = e
	}
	var fences []store.EpisodeRecord
	for _, r := range p.Records {
		if r.Kind != "exclude" {
			continue
		}
		e, ok := external[r.CommandKey]
		id := contracts.EpisodeDeletionIdentity{BrainHash: contracts.EpisodeDeletionBrainHash(req.BrainID), ConversationHash: r.ConversationKey, CommandHash: r.CommandKey, RequestDigest: r.RequestDigest}
		if !ok || e.SubjectID != id.String() || !e.RecordedAt.Equal(r.RecordedAt) {
			return nil, ErrEpisodePreparationUnavailable
		}
		fences = append(fences, r)
		delete(external, r.CommandKey)
	}
	if len(external) != 0 || len(fences) == 0 || uint64(len(fences)) != p.Generation {
		return nil, ErrEpisodePreparationUnavailable
	}
	sort.Slice(fences, func(i, j int) bool { return fences[i].Generation < fences[j].Generation })
	for i, f := range fences {
		if f.Generation != uint64(i+1) {
			return nil, ErrEpisodePreparationUnavailable
		}
	}
	return fences, nil
}
