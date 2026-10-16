package gateway

import (
	"errors"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

var ErrEpisodeQualification = errors.New("episode qualification unavailable")

const EpisodeQualificationDuration = 15 * time.Minute

type episodeQualification struct {
	brains []string
	until  time.Time
}

// SetEpisodeQualification is for the existing private operator transport only.
// Its caller verifies dedicated synthetic account ownership. It creates no
// credentials and cannot override the compiled safety gate or permanent config.
// An empty list with zero expiration closes the temporary gate immediately.
func (g *Gateway) SetEpisodeQualification(brains []string, until time.Time) error {
	if len(brains) == 0 {
		if !until.IsZero() {
			return ErrEpisodeQualification
		}
		g.mu.Lock()
		g.episodeQualification = nil
		g.mu.Unlock()
		return nil
	}
	now := time.Now()
	if !episodeArchiveBuildEnabled || len(brains) > 2 || !until.After(now) || until.After(now.Add(EpisodeQualificationDuration)) {
		return ErrEpisodeQualification
	}
	seen := map[string]bool{}
	for _, brain := range brains {
		if !store.ValidEpisodeID(brain) || seen[brain] {
			return ErrEpisodeQualification
		}
		seen[brain] = true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || g.EpisodeArchiveEnabled || len(g.EpisodeSyntheticBrains) != 0 {
		return ErrEpisodeQualification
	}
	g.episodeQualification = &episodeQualification{brains: append([]string(nil), brains...), until: until}
	return nil
}
func (g *Gateway) episodeEnrollment() (bool, []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !episodeArchiveBuildEnabled {
		return false, nil
	}
	if g.EpisodeArchiveEnabled {
		return true, append([]string(nil), g.EpisodeSyntheticBrains...)
	}
	q := g.episodeQualification
	if q == nil || !time.Now().Before(q.until) {
		return false, nil
	}
	return true, append([]string(nil), q.brains...)
}
func (g *Gateway) episodeBrainAllowed(brain string) bool {
	enabled, brains := g.episodeEnrollment()
	if !enabled {
		return false
	}
	for _, b := range brains {
		if b == brain {
			return true
		}
	}
	return false
}
