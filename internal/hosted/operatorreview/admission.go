// Package operatorreview verifies immutable, signed operator-review cases.
// It has no signing or persistence capability.
package operatorreview

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sirerun/serenity/internal/hosted/admintransport"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/privatefs"
	"github.com/sirerun/serenity/internal/hosted/service"
)

const (
	caseMaxBytes    = 8192
	maxJSONDepth    = 16
	signatureDomain = "serenity.operator-review.v1\x00"
)

var errDenied = errors.New("operator review admission denied")

// TrustedKey is one row in the current trusted-key snapshot returned by the
// injected source. The source must authenticate and refresh its current view
// for every Lookup; this package does not cache it.
type TrustedKey struct {
	PublicKey  []byte
	OperatorID string
	NotBefore  time.Time
	NotAfter   time.Time
	Revoked    bool
}

// TrustedKeySource must return the current key state on every call. Implementations
// must authenticate the trust root, reject duplicate mappings, and avoid caches
// that can outlive a revocation.
type TrustedKeySource interface {
	Lookup(ctx context.Context, keyID string) (TrustedKey, error)
}

// Policy has no implicit values. The current-key Lookup snapshot is the
// admission linearization point. A revocation after that snapshot does not
// cancel that admitted request, while every later Authorize consults the
// source again.
type Policy struct {
	ExpectedPeerUID uint32
	MaxCaseAge      time.Duration
	MaxCaseLifetime time.Duration
	Now             func() time.Time
}

// Admission implements service.OperatorReviewAdmission using immutable case
// files and an explicitly injected current trust source.
type Admission struct {
	caseDirectory string
	keys          TrustedKeySource
	policy        Policy
}

func New(ctx context.Context, caseDirectory string, keys TrustedKeySource, policy Policy) (*Admission, error) {
	if ctx == nil {
		return nil, errors.New("operator review: context required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if isNil(keys) {
		return nil, errors.New("operator review: explicit trust source and policy required")
	}
	if policy.MaxCaseAge <= 0 || policy.MaxCaseLifetime <= 0 || policy.Now == nil {
		return nil, errors.New("operator review: explicit positive time policy required")
	}
	if policy.ExpectedPeerUID != uint32(os.Geteuid()) {
		return nil, errors.New("operator review: peer UID must match service effective UID")
	}
	if err := privatefs.ValidateDirectory(ctx, caseDirectory); err != nil {
		return nil, fmt.Errorf("operator review: invalid case directory: %w", err)
	}
	return &Admission{caseDirectory: caseDirectory, keys: keys, policy: policy}, nil
}

// Authorize verifies transport identity before reading either the case or the
// injected trust source. It only admits positive committed-fact proofs.
func (a *Admission) Authorize(ctx context.Context, request *http.Request, operationID, reviewRef string) (service.ApprovedOperatorReview, error) {
	if a == nil || ctx == nil || request == nil || request.Context() == nil || isNil(a.keys) || ctx.Err() != nil || request.Context().Err() != nil {
		return service.ApprovedOperatorReview{}, errDenied
	}
	uid, ok := admintransport.AuthenticatedUID(ctx)
	requestUID, requestOK := admintransport.AuthenticatedUID(request.Context())
	if !ok || !requestOK || uid != a.policy.ExpectedPeerUID || requestUID != uid {
		return service.ApprovedOperatorReview{}, errDenied
	}
	if !validOpaque(operationID) || !validReviewRef(reviewRef) {
		return service.ApprovedOperatorReview{}, errDenied
	}
	digest := strings.TrimPrefix(reviewRef, "sha256:")
	caseBytes, err := privatefs.ReadFile(ctx, a.caseDirectory, digest+".json", caseMaxBytes)
	if err != nil {
		return service.ApprovedOperatorReview{}, errDenied
	}
	if err = ctx.Err(); err != nil {
		return service.ApprovedOperatorReview{}, errDenied
	}
	caseHash := sha256.Sum256(caseBytes)
	if "sha256:"+hex.EncodeToString(caseHash[:]) != reviewRef {
		return service.ApprovedOperatorReview{}, errDenied
	}
	envelope, err := parseEnvelope(caseBytes)
	if err != nil || envelope.OperationID != operationID || envelope.Outcome != "committed" || !validOpaque(envelope.OperatorID) || !validKeyID(envelope.KeyID) || !validFactRef(envelope.ExpectedCanonicalRef) {
		return service.ApprovedOperatorReview{}, errDenied
	}
	approvedAt, err := parseCanonicalTime(envelope.ApprovedAt)
	if err != nil {
		return service.ApprovedOperatorReview{}, errDenied
	}
	expiresAt, err := parseCanonicalTime(envelope.ExpiresAt)
	if err != nil {
		return service.ApprovedOperatorReview{}, errDenied
	}
	signature, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil || base64.StdEncoding.EncodeToString(signature) != envelope.Signature || len(signature) != ed25519.SignatureSize {
		return service.ApprovedOperatorReview{}, errDenied
	}
	if ctx.Err() != nil {
		return service.ApprovedOperatorReview{}, errDenied
	}
	key, err := a.keys.Lookup(ctx, envelope.KeyID)
	if err != nil || ctx.Err() != nil || len(key.PublicKey) != ed25519.PublicKeySize || key.Revoked || key.OperatorID != envelope.OperatorID || !validOpaque(key.OperatorID) {
		return service.ApprovedOperatorReview{}, errDenied
	}
	keyID := sha256.Sum256(key.PublicKey)
	if envelope.KeyID != "ed25519-sha256:"+hex.EncodeToString(keyID[:]) {
		return service.ApprovedOperatorReview{}, errDenied
	}
	now := a.policy.Now()
	if now.IsZero() || approvedAt.IsZero() || expiresAt.IsZero() {
		return service.ApprovedOperatorReview{}, errDenied
	}
	now = now.UTC()
	if key.NotBefore.IsZero() || key.NotAfter.IsZero() || now.Before(key.NotBefore) || !now.Before(key.NotAfter) ||
		approvedAt.Before(key.NotBefore) || approvedAt.After(now) || !expiresAt.After(approvedAt) || !now.Before(expiresAt) ||
		approvedAt.Before(now.Add(-a.policy.MaxCaseAge)) || expiresAt.After(approvedAt.Add(a.policy.MaxCaseLifetime)) {
		return service.ApprovedOperatorReview{}, errDenied
	}
	payload, err := canonicalPayload(envelope)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(key.PublicKey), append([]byte(signatureDomain), payload...), signature) || ctx.Err() != nil || request.Context().Err() != nil {
		return service.ApprovedOperatorReview{}, errDenied
	}
	return service.ApprovedOperatorReview{
		OperationID: envelope.OperationID, ReviewRef: reviewRef, OperatorID: envelope.OperatorID,
		Outcome: contracts.OperationCommitted, ExpectedCanonicalRef: envelope.ExpectedCanonicalRef, ApprovedAt: approvedAt,
	}, nil
}

type signedEnvelope struct {
	Version              int    `json:"version"`
	KeyID                string `json:"key_id"`
	OperatorID           string `json:"operator_id"`
	OperationID          string `json:"operation_id"`
	Outcome              string `json:"outcome"`
	ExpectedCanonicalRef string `json:"expected_canonical_ref"`
	ApprovedAt           string `json:"approved_at"`
	ExpiresAt            string `json:"expires_at"`
	Signature            string `json:"signature"`
}

type signedPayload struct {
	Version              int    `json:"version"`
	KeyID                string `json:"key_id"`
	OperatorID           string `json:"operator_id"`
	OperationID          string `json:"operation_id"`
	Outcome              string `json:"outcome"`
	ExpectedCanonicalRef string `json:"expected_canonical_ref"`
	ApprovedAt           string `json:"approved_at"`
	ExpiresAt            string `json:"expires_at"`
}

func parseEnvelope(data []byte) (signedEnvelope, error) {
	var envelope signedEnvelope
	if len(data) == 0 || len(data) > caseMaxBytes || !utf8.Valid(data) || validateJSON(data) != nil {
		return envelope, errDenied
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return envelope, errDenied
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return envelope, errDenied
	}
	canonical, err := json.Marshal(envelope)
	if err != nil || string(canonical) != string(data) || envelope.Version != 1 {
		return envelope, errDenied
	}
	return envelope, nil
}

func canonicalPayload(envelope signedEnvelope) ([]byte, error) {
	return json.Marshal(signedPayload{
		Version: envelope.Version, KeyID: envelope.KeyID, OperatorID: envelope.OperatorID,
		OperationID: envelope.OperationID, Outcome: envelope.Outcome,
		ExpectedCanonicalRef: envelope.ExpectedCanonicalRef, ApprovedAt: envelope.ApprovedAt,
		ExpiresAt: envelope.ExpiresAt,
	})
}

func validateJSON(data []byte) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := scanValue(decoder, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("operator review: trailing JSON")
	}
	return nil
}

func scanValue(decoder *json.Decoder, depth int) error {
	if depth > maxJSONDepth {
		return errors.New("operator review: JSON nesting exceeded")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return errors.New("operator review: invalid JSON object key")
			}
			if _, exists := seen[name]; exists {
				return errors.New("operator review: duplicate JSON key")
			}
			seen[name] = struct{}{}
			if err := scanValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("operator review: unterminated JSON object")
		}
	case '[':
		for decoder.More() {
			if err := scanValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("operator review: unterminated JSON array")
		}
	default:
		return errors.New("operator review: unexpected JSON delimiter")
	}
	return nil
}

func parseCanonicalTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Location() != time.UTC || parsed.Format(time.RFC3339Nano) != value {
		return time.Time{}, errDenied
	}
	return parsed, nil
}

func validReviewRef(value string) bool {
	const prefix = "sha256:"
	return len(value) == len(prefix)+64 && strings.HasPrefix(value, prefix) && isLowerHex(value[len(prefix):])
}

func validKeyID(value string) bool {
	const prefix = "ed25519-sha256:"
	return len(value) == len(prefix)+64 && strings.HasPrefix(value, prefix) && isLowerHex(value[len(prefix):])
}

func validFactRef(value string) bool {
	return len(value) == len("fact:")+64 && strings.HasPrefix(value, "fact:") && isLowerHex(value[len("fact:"):])
}

func validOpaque(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for _, c := range []byte(value) {
		if c <= 0x20 || c >= 0x7f {
			return false
		}
	}
	return true
}

func isLowerHex(value string) bool {
	if value == "" {
		return false
	}
	for _, c := range []byte(value) {
		if c >= '0' && c <= '9' {
			continue
		}
		if c < 'a' || c > 'f' {
			return false
		}
	}
	return true
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
