package recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"unicode/utf8"
)

var integerJSONPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

// EncodeRecoveryEnvelopeV1 returns canonical JSON bytes and their integrity
// digest. It never adds or implies authority.
func EncodeRecoveryEnvelopeV1(ctx context.Context, in RecoveryEnvelopeV1) ([]byte, RecoveryEnvelopeHash, error) {
	if err := ensureContext(ctx); err != nil {
		return nil, "", err
	}
	if err := ValidateRecoveryEnvelopeV1(ctx, in); err != nil {
		return nil, "", err
	}
	encoded, err := canonicalJSON(toEnvelopeWire(in), maxEnvelopeBytes)
	if err != nil {
		return nil, "", err
	}
	if err := ensureContext(ctx); err != nil {
		return nil, "", err
	}
	digest := RecoveryEnvelopeHash(domainHash(envelopeHashDomain, encoded))
	if err := ensureContext(ctx); err != nil {
		return nil, "", err
	}
	return bytes.Clone(encoded), digest, nil
}

// RecoveryEnvelopeHashV1 computes the canonical integrity digest without
// returning the serialized bytes.
func RecoveryEnvelopeHashV1(ctx context.Context, in RecoveryEnvelopeV1) (RecoveryEnvelopeHash, error) {
	if err := ensureContext(ctx); err != nil {
		return "", err
	}
	if err := ValidateRecoveryEnvelopeV1(ctx, in); err != nil {
		return "", err
	}
	encoded, err := canonicalJSON(toEnvelopeWire(in), maxEnvelopeBytes)
	if err != nil {
		return "", err
	}
	if err := ensureContext(ctx); err != nil {
		return "", err
	}
	digest := RecoveryEnvelopeHash(domainHash(envelopeHashDomain, encoded))
	if err := ensureContext(ctx); err != nil {
		return "", err
	}
	return digest, nil
}

// DecodeRecoveryEnvelopeV1 rejects noncanonical or malformed JSON, validates
// the inert envelope value, and returns its canonical integrity digest.
func DecodeRecoveryEnvelopeV1(ctx context.Context, encoded []byte) (RecoveryEnvelopeV1, RecoveryEnvelopeHash, error) {
	if err := ensureContext(ctx); err != nil {
		return RecoveryEnvelopeV1{}, "", err
	}
	if len(encoded) > maxEnvelopeBytes {
		return RecoveryEnvelopeV1{}, "", ErrRecoveryEnvelopeTooLarge
	}
	if !utf8.Valid(encoded) {
		return RecoveryEnvelopeV1{}, "", fmt.Errorf("%w: input is not valid UTF-8", ErrRecoveryEnvelopeNonCanonical)
	}
	if err := preflightEnvelopeJSON(ctx, encoded); err != nil {
		if errors.Is(err, ErrRecoveryEnvelopeTooLarge) {
			return RecoveryEnvelopeV1{}, "", err
		}
		return RecoveryEnvelopeV1{}, "", fmt.Errorf("%w: %v", ErrRecoveryEnvelopeNonCanonical, err)
	}
	if err := ensureContext(ctx); err != nil {
		return RecoveryEnvelopeV1{}, "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var wire envelopeV1Wire
	if err := decoder.Decode(&wire); err != nil {
		return RecoveryEnvelopeV1{}, "", fmt.Errorf("%w: decode envelope: %v", ErrRecoveryEnvelopeNonCanonical, err)
	}
	if err := ensureEOF(decoder); err != nil {
		return RecoveryEnvelopeV1{}, "", err
	}
	value, err := fromEnvelopeWire(wire)
	if err != nil {
		return RecoveryEnvelopeV1{}, "", err
	}
	if err := ValidateRecoveryEnvelopeV1(ctx, value); err != nil {
		return RecoveryEnvelopeV1{}, "", err
	}
	canonical, err := canonicalJSON(toEnvelopeWire(value), maxEnvelopeBytes)
	if err != nil {
		return RecoveryEnvelopeV1{}, "", err
	}
	if !bytes.Equal(canonical, encoded) {
		return RecoveryEnvelopeV1{}, "", ErrRecoveryEnvelopeNonCanonical
	}
	if err := ensureContext(ctx); err != nil {
		return RecoveryEnvelopeV1{}, "", err
	}
	digest := RecoveryEnvelopeHash(domainHash(envelopeHashDomain, canonical))
	if err := ensureContext(ctx); err != nil {
		return RecoveryEnvelopeV1{}, "", err
	}
	return cloneEnvelope(value), digest, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("%w: trailing JSON value", ErrRecoveryEnvelopeNonCanonical)
		}
		return fmt.Errorf("%w: trailing bytes: %v", ErrRecoveryEnvelopeNonCanonical, err)
	}
	return nil
}
