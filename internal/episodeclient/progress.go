//go:build linux || darwin

package episodeclient

import (
	"encoding/json"
	"reflect"
	"strings"
)

const MaxImportSources = 128
const MaxPageInputs = 64
const MaxProgressRecords = 1 << 24

// ImportKey is trusted enrollment identity, never a host path. The queue's
// authenticated encryption additionally binds every source to its brain/policy.
type ImportKey struct {
	Format       string `json:"format"`
	Source       string `json:"source"`
	Workspace    string `json:"workspace"`
	Conversation string `json:"conversation"`
}
type ImportCursor struct {
	Records      uint64 `json:"records"`
	PrefixSHA256 string `json:"prefix_sha256"`
}

// ImportProgress contains no original transcript bytes. Gaps are fixed adapter
// codes and cumulative counts, not prose. It is encrypted with the queued data.
type ImportProgress struct {
	Key         ImportKey         `json:"key"`
	Cursor      ImportCursor      `json:"cursor"`
	Gaps        map[string]uint64 `json:"gaps,omitempty"`
	Complete    bool              `json:"complete"`
	PartialTail bool              `json:"partial_tail"`
}

func (k ImportKey) valid() bool {
	return validID(k.Format) && validID(k.Source) && validID(k.Workspace) && validID(k.Conversation)
}
func (k ImportKey) hash() string { b, _ := json.Marshal(k); return digest(b) }
func (p ImportProgress) valid() bool {
	if !p.Key.valid() || p.Cursor.Records > MaxProgressRecords || len(p.Gaps) > 32 || p.Complete && p.PartialTail {
		return false
	}
	if p.Cursor.Records == 0 {
		if p.Cursor.PrefixSHA256 != "" {
			return false
		}
	} else if !validFenceKey(p.Cursor.PrefixSHA256) {
		return false
	}
	for code, n := range p.Gaps {
		if len(code) == 0 || len(code) > 64 || n == 0 || n > MaxProgressRecords {
			return false
		}
		for _, c := range code {
			if (c < 'a' || c > 'z') && c != '_' {
				return false
			}
		}
	}
	return true
}
func validateImports(s queueState) error {
	if len(s.Imports) > MaxImportSources || s.Version == 1 && len(s.Imports) > 0 {
		return ErrStorage
	}
	for key, p := range s.Imports {
		if !p.valid() || key != p.Key.hash() {
			return ErrStorage
		}
	}
	return nil
}
func currentProgress(s queueState, k ImportKey) ImportProgress {
	if p, ok := s.Imports[k.hash()]; ok {
		return p
	}
	return ImportProgress{Key: k}
}

// ImportProgress reads detached encrypted progress. It never discovers history.
func (q *Queue) ImportProgress(k ImportKey) (ImportProgress, error) {
	var out ImportProgress
	if !k.valid() {
		return out, ErrInvalid
	}
	err := q.withLock(func() error {
		s, e := q.read()
		if e != nil {
			return e
		}
		out = currentProgress(s, k)
		return nil
	})
	return out, err
}

// CapturePage atomically commits all redacted admissions and their cursor.
// A failed admission leaves both unchanged. expected is a compare-and-swap
// guard across processes; concurrent or stale importers cannot advance it.
// Publication may drain between reading progress and this call, which is safe:
// event identities remain stable and server publication deduplicates retries.
func (q *Queue) CapturePage(expected, next ImportProgress, inputs []Input) error {
	if !expected.valid() || !next.valid() || expected.Key != next.Key || len(inputs) > MaxPageInputs || next.Cursor.Records < expected.Cursor.Records {
		return ErrInvalid
	}
	if next.Cursor.Records == expected.Cursor.Records && next.Cursor.PrefixSHA256 != expected.Cursor.PrefixSHA256 {
		return ErrRotation
	}
	if next.Cursor.Records == expected.Cursor.Records && len(inputs) > 0 {
		return ErrInvalid
	}
	for code, n := range expected.Gaps {
		if next.Gaps[code] < n {
			return ErrInvalid
		}
	}
	return q.withLock(func() error {
		s, e := q.read()
		if e != nil {
			return e
		}
		if !reflect.DeepEqual(currentProgress(s, expected.Key), expected) {
			return ErrConflict
		}
		// A zero-message page must still respect current enrollment, pause and fence.
		if err := q.policy.resolve(next.Key.Workspace); err != nil {
			return err
		}
		sample := Record{Binding: q.policy.Binding, Workspace: next.Key.Workspace, Conversation: next.Key.Conversation}
		if paused(s, sample) {
			return ErrPaused
		}
		if s.Fenced[ConversationFenceKey(q.policy.Binding.Producer, next.Key.Conversation)] {
			return ErrBlocked
		}
		for _, in := range inputs {
			if in.Workspace != next.Key.Workspace || in.Conversation != next.Key.Conversation {
				return ErrInvalid
			}
			if _, e = q.captureInto(&s, in); e != nil {
				return e
			}
		}
		if s.Imports == nil {
			s.Imports = map[string]ImportProgress{}
		}
		if _, ok := s.Imports[next.Key.hash()]; !ok && len(s.Imports) >= MaxImportSources {
			return ErrCapacity
		}
		// Normalize nil/empty gap maps so persisted round trips retain CAS equality.
		if len(next.Gaps) == 0 {
			next.Gaps = nil
		}
		s.Imports[next.Key.hash()] = next
		return q.write(s)
	})
}

// ProgressFormat is a stable adapter/version identity without paths.
func ProgressFormat(host, version string) string { return strings.Join([]string{host, version}, ".") }
