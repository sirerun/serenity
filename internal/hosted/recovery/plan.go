// Package recovery stores immutable, locally approved recovery-plan artifacts.
// A plan hash binds the artifact's canonical bytes; it does not authenticate
// operator approval, provider truth, a journal seal, or writer adoption.
package recovery

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
	"sort"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

const (
	planFormatVersion = 1
	maxPlanFileBytes  = 1 << 20
	maxPlanAccounts   = 10_000
	maxAccountIDBytes = 256
)

var (
	ErrPlanInvalid      = errors.New("hosted/recovery: invalid recovery plan")
	ErrPlanUntrustedDir = errors.New("hosted/recovery: plan directory is not private and safe")
	ErrPlanExists       = errors.New("hosted/recovery: immutable plan already exists with different content")
)

// PlanInput contains the bounded metadata needed to persist a recovery plan.
// SnapshotSHA256 identifies the exact manifest-v2 source bytes; it is not a
// release build identity and is not inferred from one. Accounts are sorted by
// CreatePlan before hashing.
type PlanInput struct {
	SnapshotSHA256   string
	JournalWatermark contracts.DeletionWatermark
	FenceGeneration  int64
	ProviderObserved time.Time
	Accounts         []string
}

// Plan is an immutable plan artifact. The hash is SHA-256 over the canonical
// payload fields, excluding PlanHash itself.
type Plan struct {
	PlanHash         string                      `json:"plan_hash"`
	FormatVersion    int                         `json:"format_version"`
	SnapshotSHA256   string                      `json:"snapshot_sha256"`
	JournalWatermark contracts.DeletionWatermark `json:"journal_watermark"`
	FenceGeneration  int64                       `json:"fence_generation"`
	ProviderObserved string                      `json:"provider_observed_at"`
	Accounts         []string                    `json:"accounts"`
}

type planPayload struct {
	FormatVersion    int                         `json:"format_version"`
	SnapshotSHA256   string                      `json:"snapshot_sha256"`
	JournalWatermark contracts.DeletionWatermark `json:"journal_watermark"`
	FenceGeneration  int64                       `json:"fence_generation"`
	ProviderObserved string                      `json:"provider_observed_at"`
	Accounts         []string                    `json:"accounts"`
}

type planDocument struct {
	PlanHash string      `json:"plan_hash"`
	Payload  planPayload `json:"payload"`
}

// CreatePlan validates and atomically persists one immutable plan under dir.
// An identical existing plan is returned idempotently. The directory must
// already be an absolute, private, non-symlink operator-owned directory.
func CreatePlan(ctx context.Context, dir string, input PlanInput) (Plan, error) {
	if err := requireContext(ctx); err != nil {
		return Plan{}, err
	}
	var err error
	dir, err = privateDirectoryPath(dir)
	if err != nil {
		return Plan{}, err
	}
	plan, err := newPlan(input)
	if err != nil {
		return Plan{}, err
	}
	data, err := encodePlan(plan)
	if err != nil {
		return Plan{}, err
	}
	if len(data) > maxPlanFileBytes {
		return Plan{}, fmt.Errorf("%w: encoded plan exceeds size limit", ErrPlanInvalid)
	}
	if err = ctx.Err(); err != nil {
		return Plan{}, err
	}
	target := filepath.Join(dir, plan.PlanHash+".json")
	tmp, err := os.CreateTemp(dir, ".recovery-plan-*.tmp")
	if err != nil {
		return Plan{}, fmt.Errorf("hosted/recovery: create private plan temporary: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return Plan{}, fmt.Errorf("hosted/recovery: write private plan temporary: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return Plan{}, err
	}
	if err = linkPlanNoReplace(tmpName, target); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return Plan{}, fmt.Errorf("hosted/recovery: publish immutable plan: %w", err)
		}
		existing, loadErr := LoadPlan(ctx, dir, plan.PlanHash)
		if loadErr != nil || !plansEqual(existing, plan) {
			return Plan{}, errors.Join(ErrPlanExists, loadErr)
		}
		return existing, nil
	}
	if err = syncPlanDirectory(dir); err != nil {
		return Plan{}, fmt.Errorf("hosted/recovery: sync plan directory: %w", err)
	}
	return plan, nil
}

// LoadPlan reloads the exact hash-named artifact the operator approved. It
// rejects noncanonical encodings, unsafe files, stale hashes and extra JSON.
func LoadPlan(ctx context.Context, dir, expectedHash string) (Plan, error) {
	if err := requireContext(ctx); err != nil {
		return Plan{}, err
	}
	if !validSHA256(expectedHash) {
		return Plan{}, fmt.Errorf("%w: expected plan hash is malformed", ErrPlanInvalid)
	}
	var err error
	dir, err = privateDirectoryPath(dir)
	if err != nil {
		return Plan{}, err
	}
	path := filepath.Join(dir, expectedHash+".json")
	f, err := openPlanNoFollow(path)
	if err != nil {
		return Plan{}, fmt.Errorf("hosted/recovery: open immutable plan: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return Plan{}, fmt.Errorf("hosted/recovery: stat immutable plan: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > maxPlanFileBytes {
		return Plan{}, fmt.Errorf("%w: plan file type, permissions, or size is unsafe", ErrPlanInvalid)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxPlanFileBytes+1))
	if err != nil {
		return Plan{}, fmt.Errorf("hosted/recovery: read immutable plan: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return Plan{}, err
	}
	if len(data) > maxPlanFileBytes {
		return Plan{}, fmt.Errorf("%w: plan file exceeds size limit", ErrPlanInvalid)
	}
	doc, err := decodePlan(data)
	if err != nil {
		return Plan{}, err
	}
	plan := planFromDocument(doc)
	if err = validatePlan(plan); err != nil {
		return Plan{}, err
	}
	payload := payloadFromPlan(plan)
	canonicalPayload, err := json.Marshal(payload)
	if err != nil {
		return Plan{}, err
	}
	sum := sha256.Sum256(canonicalPayload)
	computed := hex.EncodeToString(sum[:])
	if plan.PlanHash != expectedHash || computed != expectedHash {
		return Plan{}, fmt.Errorf("%w: artifact hash does not match approved hash", ErrPlanInvalid)
	}
	canonicalDocument, err := encodePlan(plan)
	if err != nil {
		return Plan{}, err
	}
	if !bytes.Equal(data, canonicalDocument) {
		return Plan{}, fmt.Errorf("%w: artifact is not canonically encoded", ErrPlanInvalid)
	}
	return plan, nil
}

func newPlan(input PlanInput) (Plan, error) {
	accounts := append([]string(nil), input.Accounts...)
	sort.Strings(accounts)
	observed := input.ProviderObserved.UTC()
	plan := Plan{
		FormatVersion:    planFormatVersion,
		SnapshotSHA256:   input.SnapshotSHA256,
		JournalWatermark: input.JournalWatermark,
		FenceGeneration:  input.FenceGeneration,
		ProviderObserved: observed.Format(time.RFC3339Nano),
		Accounts:         accounts,
	}
	if err := validatePlanPayload(plan); err != nil {
		return Plan{}, err
	}
	canonical, err := json.Marshal(payloadFromPlan(plan))
	if err != nil {
		return Plan{}, err
	}
	sum := sha256.Sum256(canonical)
	plan.PlanHash = hex.EncodeToString(sum[:])
	return plan, nil
}

func validatePlan(plan Plan) error {
	if !validSHA256(plan.PlanHash) {
		return fmt.Errorf("%w: plan digest is malformed", ErrPlanInvalid)
	}
	return validatePlanPayload(plan)
}

func validatePlanPayload(plan Plan) error {
	if plan.FormatVersion != planFormatVersion || !validSHA256(plan.SnapshotSHA256) {
		return fmt.Errorf("%w: version or snapshot digest is malformed", ErrPlanInvalid)
	}
	observed, err := time.Parse(time.RFC3339Nano, plan.ProviderObserved)
	if err != nil || observed.IsZero() || observed.Location() != time.UTC || observed.Format(time.RFC3339Nano) != plan.ProviderObserved {
		return fmt.Errorf("%w: provider observation must be a canonical UTC timestamp", ErrPlanInvalid)
	}
	w := plan.JournalWatermark
	if !w.IsZero() && (w.Generation < 1 || w.SequenceID < 1 || !validSHA256(w.EntryHash)) {
		return fmt.Errorf("%w: journal watermark is malformed", ErrPlanInvalid)
	}
	if plan.FenceGeneration < 1 || plan.FenceGeneration < w.Generation {
		return fmt.Errorf("%w: fence generation precedes the journal watermark", ErrPlanInvalid)
	}
	if len(plan.Accounts) == 0 || len(plan.Accounts) > maxPlanAccounts {
		return fmt.Errorf("%w: account scope is empty or exceeds its bound", ErrPlanInvalid)
	}
	for i, account := range plan.Accounts {
		if account == "" || len(account) > maxAccountIDBytes || account == "*" || strings.EqualFold(account, "all") || strings.ContainsAny(account, "@ \t\r\n/") {
			return fmt.Errorf("%w: account scope contains an invalid opaque identifier", ErrPlanInvalid)
		}
		if i > 0 && plan.Accounts[i-1] >= account {
			return fmt.Errorf("%w: account scope is not sorted and unique", ErrPlanInvalid)
		}
	}
	return nil
}

func payloadFromPlan(plan Plan) planPayload {
	return planPayload{
		FormatVersion: plan.FormatVersion, SnapshotSHA256: plan.SnapshotSHA256,
		JournalWatermark: plan.JournalWatermark, FenceGeneration: plan.FenceGeneration,
		ProviderObserved: plan.ProviderObserved, Accounts: plan.Accounts,
	}
}

func documentFromPlan(plan Plan) planDocument {
	return planDocument{PlanHash: plan.PlanHash, Payload: payloadFromPlan(plan)}
}

func planFromDocument(doc planDocument) Plan {
	return Plan{
		PlanHash: doc.PlanHash, FormatVersion: doc.Payload.FormatVersion,
		SnapshotSHA256: doc.Payload.SnapshotSHA256, JournalWatermark: doc.Payload.JournalWatermark,
		FenceGeneration: doc.Payload.FenceGeneration, ProviderObserved: doc.Payload.ProviderObserved,
		Accounts: doc.Payload.Accounts,
	}
}

func encodePlan(plan Plan) ([]byte, error) {
	data, err := json.Marshal(documentFromPlan(plan))
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func decodePlan(data []byte) (planDocument, error) {
	if err := scanStrictJSON(data); err != nil {
		return planDocument{}, fmt.Errorf("%w: %v", ErrPlanInvalid, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var doc planDocument
	if err := dec.Decode(&doc); err != nil {
		return planDocument{}, fmt.Errorf("%w: decode artifact: %v", ErrPlanInvalid, err)
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return planDocument{}, fmt.Errorf("%w: trailing JSON data", ErrPlanInvalid)
	}
	return doc, nil
}

func scanStrictJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := scanValue(dec, "root"); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON data")
	}
	return nil
}

func scanValue(dec *json.Decoder, path string) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	switch token {
	case json.Delim('{'):
		allowed, ok := jsonObjectFields[path]
		if !ok {
			return fmt.Errorf("unexpected object at %s", path)
		}
		seen := make(map[string]struct{}, len(allowed))
		for dec.More() {
			keyToken, keyErr := dec.Token()
			if keyErr != nil {
				return keyErr
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("non-string key at %s", path)
			}
			if _, ok = allowed[key]; !ok {
				return fmt.Errorf("unknown or case-aliased key %q at %s", key, path)
			}
			if _, ok = seen[key]; ok {
				return fmt.Errorf("duplicate key %q at %s", key, path)
			}
			seen[key] = struct{}{}
			if err = scanValue(dec, childJSONPath(path, key)); err != nil {
				return err
			}
		}
		if _, err = dec.Token(); err != nil {
			return err
		}
		return nil
	case json.Delim('['):
		if path != "root.payload.accounts" {
			return fmt.Errorf("unexpected array at %s", path)
		}
		for dec.More() {
			if err = scanValue(dec, "root.payload.accounts[]"); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	case json.Delim(']'), json.Delim('}'):
		return fmt.Errorf("unexpected delimiter at %s", path)
	default:
		if path == "root.payload.accounts[]" {
			if _, ok := token.(string); !ok {
				return errors.New("account scope entries must be strings")
			}
		}
		return nil
	}
}

var jsonObjectFields = map[string]map[string]struct{}{
	"root": {"plan_hash": {}, "payload": {}},
	"root.payload": {
		"format_version": {}, "snapshot_sha256": {}, "journal_watermark": {},
		"fence_generation": {}, "provider_observed_at": {}, "accounts": {},
	},
	"root.payload.journal_watermark": {"generation": {}, "sequence_id": {}, "entry_hash": {}},
}

func childJSONPath(parent, key string) string {
	if parent == "root" {
		return "root." + key
	}
	if parent == "root.payload" && key == "journal_watermark" {
		return "root.payload.journal_watermark"
	}
	return parent + "." + key
}

func validSHA256(s string) bool {
	if len(s) != sha256.Size*2 || s != strings.ToLower(s) {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func plansEqual(a, b Plan) bool {
	left, leftErr := encodePlan(a)
	right, rightErr := encodePlan(b)
	return leftErr == nil && rightErr == nil && bytes.Equal(left, right)
}

func requireContext(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: context is required", ErrPlanInvalid)
	}
	return ctx.Err()
}
