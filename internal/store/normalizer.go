package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// NormalizeKey is part of the spec (RFC §7.2): lowercase, collapse
// whitespace, canonical date and number forms. It is idempotent
// (property-tested) because claim-id derivation and semantic dedup both
// depend on two machines producing the same key for the same object.
func NormalizeKey(object string) string {
	k := strings.ToLower(strings.TrimSpace(object))
	k = wsRx.ReplaceAllString(k, " ")
	k = dateRx.ReplaceAllStringFunc(k, canonicalDate)
	if n, err := strconv.ParseFloat(k, 64); err == nil {
		// FormatFloat with -1 precision emits the minimal representation
		// that round-trips, so re-normalizing is a fixed point.
		k = strconv.FormatFloat(n, 'f', -1, 64)
	}
	return k
}

var (
	wsRx   = regexp.MustCompile(`\s+`)
	dateRx = regexp.MustCompile(`\b(\d{4})[-/](\d{1,2})[-/](\d{1,2})\b`)
)

func canonicalDate(d string) string {
	m := dateRx.FindStringSubmatch(d)
	mo, _ := strconv.Atoi(m[2])
	day, _ := strconv.Atoi(m[3])
	return fmt.Sprintf("%s-%02d-%02d", m[1], mo, day)
}

// DerivedID implements §7.2's claim-id rule:
// shorthash(subject_slug, predicate, normalized_object_key, valid_from,
// source_ref). Two machines extracting the same claim from the same source
// derive the same id; the same logical claim from different sources gets
// different ids by design (that is corroboration — dedup is semantic, at
// reconcile, never id equality). A detected collision (same id, different
// normalized content) is a hard error that widens the hash via migration —
// the reconciler (M2) owns that check.
func DerivedID(subject, predicate, objectKey, validFrom, sourceRef string) string {
	h := sha256.Sum256([]byte(strings.Join(
		[]string{subject, predicate, objectKey, validFrom, sourceRef}, "\x00")))
	return hex.EncodeToString(h[:4]) // 8 hex chars, sized for per-entity populations
}
