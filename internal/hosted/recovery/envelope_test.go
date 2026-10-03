package recovery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

const (
	goldenBrainDigest       = "f1eb7d370f424a93bd89aef88ddf05822fdc1eded3042719339054086d210c3e"
	goldenInventoryDigest   = "04a8e01ab69e24a230882daabf374d67eb9bb2bb004c8b81bb2e22dab6f7bc70"
	goldenContractHash      = "a6ead4c0d083a8d580b623ef0fedf2414256f35e8bee2d0ecc7d4be32c46d254"
	nonemptyBrainDigest     = "76e08381209014163a6e094d53237cde3864f86b72ce83981f6e5565ee2622bb"
	nonemptyInventoryDigest = "a6196921459003539046ad406628ef9974e45b06d0fd24bd387895944696b917"
)

const goldenEligibleJSON = `{"format_version":1,"kind":"ELIGIBLE","plan_ref":"plan-01","snapshot_pin_id":"pin-01","reservation_version":1,"manifest_sha256":"0000000000000000000000000000000000000000000000000000000000000000","manifest_journal_watermark":{"generation":0,"sequence_id":0,"entry_hash":""},"source_build_token":"fixture-build","source_schema_version":2,"snapshot_account_inventory":[{"account_id":"acct0000000000001","status":"active"}],"snapshot_brain_inventory":{"count":1,"canonical_digest":"f1eb7d370f424a93bd89aef88ddf05822fdc1eded3042719339054086d210c3e"},"control_db_length":17,"verified_artifact_count":1,"declared_artifact_bytes":17,"snapshot_inventory_sha256":"04a8e01ab69e24a230882daabf374d67eb9bb2bb004c8b81bb2e22dab6f7bc70","plan_approval_ref":{"authority":"test","record_id":"case-01","version":"v1"},"plan_approval_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","plan_approval_nonce":"nonce-01","plan_approval_expires_at":"2026-10-03T12:00:00Z","activation_allowlist":["acct0000000000001"],"operation_id":"op-1","old_writer":{"provider":"cloud","scope_ref":"scope-01","writer_ref":"writer-01","boot_ref":"boot-01","journal_store_id":"store-01","generation":2},"journal_store_id":"store-01","snapshot_cut":{"active_generation":1,"last_object_in_generation":{"generation":0,"sequence_id":0,"entry_hash":""},"predecessor_seal":{"generation":0,"sequence_id":0,"entry_hash":""},"genesis_issuance_id":"genesis-01","successor_allocation_id":""},"last_sealed":{"generation":0,"sequence_id":0,"entry_hash":""},"prefix_evidence_ref":{"authority":"journal","record_id":"ancestry-01","version":"v1"},"eligible":{"legacy_plan_artifact_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","recovery_contract_plan_hash":"a6ead4c0d083a8d580b623ef0fedf2414256f35e8bee2d0ecc7d4be32c46d254","contract_plan":{"source_snapshot":"0000000000000000000000000000000000000000000000000000000000000000","journal_watermark":{"generation":0,"sequence_id":0,"entry_hash":""},"generation":2,"provider_truth_at":"2026-10-03T11:00:00Z","accounts":["acct0000000000001"]},"eligible_account_ids":["acct0000000000001"]}}`

const goldenFrozenJSON = `{"format_version":1,"kind":"FROZEN_ONLY","plan_ref":"plan-01","snapshot_pin_id":"pin-01","reservation_version":1,"manifest_sha256":"0000000000000000000000000000000000000000000000000000000000000000","manifest_journal_watermark":{"generation":0,"sequence_id":0,"entry_hash":""},"source_build_token":"fixture-build","source_schema_version":2,"snapshot_account_inventory":[{"account_id":"acct0000000000001","status":"active"}],"snapshot_brain_inventory":{"count":1,"canonical_digest":"f1eb7d370f424a93bd89aef88ddf05822fdc1eded3042719339054086d210c3e"},"control_db_length":17,"verified_artifact_count":1,"declared_artifact_bytes":17,"snapshot_inventory_sha256":"04a8e01ab69e24a230882daabf374d67eb9bb2bb004c8b81bb2e22dab6f7bc70","plan_approval_ref":{"authority":"test","record_id":"case-01","version":"v1"},"plan_approval_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","plan_approval_nonce":"nonce-01","plan_approval_expires_at":"2026-10-03T12:00:00Z","activation_allowlist":["acct0000000000001"],"operation_id":"op-1","old_writer":{"provider":"cloud","scope_ref":"scope-01","writer_ref":"writer-01","boot_ref":"boot-01","journal_store_id":"store-01","generation":2},"journal_store_id":"store-01","snapshot_cut":{"active_generation":1,"last_object_in_generation":{"generation":0,"sequence_id":0,"entry_hash":""},"predecessor_seal":{"generation":0,"sequence_id":0,"entry_hash":""},"genesis_issuance_id":"genesis-01","successor_allocation_id":""},"last_sealed":{"generation":0,"sequence_id":0,"entry_hash":""},"prefix_evidence_ref":{"authority":"journal","record_id":"ancestry-01","version":"v1"},"frozen":{"dispositions":[{"account_id":"acct0000000000001","disposition":"WITHHELD_FROZEN","reason_code":"PROVIDER_INELIGIBLE"}]}}`

const nonemptyBrainJSON = `{"version":1,"brains":[{"id":"brain-02","empty":false,"artifact":{"relative_path":"brain-02.bundle","length":3,"sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},"heads":[{"ref":"HEAD","object_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}]}`
const emptyBrainJSON = `{"version":1,"brains":[{"id":"brain-01","empty":true,"artifact":{"relative_path":"","length":0,"sha256":""},"heads":[]}]}`
const goldenInventoryJSON = `{"version":1,"manifest_sha256":"0000000000000000000000000000000000000000000000000000000000000000","source_build_token":"fixture-build","source_schema_version":2,"accounts":[{"account_id":"acct0000000000001","status":"active"}],"control_db_length":17,"brain_count":1,"brain_inventory_sha256":"f1eb7d370f424a93bd89aef88ddf05822fdc1eded3042719339054086d210c3e","verified_artifact_count":1,"declared_artifact_bytes":17}`

func TestRecoveryEnvelopeV1IndependentGoldenArms(t *testing.T) {
	tests := []struct {
		name     string
		value    RecoveryEnvelopeV1
		wantJSON string
		wantHash RecoveryEnvelopeHash
	}{
		{"eligible", goldenEnvelope(RecoveryEnvelopeEligible), goldenEligibleJSON, "da63cb2377107e78f0e7573b95d08a5766386f5124cd814fcfc3d7c41cceec0e"},
		{"frozen", goldenEnvelope(RecoveryEnvelopeFrozenOnly), goldenFrozenJSON, "cb8066f81b8def8f63e65485fe8662c40854993abcfd0803e0db6aa6bdba0ed7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, gotHash, err := EncodeRecoveryEnvelopeV1(context.Background(), tt.value)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if string(encoded) != tt.wantJSON {
				t.Fatalf("canonical bytes mismatch\n got: %s\nwant: %s", encoded, tt.wantJSON)
			}
			if gotHash != tt.wantHash {
				t.Fatalf("hash = %s, want %s", gotHash, tt.wantHash)
			}
			decoded, decodedHash, err := DecodeRecoveryEnvelopeV1(context.Background(), []byte(tt.wantJSON))
			if err != nil {
				t.Fatalf("decode golden: %v", err)
			}
			if decodedHash != tt.wantHash || decoded.Kind != tt.value.Kind {
				t.Fatalf("decoded result mismatch: kind=%s hash=%s", decoded.Kind, decodedHash)
			}
			reencoded, _, err := EncodeRecoveryEnvelopeV1(context.Background(), decoded)
			if err != nil || string(reencoded) != tt.wantJSON {
				t.Fatalf("round trip differs: err=%v", err)
			}
		})
	}
}

func TestRecoveryInventoryIndependentGoldenAndFullProjection(t *testing.T) {
	ctx := context.Background()
	brainDigest, err := BrainInventoryDigestV1(ctx, []contracts.BrainArtifact{{ID: "brain-01", Empty: true, Heads: nil}})
	if err != nil || brainDigest != goldenBrainDigest {
		t.Fatalf("brain digest = %q, err=%v", brainDigest, err)
	}
	emptyBytes, err := canonicalJSON(brainPayload([]contracts.BrainArtifact{{ID: "brain-01", Empty: true}}), maxSnapshotInventoryBytes)
	if err != nil || string(emptyBytes) != emptyBrainJSON {
		t.Fatalf("empty brain projection bytes = %q, err=%v", emptyBytes, err)
	}
	input := SnapshotInventoryV1{
		ManifestSHA256:        strings.Repeat("0", 64),
		SourceBuildToken:      "fixture-build",
		SourceSchemaVersion:   2,
		Accounts:              []SnapshotAccountV1{{ID: "acct0000000000001", Status: "active"}},
		Brains:                []contracts.BrainArtifact{{ID: "brain-01", Empty: true}},
		ControlDBLength:       17,
		VerifiedArtifactCount: 1,
		DeclaredArtifactBytes: 17,
	}
	got, err := SnapshotInventoryDigestV1(ctx, input)
	if err != nil || got != goldenInventoryDigest {
		t.Fatalf("inventory digest = %q, err=%v", got, err)
	}
	summary := SnapshotInventorySummaryV1{
		ManifestSHA256:        input.ManifestSHA256,
		SourceBuildToken:      input.SourceBuildToken,
		SourceSchemaVersion:   input.SourceSchemaVersion,
		Accounts:              input.Accounts,
		ControlDBLength:       input.ControlDBLength,
		BrainCount:            1,
		BrainInventorySHA256:  goldenBrainDigest,
		VerifiedArtifactCount: input.VerifiedArtifactCount,
		DeclaredArtifactBytes: input.DeclaredArtifactBytes,
	}
	inventoryBytes, err := canonicalJSON(inventoryPayload(summary, summary.BrainInventorySHA256, summary.BrainCount), maxSnapshotInventoryBytes)
	if err != nil || string(inventoryBytes) != goldenInventoryJSON {
		t.Fatalf("inventory projection bytes = %q, err=%v", inventoryBytes, err)
	}
	input.DeclaredArtifactBytes++
	if _, err := SnapshotInventoryDigestV1(ctx, input); !errors.Is(err, ErrRecoveryEnvelopeInvalid) {
		t.Fatalf("wrong total should be invalid, got %v", err)
	}
	brain := contracts.BrainArtifact{
		ID:          "brain-02",
		ArtifactRef: contracts.ArtifactRef{RelativePath: "brain-02.bundle", LengthBytes: 3, SHA256: strings.Repeat("c", 64)},
		Heads:       []contracts.BundleHead{{Ref: "HEAD", ObjectID: strings.Repeat("a", 40)}},
	}
	brainDigest, err = BrainInventoryDigestV1(ctx, []contracts.BrainArtifact{brain})
	if err != nil || brainDigest != nonemptyBrainDigest {
		t.Fatalf("nonempty brain digest = %q, err=%v", brainDigest, err)
	}
	brainBytes, err := canonicalJSON(brainPayload([]contracts.BrainArtifact{brain}), maxSnapshotInventoryBytes)
	if err != nil || string(brainBytes) != nonemptyBrainJSON {
		t.Fatalf("nonempty brain projection bytes = %q, err=%v", brainBytes, err)
	}
	input.Brains = []contracts.BrainArtifact{brain}
	input.VerifiedArtifactCount = 2
	input.DeclaredArtifactBytes = 20
	got, err = SnapshotInventoryDigestV1(ctx, input)
	if err != nil || got != nonemptyInventoryDigest {
		t.Fatalf("nonempty inventory digest = %q, err=%v", got, err)
	}
}

func TestRecoveryEnvelopeStrictDecoderRejectsNoncanonicalInput(t *testing.T) {
	valid := goldenEligibleJSON
	eligibleStart := strings.Index(valid, `,"eligible":{`)
	if eligibleStart < 0 {
		t.Fatal("eligible arm absent from golden")
	}
	nullArm := valid[:eligibleStart] + `,"eligible":null}`
	cases := []struct{ name, input string }{
		{"duplicate", strings.Replace(valid, `"format_version":1`, `"format_version":1,"format_version":1`, 1)},
		{"unknown", strings.Replace(valid, `"format_version":1`, `"extra":1,"format_version":1`, 1)},
		{"trailing value", valid + `{}`},
		{"whitespace", valid + " "},
		{"fraction", strings.Replace(valid, `"format_version":1`, `"format_version":1.0`, 1)},
		{"null required array", strings.Replace(valid, `"activation_allowlist":["acct0000000000001"]`, `"activation_allowlist":null`, 1)},
		{"null union arm", nullArm},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := DecodeRecoveryEnvelopeV1(context.Background(), []byte(tc.input)); err == nil {
				t.Fatal("expected strict decoder rejection")
			}
		})
	}
}

func TestRecoveryEnvelopeValidationSeparatesCutWriterAndSeal(t *testing.T) {
	base := goldenEnvelope(RecoveryEnvelopeEligible)
	if err := ValidateRecoveryEnvelopeV1(context.Background(), base); err != nil {
		t.Fatalf("valid historical cut rejected: %v", err)
	}
	badStore := cloneEnvelope(base)
	badStore.JournalStoreID = "other-store"
	if err := ValidateRecoveryEnvelopeV1(context.Background(), badStore); !errors.Is(err, ErrRecoveryEnvelopeInvalid) {
		t.Fatalf("mismatched store accepted: %v", err)
	}
	badGeneration := cloneEnvelope(base)
	badGeneration.Eligible.ContractPlan.Generation = 1
	if err := ValidateRecoveryEnvelopeV1(context.Background(), badGeneration); !errors.Is(err, ErrRecoveryEnvelopeInvalid) {
		t.Fatalf("inner plan generation mismatch accepted: %v", err)
	}
	badCut := cloneEnvelope(base)
	badCut.SnapshotCut.ActiveGeneration = 3
	badCut.SnapshotCut.GenesisIssuanceID = ""
	badCut.SnapshotCut.SuccessorAllocationID = "successor-03"
	badCut.SnapshotCut.PredecessorSeal = WatermarkV1{Generation: 2, SequenceID: 9_999_999_999, EntryHash: strings.Repeat("c", 64)}
	if err := ValidateRecoveryEnvelopeV1(context.Background(), badCut); !errors.Is(err, ErrRecoveryEnvelopeInvalid) {
		t.Fatalf("future cut accepted: %v", err)
	}
	badH := cloneEnvelope(base)
	badH.LastSealed = WatermarkV1{Generation: 3, SequenceID: 1, EntryHash: strings.Repeat("d", 64)}
	if err := ValidateRecoveryEnvelopeV1(context.Background(), badH); !errors.Is(err, ErrRecoveryEnvelopeInvalid) {
		t.Fatalf("future seal accepted: %v", err)
	}
}

func TestRecoveryEnvelopeInventoryDigestRecomputedFromSummary(t *testing.T) {
	value := goldenEnvelope(RecoveryEnvelopeFrozenOnly)
	value.SourceBuildToken = "different-build"
	if err := ValidateRecoveryEnvelopeV1(context.Background(), value); !errors.Is(err, ErrRecoveryEnvelopeInvalid) {
		t.Fatalf("inventory summary mutation accepted: %v", err)
	}
	value = goldenEnvelope(RecoveryEnvelopeFrozenOnly)
	value.SnapshotInventorySHA256 = strings.Repeat("e", 64)
	if err := ValidateRecoveryEnvelopeV1(context.Background(), value); !errors.Is(err, ErrRecoveryEnvelopeInvalid) {
		t.Fatalf("inventory digest mutation accepted: %v", err)
	}
}

func TestRecoveryEnvelopeRejectsContractHashMismatch(t *testing.T) {
	value := goldenEnvelope(RecoveryEnvelopeEligible)
	value.Eligible.RecoveryContractPlanHash = strings.Repeat("e", 64)
	if err := ValidateRecoveryEnvelopeV1(context.Background(), value); !errors.Is(err, ErrRecoveryEnvelopeInvalid) {
		t.Fatalf("mismatched contract hash accepted: %v", err)
	}
}

func TestRecoveryEnvelopeRejectsUnionArmMisuse(t *testing.T) {
	eligibleAndFrozen := goldenEnvelope(RecoveryEnvelopeEligible)
	eligibleAndFrozen.Frozen = &FrozenArmV1{Dispositions: []FrozenDispositionV1{}}
	if err := ValidateRecoveryEnvelopeV1(context.Background(), eligibleAndFrozen); !errors.Is(err, ErrRecoveryEnvelopeInvalid) {
		t.Fatalf("eligible plus frozen arms accepted: %v", err)
	}
	frozenAndEligible := goldenEnvelope(RecoveryEnvelopeFrozenOnly)
	frozenAndEligible.Eligible = &EligibleArmV1{}
	if err := ValidateRecoveryEnvelopeV1(context.Background(), frozenAndEligible); !errors.Is(err, ErrRecoveryEnvelopeInvalid) {
		t.Fatalf("frozen plus eligible arms accepted: %v", err)
	}
}

func TestRecoveryEnvelopeCloneOwnsNestedSlices(t *testing.T) {
	original := goldenEnvelope(RecoveryEnvelopeEligible)
	cloned, err := CloneRecoveryEnvelopeV1(original)
	if err != nil {
		t.Fatal(err)
	}
	cloned.SnapshotAccountInventory[0].Status = "deleted"
	cloned.ActivationAllowlist[0] = "bbbbbbbbbbbbbbbb"
	cloned.Eligible.EligibleAccountIDs[0] = "cccccccccccccccc"
	cloned.Eligible.ContractPlan.Accounts[0] = "dddddddddddddddd"
	if original.SnapshotAccountInventory[0].Status != "active" || original.ActivationAllowlist[0] != "acct0000000000001" || original.Eligible.EligibleAccountIDs[0] != "acct0000000000001" || original.Eligible.ContractPlan.Accounts[0] != "acct0000000000001" {
		t.Fatal("clone aliases original nested slices")
	}
}

func TestRecoveryEnvelopeContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ValidateRecoveryEnvelopeV1(ctx, goldenEnvelope(RecoveryEnvelopeFrozenOnly)); !errors.Is(err, ErrRecoveryEnvelopeContext) || !errors.Is(err, context.Canceled) {
		t.Fatalf("validate cancellation = %v", err)
	}
	if _, _, err := EncodeRecoveryEnvelopeV1(ctx, goldenEnvelope(RecoveryEnvelopeFrozenOnly)); !errors.Is(err, ErrRecoveryEnvelopeContext) {
		t.Fatalf("encode cancellation = %v", err)
	}
	if _, _, err := DecodeRecoveryEnvelopeV1(ctx, []byte(goldenFrozenJSON)); !errors.Is(err, ErrRecoveryEnvelopeContext) {
		t.Fatalf("decode cancellation = %v", err)
	}
}

func goldenEnvelope(kind RecoveryEnvelopeKind) RecoveryEnvelopeV1 {
	value := RecoveryEnvelopeV1{
		FormatVersion:            1,
		Kind:                     kind,
		PlanRef:                  "plan-01",
		SnapshotPinID:            "pin-01",
		ReservationVersion:       1,
		ManifestSHA256:           strings.Repeat("0", 64),
		SourceBuildToken:         "fixture-build",
		SourceSchemaVersion:      2,
		SnapshotAccountInventory: []SnapshotAccountV1{{ID: "acct0000000000001", Status: "active"}},
		SnapshotBrainInventory:   BrainInventoryV1{Count: 1, CanonicalDigest: goldenBrainDigest},
		ControlDBLength:          17,
		VerifiedArtifactCount:    1,
		DeclaredArtifactBytes:    17,
		SnapshotInventorySHA256:  goldenInventoryDigest,
		PlanApprovalRef:          EvidenceRefV1{Authority: "test", RecordID: "case-01", Version: "v1"},
		PlanApprovalDigest:       strings.Repeat("a", 64),
		PlanApprovalNonce:        "nonce-01",
		PlanApprovalExpiresAt:    time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC),
		ActivationAllowlist:      []string{"acct0000000000001"},
		OperationID:              "op-1",
		OldWriter:                WriterV1{Provider: "cloud", ScopeRef: "scope-01", WriterRef: "writer-01", BootRef: "boot-01", JournalStoreID: "store-01", Generation: 2},
		JournalStoreID:           "store-01",
		SnapshotCut:              JournalPositionV1{ActiveGeneration: 1, GenesisIssuanceID: "genesis-01"},
		PrefixEvidenceRef:        EvidenceRefV1{Authority: "journal", RecordID: "ancestry-01", Version: "v1"},
	}
	if kind == RecoveryEnvelopeEligible {
		value.Eligible = &EligibleArmV1{
			LegacyPlanArtifactHash:   strings.Repeat("b", 64),
			RecoveryContractPlanHash: goldenContractHash,
			ContractPlan: ContractPlanV1{
				SourceSnapshot:  strings.Repeat("0", 64),
				Generation:      2,
				ProviderTruthAt: time.Date(2026, 10, 3, 11, 0, 0, 0, time.UTC),
				Accounts:        []string{"acct0000000000001"},
			},
			EligibleAccountIDs: []string{"acct0000000000001"},
		}
	} else {
		value.Frozen = &FrozenArmV1{Dispositions: []FrozenDispositionV1{{AccountID: "acct0000000000001", Disposition: FrozenDispositionWithheld, ReasonCode: "PROVIDER_INELIGIBLE"}}}
	}
	return value
}

type cancelAfterChecksContext struct {
	context.Context
	checks   int
	cancelAt int
}

func (c *cancelAfterChecksContext) Err() error {
	c.checks++
	if c.checks >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

func TestRecoveryEnvelopePreflightBoundsBeforeTypedDecode(t *testing.T) {
	tooManyAllowlist := strings.Replace(goldenEligibleJSON,
		`"activation_allowlist":["acct0000000000001"]`,
		`"activation_allowlist":[`+strings.TrimSuffix(strings.Repeat(`"acct0000000000001",`, maxEnvelopeEligibleAccounts+1), ",")+`]`, 1)
	manyUnknown := `{"` + strings.Repeat("u", 65) + `":0}`
	manyUniqueUnknown := strings.Builder{}
	manyUniqueUnknown.WriteByte('{')
	for i := 0; i < 2000; i++ {
		if i > 0 {
			manyUniqueUnknown.WriteByte(',')
		}
		fmt.Fprintf(&manyUniqueUnknown, `"unknown_%04d":0`, i)
	}
	manyUniqueUnknown.WriteByte('}')
	tooLongField := strings.Replace(goldenEligibleJSON, `"plan_ref":"plan-01"`, `"plan_ref":"`+strings.Repeat("p", maxEnvelopeFieldBytes+1)+`"`, 1)
	tooLongAccountID := strings.Replace(goldenEligibleJSON, `acct0000000000001`, strings.Repeat("a", 65), 1)
	tooLongRef := strings.Replace(goldenEligibleJSON, `"plan_ref":"plan-01"`, `"plan_ref":"`+strings.Repeat("p", 257)+`"`, 1)
	missingCommonField := strings.Replace(goldenEligibleJSON, `,"source_schema_version":2`, ``, 1)
	wrongArrayType := strings.Replace(goldenEligibleJSON, `"activation_allowlist":["acct0000000000001"]`, `"activation_allowlist":true`, 1)
	nullRequiredArray := strings.Replace(goldenEligibleJSON, `"snapshot_account_inventory":[{"account_id":"acct0000000000001","status":"active"}]`, `"snapshot_account_inventory":null`, 1)
	eligibleStart := strings.Index(goldenEligibleJSON, `,"eligible":`)
	if eligibleStart < 0 {
		t.Fatal("eligible arm missing in golden")
	}
	nullUnionArm := goldenEligibleJSON[:eligibleStart] + `,"eligible":null}`
	missingUnionArm := goldenEligibleJSON[:eligibleStart] + `}`
	extraUnionArm := strings.TrimSuffix(goldenEligibleJSON, `}`) + `,"frozen":{"dispositions":[]}}`
	tooLargeInteger := strings.Replace(goldenEligibleJSON, `"reservation_version":1`, `"reservation_version":`+strings.Repeat("9", 40), 1)
	disposition := `{"account_id":"acct0000000000001","disposition":"WITHHELD_FROZEN","reason_code":"OTHER"}`
	tooManyDispositions := `{"frozen":{"dispositions":[` + strings.TrimSuffix(strings.Repeat(disposition+`,`, maxEnvelopeDispositions+1), `,`) + `]}}`
	for name, input := range map[string]string{
		"over-limit eligible array":       tooManyAllowlist,
		"over-limit full inventory array": tooManyDispositions,
		"oversized key":                   manyUnknown,
		"many unknown members":            manyUniqueUnknown.String(),
		"oversized string":                tooLongField,
		"oversized account ID":            tooLongAccountID,
		"oversized reference":             tooLongRef,
		"integer overflow":                tooLargeInteger,
		"missing required field":          missingCommonField,
		"wrong array type":                wrongArrayType,
		"null required array":             nullRequiredArray,
		"null union arm":                  nullUnionArm,
		"missing active union arm":        missingUnionArm,
		"extra inactive union arm":        extraUnionArm,
	} {
		t.Run(name, func(t *testing.T) {
			err := preflightEnvelopeJSON(context.Background(), []byte(input))
			if err == nil {
				t.Fatal("preflight accepted over-bound input")
			}
		})
	}
}

func TestRecoveryEnvelopePreflightChecksContextDuringScan(t *testing.T) {
	ctx := &cancelAfterChecksContext{Context: context.Background(), cancelAt: 8}
	err := preflightEnvelopeJSON(ctx, []byte(goldenEligibleJSON))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("preflight error = %v, want cancellation", err)
	}
	if ctx.checks < ctx.cancelAt {
		t.Fatalf("context was not checked throughout scan: checks=%d", ctx.checks)
	}
}
