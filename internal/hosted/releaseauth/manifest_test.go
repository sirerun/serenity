package releaseauth

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
)

var testNow = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

func encode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func fixture(t *testing.T) (Manifest, Policy, []byte, []byte, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	archive := []byte("synthetic release archive")
	digest := Digest(archive)
	m := Manifest{Version: 1, Repository: Repository, Tag: "v1.2.3", Commit: strings.Repeat("a", 40), Tree: strings.Repeat("b", 40), ArchiveName: "serenity_linux_arm64.tar.gz", ArchiveSize: int64(len(archive)), ArchiveSHA256: digest, OS: "linux", Arch: "arm64", GoToolchain: "go1.26.5", BuildRecipeSHA256: digest, ReviewReceiptSHA256: digest, TestReceiptSHA256: digest, Sequence: 1, IssuedAt: testNow.Add(-time.Hour).Format(time.RFC3339), ExpiresAt: testNow.Add(time.Hour).Format(time.RFC3339), PolicyID: PolicyID, KeyID: "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012", Algorithm: Algorithm}
	p := Policy{Version: 1, PolicyID: PolicyID, Repository: Repository, KeyID: m.KeyID, Algorithm: Algorithm, SPKI: base64.StdEncoding.EncodeToString(der), SPKISHA256: Digest(der), OS: "linux", Arch: "arm64"}
	return m, p, archive, signFixture(t, key, encode(t, m)), key
}
func signFixture(t *testing.T, key *ecdsa.PrivateKey, b []byte) []byte {
	t.Helper()
	digest := sha256.Sum256(b)
	s, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func approvalFixture(t *testing.T, m Manifest, s State) Approval {
	return Approval{Version: 1, ManifestSHA256: Digest(encode(t, m)), Tag: m.Tag, Commit: m.Commit, ArchiveSHA256: m.ArchiveSHA256, Sequence: m.Sequence, Mode: "promote", PreviousStateSHA256: Digest(encode(t, s)), IssuedAt: m.IssuedAt, ExpiresAt: m.ExpiresAt}
}
func genesis() State { return State{Version: 1, ManifestSHA256: strings.Repeat("0", 64)} }

func TestRealSignatureAndArchive(t *testing.T) {
	t.Parallel()
	m, p, archive, sig, _ := fixture(t)
	canonical := encode(t, m)
	if err := VerifySignature(m, canonical, sig, p, testNow); err != nil {
		t.Fatal(err)
	}
	if err := VerifyArchive(bytes.NewReader(archive), m); err != nil {
		t.Fatal(err)
	}
	b, err := CanonicalManifest(canonical, testNow)
	if err != nil || !bytes.Equal(b, canonical) {
		t.Fatalf("canonical authoring: %s %v", b, err)
	}
	for name, changed := range map[string][]byte{"changed": []byte("changed archive"), "long": append(archive, 'x'), "short": archive[:len(archive)-1]} {
		t.Run(name, func(t *testing.T) {
			if VerifyArchive(bytes.NewReader(changed), m) == nil {
				t.Fatal("accepted wrong archive")
			}
		})
	}
}
func TestStrictManifestEncoding(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := fixture(t)
	b := encode(t, m)
	cases := map[string][]byte{"newline": append(append([]byte{}, b...), '\n'), "trailing": append(append([]byte{}, b...), []byte("{}")...), "unknown": bytes.Replace(b, []byte(`"version":1`), []byte(`"unknown":0,"version":1`), 1), "duplicate": bytes.Replace(b, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1), "null": bytes.Replace(b, []byte(`"version":1`), []byte(`"version":null`), 1), "missing": bytes.Replace(b, []byte(`"version":1,`), nil, 1), "float": bytes.Replace(b, []byte(`"version":1`), []byte(`"version":1.0`), 1), "oversize": bytes.Repeat([]byte(" "), MaxDocument+1)}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			var m Manifest
			if DecodeCanonical(data, &m) == nil {
				t.Fatal("accepted noncanonical input")
			}
		})
	}
	for _, name := range []string{"trailing", "unknown", "duplicate", "null", "missing", "float", "oversize"} {
		if _, err := CanonicalManifest(cases[name], testNow); err == nil {
			t.Errorf("authoring accepted %s", name)
		}
	}
}
func TestSignatureNegatives(t *testing.T) {
	t.Parallel()
	m, p, _, sig, key := fixture(t)
	mutations := map[string]func(*Manifest){"repository": func(m *Manifest) { m.Repository = "evil/repo" }, "key": func(m *Manifest) { m.KeyID = strings.Replace(m.KeyID, "123456789012", "999999999999", 1) }, "algorithm": func(m *Manifest) { m.Algorithm = "RSA" }, "policy": func(m *Manifest) { m.PolicyID = "other" }, "tag": func(m *Manifest) { m.Tag = "invalid" }, "commit": func(m *Manifest) { m.Commit = "short" }, "tree": func(m *Manifest) { m.Tree = "short" }, "platform": func(m *Manifest) { m.OS = "darwin" }, "arch": func(m *Manifest) { m.Arch = "386" }, "future": func(m *Manifest) { m.IssuedAt = testNow.Add(time.Minute).Format(time.RFC3339) }, "expiry": func(m *Manifest) { m.ExpiresAt = testNow.Format(time.RFC3339) }, "long-window": func(m *Manifest) { m.ExpiresAt = testNow.Add(8 * 24 * time.Hour).Format(time.RFC3339) }, "fractional": func(m *Manifest) { m.IssuedAt = "2026-10-10T11:00:00.0Z" }, "archive-size": func(m *Manifest) { m.ArchiveSize = MaxArchive + 1 }, "path": func(m *Manifest) { m.ArchiveName = "../evil" }, "sequence": func(m *Manifest) { m.Sequence = 0 }}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := m
			mutate(&changed)
			b := encode(t, changed)
			signed := signFixture(t, key, b)
			if VerifySignature(changed, b, signed, p, testNow) == nil {
				t.Fatal("accepted invalid signed manifest")
			}
		})
	}
	t.Run("signature", func(t *testing.T) {
		bad := append([]byte{}, sig...)
		bad[len(bad)-1] ^= 1
		if VerifySignature(m, encode(t, m), bad, p, testNow) == nil {
			t.Fatal("accepted signature tampering")
		}
	})
	t.Run("valid-other-key", func(t *testing.T) {
		other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if VerifySignature(m, encode(t, m), signFixture(t, other, encode(t, m)), p, testNow) == nil {
			t.Fatal("accepted other key")
		}
	})
	t.Run("fingerprint", func(t *testing.T) {
		p.SPKISHA256 = strings.Repeat("0", 64)
		if VerifySignature(m, encode(t, m), sig, p, testNow) == nil {
			t.Fatal("accepted fingerprint mismatch")
		}
	})
	t.Run("curve", func(t *testing.T) {
		other, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		der, _ := x509.MarshalPKIXPublicKey(&other.PublicKey)
		p.SPKI = base64.StdEncoding.EncodeToString(der)
		p.SPKISHA256 = Digest(der)
		if _, err := p.PublicKey(); err == nil {
			t.Fatal("accepted P384")
		}
	})
}
func TestApprovalSequenceAndPinnedRollback(t *testing.T) {
	t.Parallel()
	m, _, _, _, _ := fixture(t)
	s := genesis()
	a := approvalFixture(t, m, s)
	next, err := NextState(m, encode(t, m), a, s, encode(t, s), m.Tag, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if next.HighestSequence != 1 {
		t.Fatal("high water")
	}
	for name, change := range map[string]func(*Approval){"manifest": func(a *Approval) { a.ManifestSHA256 = strings.Repeat("f", 64) }, "commit": func(a *Approval) { a.Commit = strings.Repeat("f", 40) }, "archive": func(a *Approval) { a.ArchiveSHA256 = strings.Repeat("f", 64) }, "tag": func(a *Approval) { a.Tag = "v1.0.0" }, "state": func(a *Approval) { a.PreviousStateSHA256 = strings.Repeat("f", 64) }, "mode": func(a *Approval) { a.Mode = "anything" }, "expiry": func(a *Approval) { a.ExpiresAt = testNow.Format(time.RFC3339) }} {
		t.Run(name, func(t *testing.T) {
			bad := a
			change(&bad)
			if _, err := NextState(m, encode(t, m), bad, s, encode(t, s), m.Tag, testNow); err == nil {
				t.Fatal("accepted bad approval")
			}
		})
	}
	a = approvalFixture(t, m, next)
	if _, err = NextState(m, encode(t, m), a, next, encode(t, next), m.Tag, testNow); err == nil {
		t.Fatal("accepted replay")
	}
	s = State{Version: 1, HighestSequence: 10, InstalledSequence: 8, ManifestSHA256: strings.Repeat("a", 64)}
	a = approvalFixture(t, m, s)
	a.Mode = "rollback"
	next, err = NextState(m, encode(t, m), a, s, encode(t, s), m.Tag, testNow)
	if err != nil || next.HighestSequence != 10 || next.InstalledSequence != 1 {
		t.Fatalf("rollback lost high water: %+v %v", next, err)
	}
	a = approvalFixture(t, m, next)
	a.Mode = "rollback"
	if _, err = NextState(m, encode(t, m), a, next, encode(t, next), m.Tag, testNow); err == nil {
		t.Fatal("same sequence rollback accepted")
	}
}

type fakeKMS struct {
	key   *ecdsa.PrivateKey
	keyID string
	bad   bool
}

func (f fakeKMS) Sign(_ context.Context, in *kms.SignInput, _ ...func(*kms.Options)) (*kms.SignOutput, error) {
	if in.MessageType != types.MessageTypeDigest || in.SigningAlgorithm != types.SigningAlgorithmSpecEcdsaSha256 || aws.ToString(in.KeyId) != f.keyID || len(in.Message) != 32 {
		return nil, fmt.Errorf("bad KMS request")
	}
	sig, err := ecdsa.SignASN1(rand.Reader, f.key, in.Message)
	id := f.keyID
	if f.bad {
		id = "other"
	}
	return &kms.SignOutput{KeyId: aws.String(id), Signature: sig, SigningAlgorithm: types.SigningAlgorithmSpecEcdsaSha256}, err
}
func TestKMSSignContract(t *testing.T) {
	t.Parallel()
	m, p, _, _, key := fixture(t)
	client := fakeKMS{key: key, keyID: p.KeyID}
	sig, err := Sign(context.Background(), client, encode(t, m), p, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if VerifySignature(m, encode(t, m), sig, p, testNow) != nil {
		t.Fatal("bad real signature")
	}
	client.bad = true
	if _, err = Sign(context.Background(), client, encode(t, m), p, testNow); err == nil {
		t.Fatal("accepted wrong KMS response")
	}
}
