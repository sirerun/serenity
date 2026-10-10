package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sirerun/serenity/internal/hosted/authority"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/deletion"
)

const authoritySidecar = "authority-cut.json"
const maxAuthoritySidecarBytes = 16 * 1024

// CutAuthority authenticates the remote cut; snapshot files never authorize
// themselves. The production implementation is authority.Client over the broker.
type CutAuthority interface {
	Cut(context.Context, authority.Lease, string, string) (authority.Cut, error)
	FinalizeCut(context.Context, authority.Cut, string, string) (authority.FinalizedCut, error)
	ReadFinalizedCut(context.Context, string) (authority.FinalizedCut, error)
	CutReader(authority.Cut) contracts.JournalObjectReader
}

type cutSidecar struct {
	Version        int    `json:"version"`
	CutID          string `json:"cut_id"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

type observedWatermark struct{ read contracts.DeletionRead }

func (r observedWatermark) ReadThrough(ctx context.Context, after contracts.DeletionWatermark) (contracts.DeletionRead, error) {
	if err := ctx.Err(); err != nil {
		return contracts.DeletionRead{}, err
	}
	if !after.IsZero() {
		return contracts.DeletionRead{}, errors.New("backup cut requires complete history")
	}
	return r.read, nil
}

// CreateAuthorized runs only under the service maintenance lock (or equivalent
// exclusive offline lock). Its immutable authority cut precedes all data copies;
// the snapshot is published only after authoritative manifest finalization.
func CreateAuthorized(ctx context.Context, dataDir, destination, buildSHA string, a CutAuthority, lease authority.Lease) error {
	if isNilInterface(ctx) {
		return ErrNilContext
	}
	if isNilInterface(a) {
		return errors.New("hosted/backup: missing cut authority")
	}
	attempt, err := authority.NewOperationID()
	if err != nil {
		return err
	}
	op, err := authority.NewOperationID()
	if err != nil {
		return err
	}
	cut, err := a.Cut(ctx, lease, attempt, op)
	if err != nil {
		return err
	}
	if cut.ID == "" || cut.SnapshotAttemptID != attempt || cut.LeaseID != lease.ID {
		return errors.New("hosted/backup: authority returned a mismatched cut")
	}
	observation, err := observeCut(ctx, a, cut)
	if err != nil {
		return err
	}
	return createWithFinalizer(ctx, dataDir, destination, buildSHA, observedWatermark{observation.Read},
		func(ctx context.Context, staging string, manifest []byte) error {
			sum := sha256.Sum256(manifest)
			hash := hex.EncodeToString(sum[:])
			finalOp, err := authority.NewOperationID()
			if err != nil {
				return err
			}
			finalized, err := a.FinalizeCut(ctx, cut, hash, finalOp)
			if err != nil {
				return err
			}
			if finalized.Cut != cut || finalized.ManifestSHA256 != hash || finalized.OperationID != finalOp {
				return errors.New("hosted/backup: authority finalization mismatch")
			}
			body, err := json.Marshal(cutSidecar{1, cut.ID, hash})
			if err != nil {
				return err
			}
			return writeFileSynced(filepath.Join(staging, authoritySidecar), body, 0600)
		})
}

func observeCut(ctx context.Context, a CutAuthority, cut authority.Cut) (contracts.JournalObservation, error) {
	reader, err := deletion.NewJournalReader(a.CutReader(cut), cut.Position)
	if err != nil {
		return contracts.JournalObservation{}, err
	}
	observed, err := reader.Observe(ctx)
	if err != nil {
		return contracts.JournalObservation{}, err
	}
	if observed.Position != cut.Position {
		return contracts.JournalObservation{}, contracts.ErrDeletionJournalHistoryMismatch
	}
	return observed, nil
}

// RestoreAuthorized verifies the cut against the authority and validates the
// exact manifest bytes subsequently used by restore. It produces only the
// normal frozen, pending-reactivation destination, never startup admission.
func RestoreAuthorized(ctx context.Context, snapshot, destination string, a CutAuthority) error {
	if isNilInterface(ctx) {
		return ErrNilContext
	}
	if isNilInterface(a) {
		return errors.New("hosted/backup: missing cut authority")
	}
	return restoreWithManifestReader(ctx, snapshot, destination, "", nil,
		func(ctx context.Context, root *os.Root) (contracts.ManifestV2, error) {
			var zero contracts.ManifestV2
			side, err := readCutSidecar(root)
			if err != nil {
				return zero, err
			}
			finalized, err := a.ReadFinalizedCut(ctx, side.CutID)
			if err != nil {
				return zero, err
			}
			if finalized.Cut.ID != side.CutID || finalized.ManifestSHA256 != side.ManifestSHA256 {
				return zero, errors.New("hosted/backup: snapshot authority binding mismatch")
			}
			observed, err := observeCut(ctx, a, finalized.Cut)
			if err != nil {
				return zero, err
			}
			raw, err := readManifestBytes(ctx, root)
			if err != nil {
				return zero, err
			}
			sum := sha256.Sum256(raw)
			if hex.EncodeToString(sum[:]) != finalized.ManifestSHA256 {
				return zero, errors.New("hosted/backup: finalized manifest digest mismatch")
			}
			manifest, err := parseManifest(raw)
			if err != nil {
				return zero, err
			}
			if manifest.JournalWatermark != observed.Read.To {
				return zero, errors.New("hosted/backup: cut watermark mismatch")
			}
			return manifest, nil
		})
}

func readCutSidecar(root *os.Root) (result cutSidecar, resultErr error) {
	f, err := openSnapshotRegular(root, authoritySidecar)
	if err != nil {
		return result, err
	}
	defer func() { resultErr = errors.Join(resultErr, f.Close()) }()
	raw, err := io.ReadAll(io.LimitReader(f, maxAuthoritySidecarBytes+1))
	if err != nil {
		return result, err
	}
	if len(raw) > maxAuthoritySidecarBytes {
		return result, errors.New("hosted/backup: oversized authority sidecar")
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	canonical, err := json.Marshal(result)
	if err != nil {
		return result, err
	}
	if !bytes.Equal(raw, canonical) || result.Version != 1 || len(result.CutID) != 64 || len(result.ManifestSHA256) != 64 {
		return cutSidecar{}, fmt.Errorf("hosted/backup: noncanonical authority sidecar")
	}
	for _, value := range []string{result.CutID, result.ManifestSHA256} {
		decoded, err := hex.DecodeString(value)
		if err != nil || hex.EncodeToString(decoded) != value {
			return cutSidecar{}, errors.New("hosted/backup: invalid authority sidecar digest")
		}
	}
	return result, nil
}
