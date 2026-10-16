package episodeclient

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
)

const (
	MaxImportBytes   = 8 << 20
	MaxImportRecords = 4096
	MaxLineBytes     = 768 << 10
)

// LogSource is trusted enrollment metadata. ID identifies a logical log, never
// its path. Generation identifies one incarnation. A changed incarnation needs
// explicit handling; Import never silently resets a cursor or invents a fork.
type LogSource struct{ ID, Generation string }

// Cursor is content-free progress, returned after each safely consumed record.
// Persist it in protected adapter state. Losing it causes SAME-identity replay,
// not new identities. PrefixSHA256 detects changed/reordered/truncated prefixes.
type Cursor struct {
	Source       LogSource
	Records      int
	PrefixSHA256 string
}
type GapCount struct {
	Gap   Gap
	Count int
}
type ImportResult struct {
	Cursor             Cursor
	Admitted, Excluded int
	Gaps               []GapCount
}

// Import accepts only normalized-visible-v1 JSONL. It reads at most 8 MiB+1,
// checks the old prefix before invoking emit, rejects unknown fields, and never
// opens paths mentioned in input. Complete lines before a partial final line
// can be consumed; ErrPartial makes that coverage boundary explicit.
// emit should be Queue.Capture. A non-exclusion error leaves the failing record
// unconsumed. The caller must not advance progress beyond the returned cursor.
func Import(r io.Reader, source LogSource, previous Cursor, emit func(Input) (Decision, error)) (ImportResult, error) {
	out := ImportResult{Cursor: previous}
	if r == nil || emit == nil || !validID(source.ID) || !validID(source.Generation) {
		return out, ErrInvalid
	}
	if previous.Records < 0 || previous.Records > MaxImportRecords {
		return out, ErrInvalid
	}
	if previous.Source != (LogSource{}) && previous.Source != source {
		return out, ErrRotation
	}
	if previous.Source == (LogSource{}) && (previous.Records != 0 || previous.PrefixSHA256 != "") {
		return out, ErrInvalid
	}
	b, e := io.ReadAll(io.LimitReader(r, MaxImportBytes+1))
	if e != nil {
		return out, ErrInvalid
	}
	if len(b) > MaxImportBytes {
		return out, ErrCapacity
	}
	end := bytes.LastIndexByte(b, '\n') + 1
	complete := b[:end]
	partial := end < len(b)
	if bytes.Count(complete, []byte{'\n'}) > MaxImportRecords {
		return out, ErrCapacity
	}
	lines := bytes.SplitAfter(complete, []byte{'\n'})
	lines = lines[:len(lines)-1]
	if len(lines) > MaxImportRecords {
		return out, ErrCapacity
	}
	if len(lines) < previous.Records {
		return out, ErrRotation
	}
	offset := 0
	for _, line := range lines[:previous.Records] {
		offset += len(line)
	}
	if previous.Source != (LogSource{}) && previous.PrefixSHA256 != digest(complete[:offset]) {
		return out, ErrRotation
	}
	out.Cursor = Cursor{source, previous.Records, digest(complete[:offset])}
	prefix := sha256.New()
	prefix.Write(complete[:offset])
	for i := previous.Records; i < len(lines); i++ {
		line := lines[i]
		if !utf8.Valid(line) {
			return out, ErrUnsupported
		}
		if len(line) > MaxLineBytes {
			return out, ErrCapacity
		}
		if !strictJSON(line) {
			return out, ErrUnsupported
		}
		var in Input
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		if e = dec.Decode(&in); e != nil {
			return out, ErrUnsupported
		}
		var extra any
		if dec.Decode(&extra) != io.EOF {
			return out, ErrUnsupported
		}
		// IDs without a host identity use source incarnation + physical line number,
		// never content-only deduplication. Identical lines remain distinct events.
		if in.Event == "" {
			in.Event = DeterministicEventID(source, uint64(i+1))
		}
		if in.Sequence == 0 {
			in.Sequence = uint64(i + 1)
		}
		d, e := emit(in)
		if e != nil && e != ErrExcluded && e != ErrUnsupported {
			return out, e
		}
		if e != nil {
			out.Excluded++
		} else if d.Admitted {
			out.Admitted++
		} else {
			return out, ErrInvalid
		}
		// Aggregate counts by fixed reason, not per-record identifiers/content. Bound
		// this diagnostics list independently from the input size.
		for _, gap := range d.Gaps {
			found := false
			for j, old := range out.Gaps {
				if old.Gap == gap {
					out.Gaps[j].Count++
					found = true
					break
				}
			}
			if !found && len(out.Gaps) < 128 {
				out.Gaps = append(out.Gaps, GapCount{gap, 1})
			}
		}
		offset += len(line)
		prefix.Write(line)
		out.Cursor = Cursor{source, i + 1, hex.EncodeToString(prefix.Sum(nil))}
	}
	if partial {
		return out, ErrPartial
	}
	return out, nil
}

// DeterministicEventID is meaningful only with an enrolled logical source and
// incarnation. Live adapters must reuse this same mapping when observing logs.
func DeterministicEventID(source LogSource, line uint64) string {
	if !validID(source.ID) || !validID(source.Generation) || line == 0 {
		return ""
	}
	return "log-" + digest([]byte(fmt.Sprintf("episodeclient.log.v1\x00%s\x00%s\x00%d", source.ID, source.Generation, line)))
}
