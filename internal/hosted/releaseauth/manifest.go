// Package releaseauth authenticates operator-built releases against independently
// installed policy and exact deployment approvals. It does not establish that
// the review or tests named by a signed manifest actually occurred.
package releaseauth

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"time"
)

const (
	PolicyID     = "owner-kms-manifest-v1"
	Algorithm    = "ECDSA_SHA_256"
	Repository   = "github.com/sirerun/serenity"
	MaxDocument  = 16 * 1024
	MaxArchive   = int64(2 * 1024 * 1024 * 1024)
	MaxSignature = 256
)

var (
	hex40       = regexp.MustCompile(`^[a-f0-9]{40}$`)
	hex64       = regexp.MustCompile(`^[a-f0-9]{64}$`)
	archiveName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$`)
	tagName     = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9.-]+)?$`)
	toolchain   = regexp.MustCompile(`^go1\.[0-9]+\.[0-9]+$`)
	keyARN      = regexp.MustCompile(`^arn:aws:kms:[a-z]{2}-[a-z]+-[0-9]:[0-9]{12}:key/[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
)

type Manifest struct {
	Version             int    `json:"version"`
	Repository          string `json:"repository"`
	Tag                 string `json:"tag"`
	Commit              string `json:"commit"`
	Tree                string `json:"tree"`
	ArchiveName         string `json:"archive_name"`
	ArchiveSize         int64  `json:"archive_size"`
	ArchiveSHA256       string `json:"archive_sha256"`
	OS                  string `json:"os"`
	Arch                string `json:"arch"`
	GoToolchain         string `json:"go_toolchain"`
	BuildRecipeSHA256   string `json:"build_recipe_sha256"`
	ReviewReceiptSHA256 string `json:"review_receipt_sha256"`
	TestReceiptSHA256   string `json:"test_receipt_sha256"`
	Sequence            uint64 `json:"sequence"`
	IssuedAt            string `json:"issued_at"`
	ExpiresAt           string `json:"expires_at"`
	PolicyID            string `json:"policy_id"`
	KeyID               string `json:"key_id"`
	Algorithm           string `json:"algorithm"`
}

type Policy struct {
	Version    int    `json:"version"`
	PolicyID   string `json:"policy_id"`
	Repository string `json:"repository"`
	KeyID      string `json:"key_id"`
	Algorithm  string `json:"algorithm"`
	SPKI       string `json:"spki"`
	SPKISHA256 string `json:"spki_sha256"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
}

type Approval struct {
	Version             int    `json:"version"`
	ManifestSHA256      string `json:"manifest_sha256"`
	Tag                 string `json:"tag"`
	Commit              string `json:"commit"`
	ArchiveSHA256       string `json:"archive_sha256"`
	Sequence            uint64 `json:"sequence"`
	Mode                string `json:"mode"`
	PreviousStateSHA256 string `json:"previous_state_sha256"`
	IssuedAt            string `json:"issued_at"`
	ExpiresAt           string `json:"expires_at"`
}

type State struct {
	Version           int    `json:"version"`
	HighestSequence   uint64 `json:"highest_sequence"`
	InstalledSequence uint64 `json:"installed_sequence"`
	ManifestSHA256    string `json:"manifest_sha256"`
}

type Pending struct {
	PreviousState       State  `json:"previous_state"`
	Version             int    `json:"version"`
	PreviousStateSHA256 string `json:"previous_state_sha256"`
	ManifestSHA256      string `json:"manifest_sha256"`
}

func Digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// DecodeCanonical rejects every alternate encoding, including duplicates,
// unknown fields, nulls, missing fields, trailing whitespace, and JSON values.
func DecodeCanonical(data []byte, out any) error {
	if len(data) == 0 || len(data) > MaxDocument {
		return fmt.Errorf("document outside size limit")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("decoding document: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing JSON data")
	}
	canonical, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("encoding document: %w", err)
	}
	if !bytes.Equal(canonical, data) {
		return fmt.Errorf("document is not exact canonical JSON")
	}
	return nil
}

// CanonicalManifest accepts a structured, noncanonical manifest only at the
// authoring boundary. Duplicate keys are rejected before canonicalization.
func CanonicalManifest(data []byte, now time.Time) ([]byte, error) {
	if len(data) == 0 || len(data) > MaxDocument {
		return nil, fmt.Errorf("manifest outside size limit")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("manifest must be an object")
	}
	seen := map[string]bool{}
	for d.More() {
		tok, e := d.Token()
		if e != nil {
			return nil, e
		}
		key, ok := tok.(string)
		if !ok || seen[key] {
			return nil, fmt.Errorf("duplicate manifest key")
		}
		seen[key] = true
		var raw json.RawMessage
		if e = d.Decode(&raw); e != nil {
			return nil, e
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, fmt.Errorf("null manifest field")
		}
	}
	if _, err = d.Token(); err != nil {
		return nil, err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, fmt.Errorf("trailing manifest data")
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&m); err != nil {
		return nil, err
	}
	if len(seen) != 20 {
		return nil, fmt.Errorf("manifest must contain every field")
	}
	if err = m.Validate(now); err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

func validWindow(issued, expires string, now time.Time) error {
	i, ei := time.Parse(time.RFC3339, issued)
	e, ee := time.Parse(time.RFC3339, expires)
	if ei != nil || ee != nil || i.UTC().Format("2006-01-02T15:04:05Z") != issued || e.UTC().Format("2006-01-02T15:04:05Z") != expires {
		return fmt.Errorf("timestamps must use canonical UTC seconds")
	}
	if i.After(now) || !e.After(now) || !e.After(i) || e.Sub(i) > 7*24*time.Hour {
		return fmt.Errorf("invalid or expired validity window")
	}
	return nil
}

func (m Manifest) Validate(now time.Time) error {
	if m.Version != 1 || m.Repository != Repository || m.PolicyID != PolicyID || m.Algorithm != Algorithm || !keyARN.MatchString(m.KeyID) {
		return fmt.Errorf("unsupported manifest identity")
	}
	if len(m.Tag) > 100 || !tagName.MatchString(m.Tag) || !hex40.MatchString(m.Commit) || !hex40.MatchString(m.Tree) {
		return fmt.Errorf("invalid source identity")
	}
	if !archiveName.MatchString(m.ArchiveName) || m.ArchiveSize <= 0 || m.ArchiveSize > MaxArchive || !hex64.MatchString(m.ArchiveSHA256) {
		return fmt.Errorf("invalid archive identity")
	}
	if m.OS != "linux" || (m.Arch != "arm64" && m.Arch != "amd64") || len(m.GoToolchain) > 32 || !toolchain.MatchString(m.GoToolchain) {
		return fmt.Errorf("unsupported build platform")
	}
	if !hex64.MatchString(m.BuildRecipeSHA256) || !hex64.MatchString(m.ReviewReceiptSHA256) || !hex64.MatchString(m.TestReceiptSHA256) || m.Sequence == 0 {
		return fmt.Errorf("invalid release receipts or sequence")
	}
	return validWindow(m.IssuedAt, m.ExpiresAt, now)
}

func (p Policy) PublicKey() (*ecdsa.PublicKey, error) {
	if p.Version != 1 || p.PolicyID != PolicyID || p.Repository != Repository || p.Algorithm != Algorithm || !keyARN.MatchString(p.KeyID) || p.OS != "linux" || (p.Arch != "arm64" && p.Arch != "amd64") || !hex64.MatchString(p.SPKISHA256) {
		return nil, fmt.Errorf("invalid release policy")
	}
	der, err := base64.StdEncoding.DecodeString(p.SPKI)
	if err != nil || base64.StdEncoding.EncodeToString(der) != p.SPKI || Digest(der) != p.SPKISHA256 {
		return nil, fmt.Errorf("SPKI fingerprint mismatch")
	}
	pub, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("parsing SPKI: %w", err)
	}
	key, ok := pub.(*ecdsa.PublicKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, fmt.Errorf("policy requires P256 public key")
	}
	return key, nil
}

func VerifySignature(m Manifest, canonical, signature []byte, p Policy, now time.Time) error {
	if err := m.Validate(now); err != nil {
		return err
	}
	var decoded Manifest
	if err := DecodeCanonical(canonical, &decoded); err != nil {
		return err
	}
	if decoded != m {
		return fmt.Errorf("manifest bytes mismatch")
	}
	if m.PolicyID != p.PolicyID || m.Repository != p.Repository || m.KeyID != p.KeyID || m.Algorithm != p.Algorithm || m.OS != p.OS || m.Arch != p.Arch {
		return fmt.Errorf("manifest does not match trust policy")
	}
	key, err := p.PublicKey()
	if err != nil {
		return err
	}
	if len(signature) == 0 || len(signature) > MaxSignature {
		return fmt.Errorf("signature outside size limit")
	}
	digest := sha256.Sum256(canonical)
	if !ecdsa.VerifyASN1(key, digest[:], signature) {
		return fmt.Errorf("invalid manifest signature")
	}
	return nil
}

func VerifyArchive(r io.Reader, m Manifest) error {
	if r == nil || m.ArchiveSize <= 0 || m.ArchiveSize > MaxArchive || !hex64.MatchString(m.ArchiveSHA256) {
		return fmt.Errorf("invalid archive bounds")
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(r, m.ArchiveSize+1))
	if err != nil {
		return fmt.Errorf("hashing archive: %w", err)
	}
	if n != m.ArchiveSize || hex.EncodeToString(h.Sum(nil)) != m.ArchiveSHA256 {
		return fmt.Errorf("archive size or digest mismatch")
	}
	return nil
}

func (s State) Validate() error {
	if s.Version != 1 || !hex64.MatchString(s.ManifestSHA256) || s.InstalledSequence > s.HighestSequence {
		return fmt.Errorf("invalid deployment state")
	}
	zero := string(bytes.Repeat([]byte("0"), 64))
	if (s.HighestSequence == 0 && (s.InstalledSequence != 0 || s.ManifestSHA256 != zero)) || (s.HighestSequence > 0 && (s.InstalledSequence == 0 || s.ManifestSHA256 == zero)) {
		return fmt.Errorf("invalid genesis state")
	}
	return nil
}

func NextState(m Manifest, canonical []byte, a Approval, s State, stateBytes []byte, expectedTag string, now time.Time) (State, error) {
	if err := m.Validate(now); err != nil {
		return State{}, err
	}
	var decodedManifest Manifest
	if err := DecodeCanonical(canonical, &decodedManifest); err != nil {
		return State{}, err
	}
	if decodedManifest != m {
		return State{}, fmt.Errorf("manifest bytes mismatch")
	}
	if err := s.Validate(); err != nil {
		return State{}, err
	}
	var decoded State
	if err := DecodeCanonical(stateBytes, &decoded); err != nil {
		return State{}, err
	}
	if decoded != s {
		return State{}, fmt.Errorf("state bytes mismatch")
	}
	if a.Version != 1 || a.ManifestSHA256 != Digest(canonical) || a.Tag != m.Tag || a.Commit != m.Commit || a.ArchiveSHA256 != m.ArchiveSHA256 || a.Sequence != m.Sequence || a.PreviousStateSHA256 != Digest(stateBytes) || expectedTag != m.Tag {
		return State{}, fmt.Errorf("release does not match exact approval or previous state")
	}
	if err := validWindow(a.IssuedAt, a.ExpiresAt, now); err != nil {
		return State{}, err
	}
	switch a.Mode {
	case "promote":
		if m.Sequence <= s.HighestSequence {
			return State{}, fmt.Errorf("release sequence does not advance high water")
		}
	case "rollback":
		if m.Sequence >= s.InstalledSequence || m.Sequence > s.HighestSequence {
			return State{}, fmt.Errorf("rollback is not an exact earlier release")
		}
	default:
		return State{}, fmt.Errorf("invalid approval mode")
	}
	next := State{Version: 1, HighestSequence: s.HighestSequence, InstalledSequence: m.Sequence, ManifestSHA256: a.ManifestSHA256}
	if m.Sequence > next.HighestSequence {
		next.HighestSequence = m.Sequence
	}
	return next, nil
}
