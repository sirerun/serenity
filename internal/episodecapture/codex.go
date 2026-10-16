package episodecapture

import (
	"io"

	"github.com/sirerun/serenity/internal/episodeclient"
	"github.com/sirerun/serenity/internal/episodehost"
)

type StagingResult struct {
	Cursor      episodehost.Cursor
	Gaps        []episodehost.Gap
	Admitted    int
	Complete    bool
	PartialTail bool
}

// StageCodex admits a synthetic enrolled snapshot into the protected queue.
// It never uploads. The returned cursor advances only if the entire page was
// safely admitted, so partial failures replay the same deterministic identities.
// Content-free gaps remain in the result and must be shown/persisted by the enrolling adapter.
// Timestamp attribution is the host log's recorded observation, not a claim
// that the original statement occurred exactly when the logger wrote it.
func StageCodex(q *episodeclient.Queue, r io.Reader, e episodehost.Enrollment, previous episodehost.Cursor, workspace string, synthetic bool) (StagingResult, error) {
	if q == nil || !synthetic {
		return StagingResult{}, ErrUnavailable
	}
	page, err := episodehost.ReadCodex(r, e, previous, episodehost.MaxPageEvents)
	if err != nil {
		return StagingResult{Cursor: previous}, err
	}
	out := StagingResult{Cursor: previous, Gaps: page.Gaps, Complete: page.Complete, PartialTail: page.PartialTail}
	for _, m := range page.Messages {
		_, err = q.Capture(episodeclient.Input{Workspace: workspace, Conversation: e.SessionID, Event: m.ID, Sequence: uint64(m.Sequence), Role: m.Role, Kind: "message", SourceTime: m.OccurredAt, ObservedTime: m.OccurredAt, HostMode: "normalized-visible-v1", Privacy: "public", Class: "visible", Text: m.Text})
		if err != nil {
			return out, err
		}
		out.Admitted++
	}
	out.Cursor = page.Cursor
	return out, nil
}
