package recovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

const (
	envelopeFormatVersion       = 1
	maxEnvelopeBytes            = 4 << 20
	maxSnapshotInventoryBytes   = 1 << 20
	maxSnapshotAccounts         = 10_000
	maxSnapshotBrains           = 10_000
	maxEnvelopeEligibleAccounts = 100
	maxEnvelopeDispositions     = 10_000
	maxEnvelopeHeads            = 100_000
	maxEnvelopeHeadsPerBrain    = 1_000
	maxEnvelopeArtifactCount    = 100_000
	maxEnvelopeArtifactBytes    = int64(1 << 40)
	maxEnvelopeEvidenceBytes    = 1 << 20
	maxEnvelopeNesting          = 12
	maxEnvelopeSourceTokenBytes = 256
	maxEnvelopeFieldBytes       = 256
	maxEnvelopeBrainIDBytes     = 255
	maxEnvelopePathBytes        = 255
	maxEnvelopeHeadRefBytes     = 1024
	maxEnvelopePlanBytes        = 1 << 20
)

const (
	envelopeHashDomain  = "serenity.recovery-envelope.v1\x00"
	brainHashDomain     = "serenity.recovery-brain-inventory.v1\x00"
	inventoryHashDomain = "serenity.recovery-snapshot-inventory.v1\x00"
)

type envelopeV1Wire struct {
	FormatVersion            int                      `json:"format_version"`
	Kind                     string                   `json:"kind"`
	PlanRef                  string                   `json:"plan_ref"`
	SnapshotPinID            string                   `json:"snapshot_pin_id"`
	ReservationVersion       int64                    `json:"reservation_version"`
	ManifestSHA256           string                   `json:"manifest_sha256"`
	ManifestJournalWatermark watermarkV1Wire          `json:"manifest_journal_watermark"`
	SourceBuildToken         string                   `json:"source_build_token"`
	SourceSchemaVersion      int                      `json:"source_schema_version"`
	SnapshotAccountInventory []accountInventoryV1Wire `json:"snapshot_account_inventory"`
	SnapshotBrainInventory   brainInventoryV1Wire     `json:"snapshot_brain_inventory"`
	ControlDBLength          int64                    `json:"control_db_length"`
	VerifiedArtifactCount    int                      `json:"verified_artifact_count"`
	DeclaredArtifactBytes    int64                    `json:"declared_artifact_bytes"`
	SnapshotInventorySHA256  string                   `json:"snapshot_inventory_sha256"`
	PlanApprovalRef          evidenceRefV1Wire        `json:"plan_approval_ref"`
	PlanApprovalDigest       string                   `json:"plan_approval_digest"`
	PlanApprovalNonce        string                   `json:"plan_approval_nonce"`
	PlanApprovalExpiresAt    string                   `json:"plan_approval_expires_at"`
	ActivationAllowlist      []string                 `json:"activation_allowlist"`
	OperationID              string                   `json:"operation_id"`
	OldWriter                writerV1Wire             `json:"old_writer"`
	JournalStoreID           string                   `json:"journal_store_id"`
	SnapshotCut              journalPositionV1Wire    `json:"snapshot_cut"`
	LastSealed               watermarkV1Wire          `json:"last_sealed"`
	PrefixEvidenceRef        evidenceRefV1Wire        `json:"prefix_evidence_ref"`
	Eligible                 *eligibleArmV1Wire       `json:"eligible,omitempty"`
	Frozen                   *frozenArmV1Wire         `json:"frozen,omitempty"`
}

type watermarkV1Wire struct {
	Generation int64  `json:"generation"`
	SequenceID int64  `json:"sequence_id"`
	EntryHash  string `json:"entry_hash"`
}

type journalPositionV1Wire struct {
	ActiveGeneration       int64           `json:"active_generation"`
	LastObjectInGeneration watermarkV1Wire `json:"last_object_in_generation"`
	PredecessorSeal        watermarkV1Wire `json:"predecessor_seal"`
	GenesisIssuanceID      string          `json:"genesis_issuance_id"`
	SuccessorAllocationID  string          `json:"successor_allocation_id"`
}

type accountInventoryV1Wire struct {
	AccountID string `json:"account_id"`
	Status    string `json:"status"`
}

type brainInventoryV1Wire struct {
	Count           int    `json:"count"`
	CanonicalDigest string `json:"canonical_digest"`
}

type evidenceRefV1Wire struct {
	Authority string `json:"authority"`
	RecordID  string `json:"record_id"`
	Version   string `json:"version"`
}

type writerV1Wire struct {
	Provider       string `json:"provider"`
	ScopeRef       string `json:"scope_ref"`
	WriterRef      string `json:"writer_ref"`
	BootRef        string `json:"boot_ref"`
	JournalStoreID string `json:"journal_store_id"`
	Generation     int64  `json:"generation"`
}

type eligibleArmV1Wire struct {
	LegacyPlanArtifactHash   string             `json:"legacy_plan_artifact_hash"`
	RecoveryContractPlanHash string             `json:"recovery_contract_plan_hash"`
	ContractPlan             contractPlanV1Wire `json:"contract_plan"`
	EligibleAccountIDs       []string           `json:"eligible_account_ids"`
}

type contractPlanV1Wire struct {
	SourceSnapshot   string          `json:"source_snapshot"`
	JournalWatermark watermarkV1Wire `json:"journal_watermark"`
	Generation       int64           `json:"generation"`
	ProviderTruthAt  string          `json:"provider_truth_at"`
	Accounts         []string        `json:"accounts"`
}

type frozenArmV1Wire struct {
	Dispositions []frozenDispositionV1Wire `json:"dispositions"`
}

type frozenDispositionV1Wire struct {
	AccountID   string `json:"account_id"`
	Disposition string `json:"disposition"`
	ReasonCode  string `json:"reason_code"`
}

type snapshotInventoryPayloadV1 struct {
	Version               int                      `json:"version"`
	ManifestSHA256        string                   `json:"manifest_sha256"`
	SourceBuildToken      string                   `json:"source_build_token"`
	SourceSchemaVersion   int                      `json:"source_schema_version"`
	Accounts              []accountInventoryV1Wire `json:"accounts"`
	ControlDBLength       int64                    `json:"control_db_length"`
	BrainCount            int                      `json:"brain_count"`
	BrainInventorySHA256  string                   `json:"brain_inventory_sha256"`
	VerifiedArtifactCount int                      `json:"verified_artifact_count"`
	DeclaredArtifactBytes int64                    `json:"declared_artifact_bytes"`
}

type brainInventoryPayloadV1 struct {
	Version int           `json:"version"`
	Brains  []brainV1Wire `json:"brains"`
}

type brainV1Wire struct {
	ID       string              `json:"id"`
	Empty    bool                `json:"empty"`
	Artifact brainArtifactV1Wire `json:"artifact"`
	Heads    []brainHeadV1Wire   `json:"heads"`
}

type brainArtifactV1Wire struct {
	RelativePath string `json:"relative_path"`
	Length       int64  `json:"length"`
	SHA256       string `json:"sha256"`
}

type brainHeadV1Wire struct {
	Ref      string `json:"ref"`
	ObjectID string `json:"object_id"`
}

func (w WatermarkV1) wire() watermarkV1Wire {
	return watermarkV1Wire(w)
}

func watermarkFromWire(w watermarkV1Wire) WatermarkV1 {
	return WatermarkV1(w)
}

func positionWire(p JournalPositionV1) journalPositionV1Wire {
	return journalPositionV1Wire{
		ActiveGeneration:       p.ActiveGeneration,
		LastObjectInGeneration: p.LastObjectInGeneration.wire(),
		PredecessorSeal:        p.PredecessorSeal.wire(),
		GenesisIssuanceID:      p.GenesisIssuanceID,
		SuccessorAllocationID:  p.SuccessorAllocationID,
	}
}

func positionFromWire(p journalPositionV1Wire) JournalPositionV1 {
	return JournalPositionV1{
		ActiveGeneration:       p.ActiveGeneration,
		LastObjectInGeneration: watermarkFromWire(p.LastObjectInGeneration),
		PredecessorSeal:        watermarkFromWire(p.PredecessorSeal),
		GenesisIssuanceID:      p.GenesisIssuanceID,
		SuccessorAllocationID:  p.SuccessorAllocationID,
	}
}

func toEnvelopeWire(in RecoveryEnvelopeV1) envelopeV1Wire {
	out := envelopeV1Wire{
		FormatVersion:            in.FormatVersion,
		Kind:                     string(in.Kind),
		PlanRef:                  in.PlanRef,
		SnapshotPinID:            in.SnapshotPinID,
		ReservationVersion:       in.ReservationVersion,
		ManifestSHA256:           in.ManifestSHA256,
		ManifestJournalWatermark: in.ManifestJournalWatermark.wire(),
		SourceBuildToken:         in.SourceBuildToken,
		SourceSchemaVersion:      in.SourceSchemaVersion,
		SnapshotAccountInventory: make([]accountInventoryV1Wire, len(in.SnapshotAccountInventory)),
		SnapshotBrainInventory:   brainInventoryV1Wire{Count: in.SnapshotBrainInventory.Count, CanonicalDigest: in.SnapshotBrainInventory.CanonicalDigest},
		ControlDBLength:          in.ControlDBLength,
		VerifiedArtifactCount:    in.VerifiedArtifactCount,
		DeclaredArtifactBytes:    in.DeclaredArtifactBytes,
		SnapshotInventorySHA256:  in.SnapshotInventorySHA256,
		PlanApprovalRef:          evidenceRefV1Wire{Authority: in.PlanApprovalRef.Authority, RecordID: in.PlanApprovalRef.RecordID, Version: in.PlanApprovalRef.Version},
		PlanApprovalDigest:       in.PlanApprovalDigest,
		PlanApprovalNonce:        in.PlanApprovalNonce,
		PlanApprovalExpiresAt:    in.PlanApprovalExpiresAt.Format(time.RFC3339Nano),
		ActivationAllowlist:      cloneSlice(in.ActivationAllowlist),
		OperationID:              in.OperationID,
		OldWriter:                writerV1Wire{Provider: in.OldWriter.Provider, ScopeRef: in.OldWriter.ScopeRef, WriterRef: in.OldWriter.WriterRef, BootRef: in.OldWriter.BootRef, JournalStoreID: in.OldWriter.JournalStoreID, Generation: in.OldWriter.Generation},
		JournalStoreID:           in.JournalStoreID,
		SnapshotCut:              positionWire(in.SnapshotCut),
		LastSealed:               in.LastSealed.wire(),
		PrefixEvidenceRef:        evidenceRefV1Wire{Authority: in.PrefixEvidenceRef.Authority, RecordID: in.PrefixEvidenceRef.RecordID, Version: in.PrefixEvidenceRef.Version},
	}
	for i, account := range in.SnapshotAccountInventory {
		out.SnapshotAccountInventory[i] = accountInventoryV1Wire{AccountID: account.ID, Status: account.Status}
	}
	if in.Eligible != nil {
		plan := in.Eligible.ContractPlan
		out.Eligible = &eligibleArmV1Wire{
			LegacyPlanArtifactHash:   in.Eligible.LegacyPlanArtifactHash,
			RecoveryContractPlanHash: in.Eligible.RecoveryContractPlanHash,
			ContractPlan: contractPlanV1Wire{
				SourceSnapshot:   plan.SourceSnapshot,
				JournalWatermark: plan.JournalWatermark.wire(),
				Generation:       plan.Generation,
				ProviderTruthAt:  plan.ProviderTruthAt.Format(time.RFC3339Nano),
				Accounts:         cloneSlice(plan.Accounts),
			},
			EligibleAccountIDs: cloneSlice(in.Eligible.EligibleAccountIDs),
		}
	}
	if in.Frozen != nil {
		out.Frozen = &frozenArmV1Wire{Dispositions: make([]frozenDispositionV1Wire, len(in.Frozen.Dispositions))}
		for i, item := range in.Frozen.Dispositions {
			out.Frozen.Dispositions[i] = frozenDispositionV1Wire(item)
		}
	}
	return out
}

func fromEnvelopeWire(in envelopeV1Wire) (RecoveryEnvelopeV1, error) {
	expires, err := parseCanonicalUTC(in.PlanApprovalExpiresAt)
	if err != nil {
		return RecoveryEnvelopeV1{}, err
	}
	out := RecoveryEnvelopeV1{
		FormatVersion:            in.FormatVersion,
		Kind:                     RecoveryEnvelopeKind(in.Kind),
		PlanRef:                  in.PlanRef,
		SnapshotPinID:            in.SnapshotPinID,
		ReservationVersion:       in.ReservationVersion,
		ManifestSHA256:           in.ManifestSHA256,
		ManifestJournalWatermark: watermarkFromWire(in.ManifestJournalWatermark),
		SourceBuildToken:         in.SourceBuildToken,
		SourceSchemaVersion:      in.SourceSchemaVersion,
		SnapshotAccountInventory: make([]SnapshotAccountV1, len(in.SnapshotAccountInventory)),
		SnapshotBrainInventory:   BrainInventoryV1{Count: in.SnapshotBrainInventory.Count, CanonicalDigest: in.SnapshotBrainInventory.CanonicalDigest},
		ControlDBLength:          in.ControlDBLength,
		VerifiedArtifactCount:    in.VerifiedArtifactCount,
		DeclaredArtifactBytes:    in.DeclaredArtifactBytes,
		SnapshotInventorySHA256:  in.SnapshotInventorySHA256,
		PlanApprovalRef:          EvidenceRefV1{Authority: in.PlanApprovalRef.Authority, RecordID: in.PlanApprovalRef.RecordID, Version: in.PlanApprovalRef.Version},
		PlanApprovalDigest:       in.PlanApprovalDigest,
		PlanApprovalNonce:        in.PlanApprovalNonce,
		PlanApprovalExpiresAt:    expires,
		ActivationAllowlist:      cloneSlice(in.ActivationAllowlist),
		OperationID:              in.OperationID,
		OldWriter:                WriterV1{Provider: in.OldWriter.Provider, ScopeRef: in.OldWriter.ScopeRef, WriterRef: in.OldWriter.WriterRef, BootRef: in.OldWriter.BootRef, JournalStoreID: in.OldWriter.JournalStoreID, Generation: in.OldWriter.Generation},
		JournalStoreID:           in.JournalStoreID,
		SnapshotCut:              positionFromWire(in.SnapshotCut),
		LastSealed:               watermarkFromWire(in.LastSealed),
		PrefixEvidenceRef:        EvidenceRefV1{Authority: in.PrefixEvidenceRef.Authority, RecordID: in.PrefixEvidenceRef.RecordID, Version: in.PrefixEvidenceRef.Version},
	}
	for i, account := range in.SnapshotAccountInventory {
		out.SnapshotAccountInventory[i] = SnapshotAccountV1{ID: account.AccountID, Status: account.Status}
	}
	if in.Eligible != nil {
		providerTruth, err := parseCanonicalUTC(in.Eligible.ContractPlan.ProviderTruthAt)
		if err != nil {
			return RecoveryEnvelopeV1{}, err
		}
		plan := in.Eligible.ContractPlan
		out.Eligible = &EligibleArmV1{
			LegacyPlanArtifactHash:   in.Eligible.LegacyPlanArtifactHash,
			RecoveryContractPlanHash: in.Eligible.RecoveryContractPlanHash,
			ContractPlan: ContractPlanV1{
				SourceSnapshot:   plan.SourceSnapshot,
				JournalWatermark: watermarkFromWire(plan.JournalWatermark),
				Generation:       plan.Generation,
				ProviderTruthAt:  providerTruth,
				Accounts:         cloneSlice(plan.Accounts),
			},
			EligibleAccountIDs: cloneSlice(in.Eligible.EligibleAccountIDs),
		}
	}
	if in.Frozen != nil {
		out.Frozen = &FrozenArmV1{Dispositions: make([]FrozenDispositionV1, len(in.Frozen.Dispositions))}
		for i, item := range in.Frozen.Dispositions {
			out.Frozen.Dispositions[i] = FrozenDispositionV1(item)
		}
	}
	return out, nil
}

func inventoryPayload(in SnapshotInventorySummaryV1, brainDigest string, brainCount int) snapshotInventoryPayloadV1 {
	accounts := make([]accountInventoryV1Wire, len(in.Accounts))
	for i, account := range in.Accounts {
		accounts[i] = accountInventoryV1Wire{AccountID: account.ID, Status: account.Status}
	}
	return snapshotInventoryPayloadV1{
		Version:               1,
		ManifestSHA256:        in.ManifestSHA256,
		SourceBuildToken:      in.SourceBuildToken,
		SourceSchemaVersion:   in.SourceSchemaVersion,
		Accounts:              accounts,
		ControlDBLength:       in.ControlDBLength,
		BrainCount:            brainCount,
		BrainInventorySHA256:  brainDigest,
		VerifiedArtifactCount: in.VerifiedArtifactCount,
		DeclaredArtifactBytes: in.DeclaredArtifactBytes,
	}
}

func brainPayload(brains []contracts.BrainArtifact) brainInventoryPayloadV1 {
	out := brainInventoryPayloadV1{Version: 1, Brains: make([]brainV1Wire, len(brains))}
	for i, brain := range brains {
		row := brainV1Wire{ID: brain.ID, Empty: brain.Empty, Heads: make([]brainHeadV1Wire, len(brain.Heads))}
		row.Artifact = brainArtifactV1Wire{RelativePath: brain.RelativePath, Length: brain.LengthBytes, SHA256: brain.SHA256}
		for j, head := range brain.Heads {
			row.Heads[j] = brainHeadV1Wire{Ref: head.Ref, ObjectID: head.ObjectID}
		}
		out.Brains[i] = row
	}
	return out
}

func canonicalJSON(value any, max int) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(encoded) > max {
		return nil, ErrRecoveryEnvelopeTooLarge
	}
	return encoded, nil
}

func domainHash(domain string, payload []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte(domain))
	_, _ = h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

func parseCanonicalUTC(value string) (time.Time, error) {
	if len(value) == 0 || len(value) > 35 || !strings.HasSuffix(value, "Z") {
		return time.Time{}, fmt.Errorf("%w: timestamp is not canonical UTC", ErrRecoveryEnvelopeInvalid)
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Location() != time.UTC || parsed.Format(time.RFC3339Nano) != value {
		return time.Time{}, fmt.Errorf("%w: timestamp is not canonical UTC", ErrRecoveryEnvelopeInvalid)
	}
	return parsed, nil
}

func validHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func isZeroWatermark(w WatermarkV1) bool {
	return w == (WatermarkV1{})
}

func validWatermarkV1(w WatermarkV1) bool {
	return w.Generation >= 1 && w.Generation <= 10_000 && w.SequenceID >= 1 && w.SequenceID <= 9_999_999_999 && validHash(w.EntryHash)
}

func validJournalPositionV1(p JournalPositionV1) bool {
	if p.ActiveGeneration < 1 || p.ActiveGeneration > 10_000 {
		return false
	}
	if p.ActiveGeneration == 1 {
		if !validAuthorityComponent(p.GenesisIssuanceID) || p.SuccessorAllocationID != "" || !isZeroWatermark(p.PredecessorSeal) {
			return false
		}
	} else if !validAuthorityComponent(p.SuccessorAllocationID) || p.GenesisIssuanceID != "" || !validWatermarkV1(p.PredecessorSeal) || p.PredecessorSeal.Generation != p.ActiveGeneration-1 {
		return false
	}
	if !isZeroWatermark(p.LastObjectInGeneration) && (!validWatermarkV1(p.LastObjectInGeneration) || p.LastObjectInGeneration.Generation != p.ActiveGeneration) {
		return false
	}
	return true
}

func isZeroOrValidWatermark(w WatermarkV1) bool { return isZeroWatermark(w) || validWatermarkV1(w) }

func validAuthorityComponent(s string) bool {
	return validASCIIComponent(s, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:/-")
}

func validASCIIComponent(s, alphabet string) bool {
	if len(s) == 0 || len(s) > maxEnvelopeFieldBytes || !isASCIIAlphaNumeric(s[0]) || !isASCIIAlphaNumeric(s[len(s)-1]) {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !isASCIIAlphaNumeric(c) && !strings.ContainsRune(alphabet, rune(c)) {
			return false
		}
	}
	return true
}

func isASCIIAlphaNumeric(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func validText(s string, max int) bool {
	if len(s) == 0 || len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func validName(s string, max int) bool {
	if s == "" || len(s) > max || s == "." || s == ".." || !utf8.ValidString(s) || strings.ContainsAny(s, "/\\") {
		return false
	}
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validSnapshotAccountID(s string) bool {
	if len(s) < 16 || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isASCIIAlphaNumeric(s[i]) {
			return false
		}
	}
	return true
}

func validHeadRef(s string) bool {
	if s != "HEAD" {
		if !strings.HasPrefix(s, "refs/") || len(s) <= len("refs/") {
			return false
		}
	}
	if len(s) > maxEnvelopeHeadRefBytes || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validObjectID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if (s[i] < '0' || s[i] > '9') && (s[i] < 'a' || s[i] > 'f') {
			return false
		}
	}
	return true
}

func validSnapshotStatus(s string) bool {
	switch s {
	case "active", "deleted", "deleting", "restore_pending":
		return true
	default:
		return false
	}
}

func validAccountRows(rows []SnapshotAccountV1) error {
	if rows == nil || len(rows) > maxSnapshotAccounts {
		return fmt.Errorf("%w: missing or oversized account inventory", ErrRecoveryEnvelopeInvalid)
	}
	for i, row := range rows {
		if !validSnapshotAccountID(row.ID) || !validSnapshotStatus(row.Status) || i > 0 && rows[i-1].ID >= row.ID {
			return fmt.Errorf("%w: account inventory row %d invalid or unsorted", ErrRecoveryEnvelopeInvalid, i)
		}
	}
	return nil
}

func validateEvidenceRef(ref EvidenceRefV1) error {
	if !validASCIIComponent(ref.Authority, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:/-") ||
		!validASCIIComponent(ref.RecordID, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:/-") ||
		!validASCIIComponent(ref.Version, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:/-") {
		return fmt.Errorf("%w: invalid evidence reference fields", ErrRecoveryEnvelopeInvalid)
	}
	return nil
}

func ensureContext(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil context", ErrRecoveryEnvelopeContext)
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(ErrRecoveryEnvelopeContext, err)
	}
	return nil
}

func checkedAdd(total, value, max int64) (int64, bool) {
	if value < 0 || total < 0 || value > max-total || total > math.MaxInt64-value {
		return total, false
	}
	return total + value, true
}

func cloneSlice[T any](in []T) []T {
	if in == nil {
		return nil
	}
	out := make([]T, len(in))
	copy(out, in)
	return out
}

func sortedUniqueStrings(values []string, limit int, validator func(string) bool) bool {
	if values == nil || len(values) > limit {
		return false
	}
	for i, value := range values {
		if !validator(value) || i > 0 && values[i-1] >= value {
			return false
		}
	}
	return true
}

func containsSorted(values []string, target string) bool {
	i, ok := slices.BinarySearch(values, target)
	return ok && i < len(values)
}
