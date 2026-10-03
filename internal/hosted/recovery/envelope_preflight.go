package recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// The preflight schema is deliberately independent of the typed wire structs:
// every container and scalar is bounded before encoding/json can allocate the
// typed envelope's slices or strings.
var envelopeObjectKeys = map[string]map[string]struct{}{
	"$":                                          keySet("format_version kind plan_ref snapshot_pin_id reservation_version manifest_sha256 manifest_journal_watermark source_build_token source_schema_version snapshot_account_inventory snapshot_brain_inventory control_db_length verified_artifact_count declared_artifact_bytes snapshot_inventory_sha256 plan_approval_ref plan_approval_digest plan_approval_nonce plan_approval_expires_at activation_allowlist operation_id old_writer journal_store_id snapshot_cut last_sealed prefix_evidence_ref eligible frozen"),
	"$.snapshot_account_inventory[]":             keySet("account_id status"),
	"$.snapshot_brain_inventory":                 keySet("count canonical_digest"),
	"$.plan_approval_ref":                        keySet("authority record_id version"),
	"$.prefix_evidence_ref":                      keySet("authority record_id version"),
	"$.old_writer":                               keySet("provider scope_ref writer_ref boot_ref journal_store_id generation"),
	"$.manifest_journal_watermark":               keySet("generation sequence_id entry_hash"),
	"$.snapshot_cut":                             keySet("active_generation last_object_in_generation predecessor_seal genesis_issuance_id successor_allocation_id"),
	"$.snapshot_cut.last_object_in_generation":   keySet("generation sequence_id entry_hash"),
	"$.snapshot_cut.predecessor_seal":            keySet("generation sequence_id entry_hash"),
	"$.last_sealed":                              keySet("generation sequence_id entry_hash"),
	"$.eligible":                                 keySet("legacy_plan_artifact_hash recovery_contract_plan_hash contract_plan eligible_account_ids"),
	"$.eligible.contract_plan":                   keySet("source_snapshot journal_watermark generation provider_truth_at accounts"),
	"$.eligible.contract_plan.journal_watermark": keySet("generation sequence_id entry_hash"),
	"$.frozen":                                   keySet("dispositions"),
	"$.frozen.dispositions[]":                    keySet("account_id disposition reason_code"),
}

var envelopeRequiredKeys = map[string]map[string]struct{}{
	"$":                                          keySet("format_version kind plan_ref snapshot_pin_id reservation_version manifest_sha256 manifest_journal_watermark source_build_token source_schema_version snapshot_account_inventory snapshot_brain_inventory control_db_length verified_artifact_count declared_artifact_bytes snapshot_inventory_sha256 plan_approval_ref plan_approval_digest plan_approval_nonce plan_approval_expires_at activation_allowlist operation_id old_writer journal_store_id snapshot_cut last_sealed prefix_evidence_ref"),
	"$.snapshot_account_inventory[]":             keySet("account_id status"),
	"$.snapshot_brain_inventory":                 keySet("count canonical_digest"),
	"$.plan_approval_ref":                        keySet("authority record_id version"),
	"$.prefix_evidence_ref":                      keySet("authority record_id version"),
	"$.old_writer":                               keySet("provider scope_ref writer_ref boot_ref journal_store_id generation"),
	"$.manifest_journal_watermark":               keySet("generation sequence_id entry_hash"),
	"$.snapshot_cut":                             keySet("active_generation last_object_in_generation predecessor_seal genesis_issuance_id successor_allocation_id"),
	"$.snapshot_cut.last_object_in_generation":   keySet("generation sequence_id entry_hash"),
	"$.snapshot_cut.predecessor_seal":            keySet("generation sequence_id entry_hash"),
	"$.last_sealed":                              keySet("generation sequence_id entry_hash"),
	"$.eligible":                                 keySet("legacy_plan_artifact_hash recovery_contract_plan_hash contract_plan eligible_account_ids"),
	"$.eligible.contract_plan":                   keySet("source_snapshot journal_watermark generation provider_truth_at accounts"),
	"$.eligible.contract_plan.journal_watermark": keySet("generation sequence_id entry_hash"),
	"$.frozen":                                   keySet("dispositions"),
	"$.frozen.dispositions[]":                    keySet("account_id disposition reason_code"),
}

var envelopeArrayCaps = map[string]int{
	"$.snapshot_account_inventory":      maxEnvelopeDispositions,
	"$.activation_allowlist":            maxEnvelopeEligibleAccounts,
	"$.eligible.eligible_account_ids":   maxEnvelopeEligibleAccounts,
	"$.eligible.contract_plan.accounts": maxEnvelopeEligibleAccounts,
	"$.frozen.dispositions":             maxEnvelopeDispositions,
}

var envelopeStringCaps = map[string]int{
	"$.kind":     16,
	"$.plan_ref": maxEnvelopeFieldBytes, "$.snapshot_pin_id": maxEnvelopeFieldBytes,
	"$.manifest_sha256": 64, "$.source_build_token": maxEnvelopeSourceTokenBytes,
	"$.snapshot_inventory_sha256": 64, "$.plan_approval_digest": 64,
	"$.plan_approval_nonce": maxEnvelopeFieldBytes, "$.plan_approval_expires_at": 35,
	"$.operation_id": maxEnvelopeFieldBytes, "$.journal_store_id": maxEnvelopeFieldBytes,
	"$.manifest_journal_watermark.entry_hash": 64, "$.last_sealed.entry_hash": 64,
	"$.snapshot_cut.last_object_in_generation.entry_hash": 64,
	"$.snapshot_cut.predecessor_seal.entry_hash":          64,
	"$.snapshot_cut.genesis_issuance_id":                  maxEnvelopeFieldBytes,
	"$.snapshot_cut.successor_allocation_id":              maxEnvelopeFieldBytes,
	"$.plan_approval_ref.authority":                       maxEnvelopeFieldBytes, "$.plan_approval_ref.record_id": maxEnvelopeFieldBytes,
	"$.plan_approval_ref.version":     maxEnvelopeFieldBytes,
	"$.prefix_evidence_ref.authority": maxEnvelopeFieldBytes, "$.prefix_evidence_ref.record_id": maxEnvelopeFieldBytes,
	"$.prefix_evidence_ref.version": maxEnvelopeFieldBytes,
	"$.old_writer.provider":         maxEnvelopeFieldBytes, "$.old_writer.scope_ref": maxEnvelopeFieldBytes,
	"$.old_writer.writer_ref": maxEnvelopeFieldBytes, "$.old_writer.boot_ref": maxEnvelopeFieldBytes,
	"$.old_writer.journal_store_id":               maxEnvelopeFieldBytes,
	"$.snapshot_brain_inventory.canonical_digest": 64,
	"$.snapshot_account_inventory[].account_id":   64,
	"$.snapshot_account_inventory[].status":       32,
	"$.activation_allowlist[]":                    64,
	"$.eligible.legacy_plan_artifact_hash":        64, "$.eligible.recovery_contract_plan_hash": 64,
	"$.eligible.contract_plan.source_snapshot":              64,
	"$.eligible.contract_plan.journal_watermark.entry_hash": 64,
	"$.eligible.contract_plan.provider_truth_at":            35,
	"$.eligible.contract_plan.accounts[]":                   64,
	"$.eligible.eligible_account_ids[]":                     64,
	"$.frozen.dispositions[].account_id":                    64,
	"$.frozen.dispositions[].disposition":                   32,
	"$.frozen.dispositions[].reason_code":                   64,
}

type envelopeIntegerRange struct{ min, max int64 }

var envelopeIntegerCaps = map[string]envelopeIntegerRange{
	"$.format_version":                                       {int64(envelopeFormatVersion), int64(envelopeFormatVersion)},
	"$.reservation_version":                                  {1, int64(^uint64(0) >> 1)},
	"$.source_schema_version":                                {1, int64(^uint32(0) >> 1)},
	"$.control_db_length":                                    {1, maxEnvelopeArtifactBytes},
	"$.verified_artifact_count":                              {1, maxEnvelopeArtifactCount},
	"$.declared_artifact_bytes":                              {1, maxEnvelopeArtifactBytes},
	"$.snapshot_brain_inventory.count":                       {0, int64(maxSnapshotBrains)},
	"$.old_writer.generation":                                {1, 10_000},
	"$.snapshot_cut.active_generation":                       {1, 10_000},
	"$.eligible.contract_plan.generation":                    {1, 10_000},
	"$.manifest_journal_watermark.generation":                {0, 10_000},
	"$.manifest_journal_watermark.sequence_id":               {0, 9_999_999_999},
	"$.last_sealed.generation":                               {0, 10_000},
	"$.last_sealed.sequence_id":                              {0, 9_999_999_999},
	"$.snapshot_cut.last_object_in_generation.generation":    {0, 10_000},
	"$.snapshot_cut.last_object_in_generation.sequence_id":   {0, 9_999_999_999},
	"$.snapshot_cut.predecessor_seal.generation":             {0, 10_000},
	"$.snapshot_cut.predecessor_seal.sequence_id":            {0, 9_999_999_999},
	"$.eligible.contract_plan.journal_watermark.generation":  {0, 10_000},
	"$.eligible.contract_plan.journal_watermark.sequence_id": {0, 9_999_999_999},
}

func keySet(keys string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, key := range strings.Fields(keys) {
		out[key] = struct{}{}
	}
	return out
}

func preflightEnvelopeJSON(ctx context.Context, encoded []byte) error {
	if err := ensureContext(ctx); err != nil {
		return err
	}
	if len(encoded) > maxEnvelopeBytes {
		return ErrRecoveryEnvelopeTooLarge
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := preflightContext(ctx); err != nil {
		return err
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if d, ok := token.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("top-level JSON value must be an object")
	}
	if err := preflightObject(ctx, decoder, "$", 1); err != nil {
		return err
	}
	if err := preflightContext(ctx); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func preflightObject(ctx context.Context, decoder *json.Decoder, path string, depth int) error {
	if depth > maxEnvelopeNesting {
		return ErrRecoveryEnvelopeTooLarge
	}
	allowed, ok := envelopeObjectKeys[path]
	if !ok {
		return fmt.Errorf("unexpected object at %s", path)
	}
	seen := make(map[string]struct{}, len(allowed))
	kind := ""
	for decoder.More() {
		if err := preflightContext(ctx); err != nil {
			return err
		}
		keyToken, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return fmt.Errorf("object key is not a string")
		}
		if len(key) > 64 {
			return ErrRecoveryEnvelopeTooLarge
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate JSON object key")
		}
		if _, known := allowed[key]; !known {
			return fmt.Errorf("unknown JSON field at %s", path)
		}
		seen[key] = struct{}{}
		if len(seen) > len(allowed) {
			return ErrRecoveryEnvelopeTooLarge
		}
		if path == "$" && key == "kind" {
			if err := preflightContext(ctx); err != nil {
				return err
			}
			value, err := decoder.Token()
			if err != nil {
				return err
			}
			text, ok := value.(string)
			if !ok || len(text) > envelopeStringCaps["$.kind"] {
				return fmt.Errorf("invalid kind field")
			}
			kind = text
			continue
		}
		if err := preflightValue(ctx, decoder, path+"."+key, depth+1); err != nil {
			return err
		}
	}
	if err := preflightContext(ctx); err != nil {
		return err
	}
	if path == "$" {
		_, hasEligible := seen["eligible"]
		_, hasFrozen := seen["frozen"]
		if kind == string(RecoveryEnvelopeEligible) && (!hasEligible || hasFrozen) ||
			kind == string(RecoveryEnvelopeFrozenOnly) && (!hasFrozen || hasEligible) ||
			kind != string(RecoveryEnvelopeEligible) && kind != string(RecoveryEnvelopeFrozenOnly) {
			return fmt.Errorf("invalid tagged union arm for envelope kind")
		}
	}
	if required := envelopeRequiredKeys[path]; required != nil {
		for requiredKey := range required {
			if _, present := seen[requiredKey]; !present {
				return fmt.Errorf("missing required JSON field %q at %s", requiredKey, path)
			}
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return fmt.Errorf("malformed JSON object")
	}
	return nil
}

func preflightValue(ctx context.Context, decoder *json.Decoder, path string, depth int) error {
	if depth > maxEnvelopeNesting {
		return ErrRecoveryEnvelopeTooLarge
	}
	if err := preflightContext(ctx); err != nil {
		return err
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			return preflightObject(ctx, decoder, path, depth)
		case '[':
			limit, ok := envelopeArrayCaps[path]
			if !ok {
				return fmt.Errorf("unexpected array at %s", path)
			}
			count := 0
			for decoder.More() {
				if err := preflightContext(ctx); err != nil {
					return err
				}
				if count >= limit {
					return ErrRecoveryEnvelopeTooLarge
				}
				count++
				if err := preflightValue(ctx, decoder, path+"[]", depth+1); err != nil {
					return err
				}
			}
			if err := preflightContext(ctx); err != nil {
				return err
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim(']') {
				return fmt.Errorf("malformed JSON array")
			}
		default:
			return fmt.Errorf("unexpected JSON delimiter")
		}
	case string:
		limit, ok := envelopeStringCaps[path]
		if !ok {
			return fmt.Errorf("unexpected string at %s", path)
		}
		if len(value) > limit {
			return ErrRecoveryEnvelopeTooLarge
		}
	case bool, nil:
		return fmt.Errorf("boolean/null is not valid for schema path %s", path)
	case json.Number:
		text := value.String()
		if !integerJSONPattern.MatchString(text) {
			return fmt.Errorf("non-integer JSON number")
		}
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return ErrRecoveryEnvelopeTooLarge
		}
		if _, err := strconv.Atoi(text); err != nil {
			return ErrRecoveryEnvelopeTooLarge
		}
		limit, ok := envelopeIntegerCaps[path]
		if !ok {
			return fmt.Errorf("unexpected integer at %s", path)
		}
		if n < limit.min || n > limit.max {
			return ErrRecoveryEnvelopeTooLarge
		}
	}
	return nil
}

func preflightContext(ctx context.Context) error { return ensureContext(ctx) }
