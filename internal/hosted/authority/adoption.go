package authority

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// LegacyHistoryOrigin explicitly records that authoritative deletion history
// starts at an owner-preserved checkpoint. It asserts no earlier journal history.
const LegacyHistoryOrigin = "owner-legacy-checkpoint-v1"

// Adoption binds the first writer of a new authority lineage to a preserved,
// stopped legacy data tree. It is owner enrollment evidence, never a cut or a
// replacement for deletion history from an existing authority lineage.
type Adoption struct {
	Version          int    `json:"version"`
	HistoryOrigin    string `json:"history_origin"`
	CheckpointSHA256 string `json:"checkpoint_sha256"`
	InventorySHA256  string `json:"inventory_sha256"`
	PriorBuildSHA    string `json:"prior_build_sha"`
}

func ValidateAdoption(a Adoption) error { return a.Validate() }

func (a Adoption) Validate() error {
	if a.Version != 1 || a.HistoryOrigin != LegacyHistoryOrigin ||
		!validID(a.CheckpointSHA256) || !validID(a.InventorySHA256) {
		return ErrInvalid
	}
	build, err := hex.DecodeString(a.PriorBuildSHA)
	if err != nil || (len(build) != 20 && len(build) != 32) ||
		strings.ToLower(a.PriorBuildSHA) != a.PriorBuildSHA {
		return ErrInvalid
	}
	return nil
}

// SHA256 hashes the canonical JSON receipt, including its truthful origin.
func (a Adoption) SHA256() (string, error) {
	if err := a.Validate(); err != nil {
		return "", err
	}
	b, err := json.Marshal(a)
	if err != nil {
		return "", err
	}
	return digest(b), nil
}

func ParseAdoption(b []byte) (Adoption, error) {
	var a Adoption
	if len(b) > 4096 {
		return a, ErrInvalid
	}
	if err := strictDecode(b, &a); err != nil {
		return a, err
	}
	return a, a.Validate()
}

func (c *Client) EnrollWithAdoption(ctx context.Context, a Adoption, op string) error {
	if err := a.Validate(); err != nil {
		return err
	}
	_, err := c.invoke(ctx, Request{Action: "enroll", OperationID: op, Adoption: &a})
	return err
}
