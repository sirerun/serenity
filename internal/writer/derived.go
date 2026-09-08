package writer

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirerun/serenity/internal/store"
)

// FenceDerived refreshes an existing page's summary, claims and claim-detail
// fences through the writer queue. The caller preserves canonical fence-tier
// claims in p; this function preserves all bytes outside the managed fences,
// including unknown frontmatter and human prose. Dirty edits still pause the
// write, exactly as Fence does. Missing or ambiguous fences are errors.
func FenceDerived(q *Queue, fw *store.FenceWriter, p *store.EntityPage) (path string, rendered []byte, err error) {
	path = fw.PathFor(p.Entity.Type, p.Entity.Slug)
	original, err := os.ReadFile(path)
	if err != nil {
		return path, nil, fmt.Errorf("derived fence: read page: %w", err)
	}
	fresh, err := fw.RenderEntity(p)
	if err != nil {
		return path, nil, err
	}
	machine, err := mergeDerivedFences(original, fresh)
	if err != nil {
		return path, nil, err
	}
	// Avoid registering a no-op as a touched path: Flush must not commit a
	// human edit merely because it already contains the desired summary.
	if bytes.Equal(original, machine) {
		return path, original, nil
	}
	rendered, err = guard(q, fw.Root, path, p.Entity.Slug, machine, func() ([]byte, error) {
		current, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("derived fence: reread page: %w", err)
		}
		if !bytes.Equal(current, original) {
			return nil, fmt.Errorf("derived fence: page changed while queued: %w", ErrDirtyTree)
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		f, err := os.CreateTemp(filepath.Dir(path), ".serenity-derived-*")
		if err != nil {
			return nil, fmt.Errorf("derived fence: create replacement: %w", err)
		}
		name := f.Name()
		defer func() { _ = os.Remove(name) }()
		if err := f.Chmod(info.Mode().Perm()); err != nil {
			_ = f.Close()
			return nil, err
		}
		if _, err := f.Write(machine); err != nil {
			_ = f.Close()
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
		if err := os.Rename(name, path); err != nil {
			return nil, fmt.Errorf("derived fence: replace page: %w", err)
		}
		return machine, nil
	})
	return path, rendered, err
}

// fenceSpan recognizes a complete, unique managed block, including its markers.
// An optional block may be wholly absent; half a block is always malformed.
func fenceSpan(data []byte, name string, optional bool) (int, int, error) {
	begin := []byte("<!-- serenity:" + name + ":begin")
	end := []byte("<!-- serenity:" + name + ":end -->")
	nb, ne := bytes.Count(data, begin), bytes.Count(data, end)
	if optional && nb == 0 && ne == 0 {
		return -1, -1, nil
	}
	if nb != 1 || ne != 1 {
		return 0, 0, fmt.Errorf("derived fence: %s requires one complete block", name)
	}
	start, finish := bytes.Index(data, begin), bytes.Index(data, end)
	close := bytes.Index(data[start:], []byte("-->"))
	if close < 0 || start+close+3 > finish {
		return 0, 0, fmt.Errorf("derived fence: malformed %s block", name)
	}
	return start, finish + len(end), nil
}

func mergeDerivedFences(original, fresh []byte) ([]byte, error) {
	// Reject overlapping sections rather than replacing text ambiguously.
	last := -1
	for _, name := range []string{"summary", "claims", "claims-detail", "metadata"} {
		start, end, err := fenceSpan(original, name, name == "claims-detail" || name == "metadata")
		if err != nil {
			return nil, err
		}
		if start < 0 {
			continue
		}
		if start < last {
			return nil, fmt.Errorf("derived fence: overlapping or reordered %s block", name)
		}
		last = end
	}
	out := bytes.Clone(original)
	for _, name := range []string{"summary", "claims", "claims-detail", "metadata"} {
		optional := name == "claims-detail" || name == "metadata"
		start, end, err := fenceSpan(out, name, optional)
		if err != nil {
			return nil, err
		}
		fs, fe, err := fenceSpan(fresh, name, optional)
		if err != nil {
			return nil, err
		}
		if start < 0 && fs < 0 {
			continue
		}
		var replacement []byte
		if fs >= 0 {
			replacement = fresh[fs:fe]
		}
		if start < 0 {
			anchor := "claims"
			if name == "metadata" {
				anchor = "timeline"
			}
			_, end, err = fenceSpan(out, anchor, false)
			if err != nil {
				return nil, err
			}
			start = end
			replacement = append([]byte("\n\n"), replacement...)
		}
		merged := make([]byte, 0, len(out)-(end-start)+len(replacement))
		merged = append(merged, out[:start]...)
		merged = append(merged, replacement...)
		out = append(merged, out[end:]...)
	}
	return out, nil
}
