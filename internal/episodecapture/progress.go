//go:build linux || darwin

package episodecapture

import (
	"io"

	"github.com/sirerun/serenity/internal/episodeclient"
	"github.com/sirerun/serenity/internal/episodehost"
)

// StageCodexDurable is synthetic-only. It atomically persists a page's redacted
// events, verified-prefix cursor and cumulative content-free gaps in the same
// encrypted queue transaction. No original log path or host discovery is used.
// Capacity/policy/concurrency errors commit neither records nor progress.
func StageCodexDurable(q *episodeclient.Queue, r io.Reader, e episodehost.Enrollment, workspace string, synthetic bool) (StagingResult, error) {
	var out StagingResult
	if q == nil || !synthetic {
		return out, ErrUnavailable
	}
	key := episodeclient.ImportKey{Format: episodeclient.ProgressFormat("codex", episodehost.CodexVersion), Source: e.SessionID, Workspace: workspace, Conversation: e.SessionID}
	old, err := q.ImportProgress(key)
	if err != nil {
		return out, err
	}
	previous := episodehost.Cursor{}
	if old.Cursor.Records > 0 {
		previous = episodehost.Cursor{SessionID: e.SessionID, Version: episodehost.CodexVersion, Lines: int(old.Cursor.Records), PrefixSHA256: old.Cursor.PrefixSHA256}
	}
	out.Cursor = previous
	page, err := episodehost.ReadCodex(r, e, previous, episodehost.MaxPageEvents)
	if err != nil {
		return out, err
	}
	next := episodeclient.ImportProgress{Key: key, Cursor: episodeclient.ImportCursor{Records: uint64(page.Cursor.Lines), PrefixSHA256: page.Cursor.PrefixSHA256}, Complete: page.Complete, PartialTail: page.PartialTail, Gaps: map[string]uint64{}}
	for code, n := range old.Gaps {
		next.Gaps[code] = n
	}
	for _, gap := range page.Gaps {
		// An incomplete final line was not consumed. Persist its pending state,
		// but never count it repeatedly as a consumed omission.
		if gap.Reason != "partial_tail" {
			next.Gaps[gap.Reason]++
		}
	}
	inputs := make([]episodeclient.Input, 0, len(page.Messages))
	for _, m := range page.Messages {
		inputs = append(inputs, episodeclient.Input{Workspace: workspace, Conversation: e.SessionID, Event: m.ID, Sequence: uint64(m.Sequence), Role: m.Role, Kind: "message", SourceTime: m.OccurredAt, ObservedTime: m.OccurredAt, HostMode: "normalized-visible-v1", Privacy: "public", Class: "visible", Text: m.Text})
	}
	if err = q.CapturePage(old, next, inputs); err != nil {
		return out, err
	}
	return StagingResult{Cursor: page.Cursor, Gaps: page.Gaps, Admitted: len(inputs), Complete: page.Complete, PartialTail: page.PartialTail}, nil
}
