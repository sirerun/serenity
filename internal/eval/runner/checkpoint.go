package runner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/sirerun/serenity/internal/eval"
)

// Checkpointing (T1.32) exists for one reason: a ModeLive run over a
// large held-out corpus (T1.32 expanded the ava corpus's held-out split
// from 52 to 312 spans) runs for a long time against a real endpoint, and
// a process-level kill mid-run -- observed twice in practice, both times
// the host's own OOM protection killing the eval-runner process, not an
// extraction error -- previously discarded every span already completed.
// A SIGKILL runs no deferred cleanup and no final report.json write, so
// the only way to preserve progress is to persist it incrementally, as
// each span finishes, to a file the next invocation can pick back up
// from.
//
// This is deliberately a resume mechanism for one eval-runner
// invocation over one corpus/config, not a general job-orchestration or
// distributed-checkpoint system: one JSONL file, one header line
// recording the (corpus, model version, held-out count) this checkpoint
// is valid for, then one line per held-out span appended as it is
// attempted. On a fully completed run the file is removed -- it is
// scratch state for surviving an interruption, never a canonical
// artifact, and leaving it in place would let a later, unrelated
// invocation silently "resume" from stale data.

// checkpointHeader is the first line of a checkpoint file, identifying
// the exact run it is valid for. Resuming against a checkpoint whose
// header doesn't match the current invocation's corpus, model version,
// or held-out span count is refused rather than silently mixing
// incompatible data (see loadCheckpoint).
type checkpointHeader struct {
	Header       bool   `json:"header"`
	Corpus       string `json:"corpus"`
	ModelVersion string `json:"model_version"`
	HeldOutCount int    `json:"held_out_count"`
}

// checkpointEntry is one held-out span's outcome, appended as soon as
// that span is attempted. Status is "scored", "skipped" (budget), or
// "errored" (extraction error) -- mirroring runLive's own three outcomes
// so a resumed run reconstructs the exact same counters an uninterrupted
// run would have produced.
type checkpointEntry struct {
	Span        string            `json:"span"`
	Status      string            `json:"status"`
	Predictions []eval.Prediction `json:"predictions,omitempty"`
	Error       string            `json:"error,omitempty"`
}

// resumedCheckpoint is what a prior, interrupted invocation had already
// accumulated before it was checkpointed.
type resumedCheckpoint struct {
	done         map[string]bool
	predictions  []eval.Prediction
	skipped      int
	errored      int
	errorSamples []string
}

// loadCheckpoint reads path if it exists and returns the state to resume
// from. A missing file is not an error -- every run's first attempt has
// no checkpoint yet, and that is the common case, not a failure.
func loadCheckpoint(path, corpus, modelVersion string, heldOutCount int) (resumedCheckpoint, bool, error) {
	result := resumedCheckpoint{done: map[string]bool{}}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return result, false, nil
		}
		return resumedCheckpoint{}, false, fmt.Errorf("open: %w", err)
	}
	defer func() { _ = f.Close() }() // read-only handle; a close error here has nothing left to affect

	scanner := bufio.NewScanner(f)
	// A checkpoint line embeds every prediction for its span; long
	// spans with several observations can exceed bufio.Scanner's 64KiB
	// default token size, so grow the buffer generously.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var sawHeader bool
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if !sawHeader {
			var hdr checkpointHeader
			if err := json.Unmarshal(line, &hdr); err != nil {
				return resumedCheckpoint{}, false, fmt.Errorf("parse header: %w", err)
			}
			if hdr.Corpus != corpus || hdr.ModelVersion != modelVersion || hdr.HeldOutCount != heldOutCount {
				return resumedCheckpoint{}, false, fmt.Errorf(
					"checkpoint %s was written for corpus=%q model_version=%q held_out_count=%d, this run is corpus=%q model_version=%q held_out_count=%d -- refusing to resume from a mismatched checkpoint; remove the file first if this is deliberately a fresh run",
					path, hdr.Corpus, hdr.ModelVersion, hdr.HeldOutCount, corpus, modelVersion, heldOutCount)
			}
			sawHeader = true
			continue
		}

		var entry checkpointEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return resumedCheckpoint{}, false, fmt.Errorf("parse entry: %w", err)
		}
		result.done[entry.Span] = true
		switch entry.Status {
		case "scored":
			result.predictions = append(result.predictions, entry.Predictions...)
		case "skipped":
			result.skipped++
		case "errored":
			result.errored++
			if len(result.errorSamples) < maxErrorSamples {
				result.errorSamples = append(result.errorSamples, fmt.Sprintf("%q: %s", entry.Span, entry.Error))
			}
		default:
			return resumedCheckpoint{}, false, fmt.Errorf("checkpoint %s: entry for span %q has unknown status %q", path, entry.Span, entry.Status)
		}
	}
	if err := scanner.Err(); err != nil {
		return resumedCheckpoint{}, false, fmt.Errorf("read: %w", err)
	}
	return result, true, nil
}

// checkpointWriter appends one line per held-out span to an open file
// handle, using plain unbuffered os.File.Write calls (no bufio.Writer)
// so every entry reaches the kernel's page cache before the call
// returns -- durable against this process being killed, which is
// exactly the failure this exists to survive. It does not need to
// survive a full power loss/host crash (that would need fsync per
// write); a killed process is enough to design for here.
type checkpointWriter struct {
	f *os.File
}

// openCheckpointWriter opens path for appending, writing a fresh header
// line first if the file is new (existed is what loadCheckpoint
// returned: whether path already held a valid, matching checkpoint to
// resume from). Resuming a run appends to the same header; a fresh run
// starts a new file with its own header.
func openCheckpointWriter(path string, existed bool, corpus, modelVersion string, heldOutCount int) (*checkpointWriter, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	w := &checkpointWriter{f: f}
	if !existed {
		hdr := checkpointHeader{Header: true, Corpus: corpus, ModelVersion: modelVersion, HeldOutCount: heldOutCount}
		if err := w.writeLine(hdr); err != nil {
			_ = f.Close() // best-effort; the write error above is what's reported
			return nil, fmt.Errorf("write header: %w", err)
		}
	}
	return w, nil
}

func (w *checkpointWriter) writeLine(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.f.Write(b)
	return err
}

func (w *checkpointWriter) writeScored(span string, predictions []eval.Prediction) error {
	return w.writeLine(checkpointEntry{Span: span, Status: "scored", Predictions: predictions})
}

func (w *checkpointWriter) writeSkipped(span string) error {
	return w.writeLine(checkpointEntry{Span: span, Status: "skipped"})
}

func (w *checkpointWriter) writeErrored(span string, err error) error {
	return w.writeLine(checkpointEntry{Span: span, Status: "errored", Error: err.Error()})
}

func (w *checkpointWriter) Close() error {
	return w.f.Close()
}
