package store

import (
	"bytes"
	"encoding/json"

	"github.com/sirerun/serenity/internal/domain"
)

// ValidateEpisodeErasureSource accepts only the exact canonical pair produced
// by PutEpisode. Erasure candidate construction must not silently preserve or
// discard unknown JSON/YAML fields as though their dependency semantics were
// understood. Ordinary episode decoding remains unchanged.
func ValidateEpisodeErasureSource(r EpisodeRecord, raw, metadata []byte) error {
	if err := validateEpisodeRecord(r); err != nil {
		return err
	}
	encoded, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if !bytes.Equal(encoded, raw) {
		return ErrEpisodeInvalid
	}
	occurred := r.RecordedAt
	if r.Input != nil {
		if len(r.Input.Events) > 0 {
			occurred = r.Input.Events[0].OccurredAt
		} else if r.Input.Checkpoint != nil {
			occurred = r.Input.Checkpoint.CreatedAt
		}
	}
	meta, err := marshalSourceMeta(domain.Source{Kind: SourceKindEpisode, URI: "memory://episode", OccurredAt: occurred})
	if err != nil {
		return err
	}
	if !bytes.Equal(meta, metadata) {
		return ErrEpisodeInvalid
	}
	return nil
}
