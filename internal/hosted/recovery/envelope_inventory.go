package recovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

// BrainInventoryDigestV1 computes an integrity digest over the supplied
// manifest-shaped brain inventory. It does not establish that these rows
// were inspected or that artifact bytes exist.
func BrainInventoryDigestV1(ctx context.Context, brains []contracts.BrainArtifact) (string, error) {
	if err := ensureContext(ctx); err != nil {
		return "", err
	}
	if len(brains) > maxSnapshotBrains {
		return "", fmt.Errorf("%w: brain inventory exceeds limit", ErrRecoveryEnvelopeTooLarge)
	}
	var totalHeads int
	nonempty := 0
	for i, brain := range brains {
		if err := ensureContext(ctx); err != nil {
			return "", err
		}
		if !validName(brain.ID, maxEnvelopeBrainIDBytes) || i > 0 && brains[i-1].ID >= brain.ID {
			return "", fmt.Errorf("%w: brain ID invalid or inventory unsorted at index %d", ErrRecoveryEnvelopeInvalid, i)
		}
		if brain.Heads == nil && !brain.Empty {
			// Empty source slices are emitted as [] by the canonical projection,
			// but nil indicates absent logical data for this typed API.
			return "", fmt.Errorf("%w: brain heads must be present at index %d", ErrRecoveryEnvelopeInvalid, i)
		}
		if len(brain.Heads) > maxEnvelopeHeadsPerBrain || len(brain.Heads) > maxEnvelopeHeads-totalHeads {
			return "", fmt.Errorf("%w: brain head inventory exceeds limit", ErrRecoveryEnvelopeTooLarge)
		}
		totalHeads += len(brain.Heads)
		if brain.Empty {
			if brain.ArtifactRef != (contracts.ArtifactRef{}) || len(brain.Heads) != 0 {
				return "", fmt.Errorf("%w: empty brain carries artifact or heads", ErrRecoveryEnvelopeInvalid)
			}
			continue
		}
		nonempty++
		if len(brain.Heads) == 0 || !validName(brain.RelativePath, maxEnvelopePathBytes) || brain.LengthBytes <= 0 || brain.LengthBytes > maxEnvelopeArtifactBytes || !validHash(brain.SHA256) {
			return "", fmt.Errorf("%w: nonempty brain artifact invalid", ErrRecoveryEnvelopeInvalid)
		}
		for j, head := range brain.Heads {
			if !validHeadRef(head.Ref) || !validObjectID(head.ObjectID) || j > 0 && brain.Heads[j-1].Ref >= head.Ref {
				return "", fmt.Errorf("%w: brain heads invalid or unsorted at brain %d index %d", ErrRecoveryEnvelopeInvalid, i, j)
			}
		}
	}
	if totalHeads > maxEnvelopeHeads || nonempty > maxSnapshotBrains {
		return "", fmt.Errorf("%w: brain inventory exceeds limit", ErrRecoveryEnvelopeTooLarge)
	}
	wire := brainPayload(brains)
	encoded, err := canonicalJSON(wire, maxSnapshotInventoryBytes)
	if err != nil {
		if errors.Is(err, ErrRecoveryEnvelopeTooLarge) {
			return "", err
		}
		return "", fmt.Errorf("%w: encode brain projection: %v", ErrRecoveryEnvelopeInvalid, err)
	}
	if err := ensureContext(ctx); err != nil {
		return "", err
	}
	digest := domainHash(brainHashDomain, encoded)
	if err := ensureContext(ctx); err != nil {
		return "", err
	}
	return digest, nil
}

// SnapshotInventorySummaryDigestV1 hashes the summary fields present in the
// envelope wire. It cannot verify the brain projection or artifact totals.
func SnapshotInventorySummaryDigestV1(ctx context.Context, in SnapshotInventorySummaryV1) (string, error) {
	if err := ensureContext(ctx); err != nil {
		return "", err
	}
	if err := validateInventorySummary(ctx, in); err != nil {
		return "", err
	}
	payload := inventoryPayload(in, in.BrainInventorySHA256, in.BrainCount)
	encoded, err := canonicalJSON(payload, maxSnapshotInventoryBytes)
	if err != nil {
		if errors.Is(err, ErrRecoveryEnvelopeTooLarge) {
			return "", err
		}
		return "", fmt.Errorf("%w: encode inventory summary: %v", ErrRecoveryEnvelopeInvalid, err)
	}
	if err := ensureContext(ctx); err != nil {
		return "", err
	}
	digest := domainHash(inventoryHashDomain, encoded)
	if err := ensureContext(ctx); err != nil {
		return "", err
	}
	return digest, nil
}

// SnapshotInventoryDigestV1 verifies arithmetic against the full supplied
// brain rows, derives their digest, then hashes the common compact summary.
func SnapshotInventoryDigestV1(ctx context.Context, in SnapshotInventoryV1) (string, error) {
	if err := ensureContext(ctx); err != nil {
		return "", err
	}
	brainDigest, err := BrainInventoryDigestV1(ctx, in.Brains)
	if err != nil {
		return "", err
	}
	if in.ControlDBLength <= 0 || in.ControlDBLength > maxEnvelopeArtifactBytes {
		return "", fmt.Errorf("%w: control database length outside limit", ErrRecoveryEnvelopeInvalid)
	}
	artifactCount := 1
	declaredBytes := in.ControlDBLength
	for _, brain := range in.Brains {
		if err := ensureContext(ctx); err != nil {
			return "", err
		}
		if brain.Empty {
			continue
		}
		artifactCount++
		var ok bool
		declaredBytes, ok = checkedAdd(declaredBytes, brain.LengthBytes, maxEnvelopeArtifactBytes)
		if !ok {
			return "", fmt.Errorf("%w: declared artifact byte total overflow or exceeds ceiling", ErrRecoveryEnvelopeInvalid)
		}
	}
	if artifactCount > maxEnvelopeArtifactCount || declaredBytes <= 0 || declaredBytes > maxEnvelopeArtifactBytes {
		return "", fmt.Errorf("%w: artifact inventory exceeds limit", ErrRecoveryEnvelopeTooLarge)
	}
	if in.VerifiedArtifactCount != artifactCount || in.DeclaredArtifactBytes != declaredBytes {
		return "", fmt.Errorf("%w: verified artifact count or byte total does not match projection", ErrRecoveryEnvelopeInvalid)
	}
	summary := SnapshotInventorySummaryV1{
		ManifestSHA256:        in.ManifestSHA256,
		SourceBuildToken:      in.SourceBuildToken,
		SourceSchemaVersion:   in.SourceSchemaVersion,
		Accounts:              in.Accounts,
		ControlDBLength:       in.ControlDBLength,
		BrainCount:            len(in.Brains),
		BrainInventorySHA256:  brainDigest,
		VerifiedArtifactCount: artifactCount,
		DeclaredArtifactBytes: declaredBytes,
	}
	return SnapshotInventorySummaryDigestV1(ctx, summary)
}

func validateInventorySummary(ctx context.Context, in SnapshotInventorySummaryV1) error {
	if err := ensureContext(ctx); err != nil {
		return err
	}
	if !validHash(in.ManifestSHA256) || !validText(in.SourceBuildToken, maxEnvelopeSourceTokenBytes) || in.SourceSchemaVersion < 1 || in.SourceSchemaVersion > math.MaxInt32 {
		return fmt.Errorf("%w: invalid source identity in inventory", ErrRecoveryEnvelopeInvalid)
	}
	if err := validAccountRows(in.Accounts); err != nil {
		return err
	}
	if in.ControlDBLength <= 0 || in.ControlDBLength > maxEnvelopeArtifactBytes || in.BrainCount < 0 || in.BrainCount > maxSnapshotBrains || !validHash(in.BrainInventorySHA256) {
		return fmt.Errorf("%w: invalid inventory summary", ErrRecoveryEnvelopeInvalid)
	}
	if in.VerifiedArtifactCount < 1 || in.VerifiedArtifactCount > maxEnvelopeArtifactCount || in.DeclaredArtifactBytes < in.ControlDBLength || in.DeclaredArtifactBytes > maxEnvelopeArtifactBytes {
		return fmt.Errorf("%w: inventory artifact summary outside limits", ErrRecoveryEnvelopeInvalid)
	}
	if in.BrainCount == 0 {
		emptyBytes, err := json.Marshal(brainInventoryPayloadV1{Version: 1, Brains: make([]brainV1Wire, 0)})
		if err != nil {
			return fmt.Errorf("%w: encode empty brain projection", ErrRecoveryEnvelopeInvalid)
		}
		emptyDigest := domainHash(brainHashDomain, emptyBytes)
		if emptyDigest != in.BrainInventorySHA256 || in.VerifiedArtifactCount != 1 || in.DeclaredArtifactBytes != in.ControlDBLength {
			return fmt.Errorf("%w: empty brain inventory summary mismatch", ErrRecoveryEnvelopeInvalid)
		}
	}
	if err := ensureContext(ctx); err != nil {
		return err
	}
	return nil
}
