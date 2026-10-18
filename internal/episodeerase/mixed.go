package episodeerase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/sirerun/serenity/internal/store"
	"gopkg.in/yaml.v3"
)

// SourcePin binds the exact bytes and metadata of an independently retained raw
// source. The caller asserts independence from erased evidence under exclusive
// maintenance ownership; neither a hash nor an allowed kind proves provenance.
type SourcePin struct {
	SHA256         string `json:"sha256"`
	MetadataSHA256 string `json:"metadata_sha256"`
}

// MixedRequest adds a complete inventory of independent raw source identities
// occurring anywhere in the qualified history. Every pin must occur, and every
// non-episode source must be pinned. A single identity must have identical
// metadata in every revision; changed metadata requires separate qualification.
type MixedRequest struct {
	Request
	IndependentSources []SourcePin
}

type mixedSources struct {
	pins  []SourcePin
	bySHA map[string]string
	seen  map[string]bool
}

// BuildMixed constructs only an inactive candidate. It accepts the narrow raw
// file/email/voice subset with canonical metadata and no dependency metadata.
// It provides no activation, authenticated provenance, hosted-brain support or
// physical-purge completion. Build retains its strict episode-only behavior.
func BuildMixed(ctx context.Context, req MixedRequest) (Result, error) {
	if len(req.IndependentSources) == 0 || len(req.IndependentSources) > 2048 {
		return Result{}, ErrUnsafe
	}
	m := &mixedSources{pins: append([]SourcePin(nil), req.IndependentSources...), bySHA: map[string]string{}, seen: map[string]bool{}}
	for _, pin := range m.pins {
		if !store.ValidSourceSHA(pin.SHA256) || !store.ValidSourceSHA(pin.MetadataSHA256) || m.bySHA[pin.SHA256] != "" {
			return Result{}, ErrUnsafe
		}
		m.bySHA[pin.SHA256] = pin.MetadataSHA256
	}
	sort.Slice(m.pins, func(i, j int) bool { return m.pins[i].SHA256 < m.pins[j].SHA256 })
	return build(ctx, req.Request, m)
}

func rawDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (m *mixedSources) validateSnapshot(root string, files map[string][]byte, episodes map[string]string) error {
	episodeIDs := map[string]bool{}
	for _, sha := range episodes {
		episodeIDs[sha] = true
	}
	pairs := 0
	for path, data := range files {
		if !strings.HasSuffix(path, "/bytes") {
			continue
		}
		parts := strings.Split(path, "/")
		if len(parts) != 5 {
			return ErrUnsafe
		}
		sha := parts[3]
		metadata, ok := files[sourceBase(sha)+"meta.yaml"]
		if !ok {
			return ErrUnsafe
		}
		pairs++
		if episodeIDs[sha] {
			// Pins for episode records cannot be reinterpreted as raw evidence.
			if m.bySHA[sha] != "" {
				return ErrUnsafe
			}
			continue
		}
		if m.bySHA[sha] == "" || rawDigest(data) != sha || rawDigest(metadata) != m.bySHA[sha] {
			return ErrUnsafe
		}
		_, src, err := store.NewSourceStore(root).Read(sha)
		if err != nil || src.IndexOnly || len(src.Meta) != 0 {
			return ErrUnsafe
		}
		switch src.Kind {
		case "file", "email", "voice":
		default:
			return ErrUnsafe
		}
		// Exact canonical encoding rejects unknown fields, aliases, extra documents
		// and dependency metadata that SourceStore's ordinary reader may ignore.
		var canonical struct {
			Kind       string `yaml:"kind"`
			URI        string `yaml:"uri"`
			OccurredAt string `yaml:"occurred_at"`
		}
		if yaml.Unmarshal(metadata, &canonical) != nil {
			return ErrUnsafe
		}
		encoded, err := yaml.Marshal(canonical)
		if err != nil || !bytes.Equal(encoded, metadata) {
			return ErrUnsafe
		}
		m.seen[sha] = true
	}
	if len(files) != 2*pairs {
		return ErrUnsafe
	}
	return nil
}
