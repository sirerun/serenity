//go:build darwin || linux

package operatorreview

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/admintransport"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/service"
)

const testUnix = "2026-10-01T17:00:00Z"

var fixedSeed = [ed25519.SeedSize]byte{
	0x13, 0x22, 0x31, 0x40, 0x55, 0x64, 0x73, 0x82,
	0x91, 0xa0, 0xb5, 0xc4, 0xd3, 0xe2, 0xf1, 0x0f,
	0x12, 0x24, 0x36, 0x48, 0x5a, 0x6c, 0x7e, 0x80,
	0x92, 0xa4, 0xb6, 0xc8, 0xda, 0xec, 0xfe, 0x01,
}

type testKeys struct {
	key   TrustedKey
	err   error
	calls atomic.Int32
}

type barrierKeys struct {
	mu          sync.Mutex
	key         TrustedKey
	snapshotted chan struct{}
	resume      chan struct{}
	calls       atomic.Int32
}

func (s *barrierKeys) Lookup(ctx context.Context, _ string) (TrustedKey, error) {
	call := s.calls.Add(1)
	s.mu.Lock()
	key := s.key
	key.PublicKey = append([]byte(nil), key.PublicKey...)
	s.mu.Unlock()
	if call == 1 {
		close(s.snapshotted)
		select {
		case <-ctx.Done():
			return TrustedKey{}, ctx.Err()
		case <-s.resume:
		}
	}
	return key, nil
}

func (s *testKeys) Lookup(ctx context.Context, _ string) (TrustedKey, error) {
	s.calls.Add(1)
	if err := ctx.Err(); err != nil {
		return TrustedKey{}, err
	}
	return s.key, s.err
}

func TestAdmissionUsesRealUnixPeerAndReturnsExactProof(t *testing.T) {
	privateDir := makePrivateDir(t)
	privateKey := ed25519.NewKeyFromSeed(fixedSeed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keyID := publicKeyID(publicKey)
	keys := &testKeys{key: validTrustedKey(publicKey, "operator-7")}
	admission, err := New(context.Background(), privateDir, keys, validPolicy())
	if err != nil {
		t.Fatal(err)
	}
	ref := writeCase(t, privateDir, privateKey, keyID, "operator-7", "operation-17", "committed", "fact:"+strings.Repeat("a", 64), testUnix, "2026-10-01T17:20:00Z")

	socketDir := makePrivateDir(t)
	socketPath := filepath.Join(socketDir, "admin.sock")
	listener, err := admintransport.Listen(context.Background(), socketPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	server, err := listener.HTTPServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		approved, authorizeErr := admission.Authorize(r.Context(), r, "operation-17", ref)
		if authorizeErr != nil {
			http.Error(w, "denied", http.StatusForbidden)
			return
		}
		if approved.OperationID != "operation-17" || approved.ReviewRef != ref || approved.OperatorID != "operator-7" ||
			approved.Outcome != contracts.OperationCommitted || approved.ExpectedCanonicalRef != "fact:"+strings.Repeat("a", 64) || approved.ApprovedAt.Format(time.RFC3339Nano) != testUnix {
			http.Error(w, "incorrect admission", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	if err != nil {
		t.Fatal(err)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Close()
		if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("Serve: %v", err)
		}
	})
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	response, err := client.Get("http://admin/review")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNoContent || keys.calls.Load() != 1 {
		t.Fatalf("status=%d trust lookups=%d", response.StatusCode, keys.calls.Load())
	}
}

func TestUnauthenticatedAndForgedRequestsDoNotReadCaseOrTrust(t *testing.T) {
	dir := makePrivateDir(t)
	privateKey := ed25519.NewKeyFromSeed(fixedSeed[:])
	keys := &testKeys{key: validTrustedKey(privateKey.Public().(ed25519.PublicKey), "operator-7")}
	admission, err := New(context.Background(), dir, keys, validPolicy())
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/admin/review", nil)
	request.Header.Set("X-Operator-UID", "0")
	request.Header.Set("Authorization", "Bearer forged")
	if _, err := admission.Authorize(request.Context(), request, "operation-17", "sha256:"+strings.Repeat("0", 64)); !errors.Is(err, errDenied) {
		t.Fatalf("unauthenticated request = %v", err)
	}
	if keys.calls.Load() != 0 {
		t.Fatalf("trust source read before peer authentication: %d", keys.calls.Load())
	}
}

func TestConstructorRejectsMissingAndTypedNilTrustSource(t *testing.T) {
	dir := makePrivateDir(t)
	policy := validPolicy()
	if _, err := New(context.Background(), dir, nil, policy); err == nil {
		t.Fatal("missing trust source accepted")
	}
	var typedNil *testKeys
	if _, err := New(context.Background(), dir, typedNil, policy); err == nil {
		t.Fatal("typed nil trust source accepted")
	}
}

func TestConstructorHonorsCanceledContextBeforeFilesystemAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	keys := &testKeys{}
	if _, err := New(ctx, filepath.Join(os.TempDir(), "nonexistent-operator-case-directory"), keys, validPolicy()); !errors.Is(err, context.Canceled) {
		t.Fatalf("New with canceled context = %v", err)
	}
	if keys.calls.Load() != 0 {
		t.Fatalf("constructor accessed trust source: %d", keys.calls.Load())
	}
}

func TestAdmissionRequiresFreshCurrentTrustAndSignature(t *testing.T) {
	dir := makePrivateDir(t)
	privateKey := ed25519.NewKeyFromSeed(fixedSeed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keys := &testKeys{key: validTrustedKey(publicKey, "operator-7")}
	admission, err := New(context.Background(), dir, keys, validPolicy())
	if err != nil {
		t.Fatal(err)
	}
	validRef := writeCase(t, dir, privateKey, publicKeyID(publicKey), "operator-7", "operation-17", "committed", "fact:"+strings.Repeat("b", 64), testUnix, "2026-10-01T17:20:00Z")
	if _, err := authorizeOverUnix(t, admission, "operation-17", validRef); err != nil {
		t.Fatal(err)
	}
	if keys.calls.Load() != 1 {
		t.Fatalf("lookup count=%d", keys.calls.Load())
	}
	keys.key.Revoked = true
	if _, err := authorizeOverUnix(t, admission, "operation-17", validRef); !errors.Is(err, errDenied) {
		t.Fatalf("revoked key admission = %v", err)
	}
	if keys.calls.Load() != 2 {
		t.Fatalf("key source not refreshed on second admission: %d", keys.calls.Load())
	}
	keys.key.Revoked = false
	path := filepath.Join(dir, strings.TrimPrefix(validRef, "sha256:")+".json")
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	bytes[len(bytes)-2] ^= 1
	if err := os.WriteFile(path, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := authorizeOverUnix(t, admission, "operation-17", validRef); !errors.Is(err, errDenied) {
		t.Fatalf("tampered signed case = %v", err)
	}
	badRef := writeUnsignedCase(t, dir, privateKey, publicKeyID(publicKey))
	if _, err := authorizeOverUnix(t, admission, "operation-17", badRef); !errors.Is(err, errDenied) {
		t.Fatalf("bad signature admitted: %v", err)
	}
}

func TestRevocationAfterSnapshotAllowsAdmittedCallButDeniesNext(t *testing.T) {
	dir := makePrivateDir(t)
	privateKey := ed25519.NewKeyFromSeed(fixedSeed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keys := &barrierKeys{key: validTrustedKey(publicKey, "operator-7"), snapshotted: make(chan struct{}), resume: make(chan struct{})}
	admission, err := New(context.Background(), dir, keys, validPolicy())
	if err != nil {
		t.Fatal(err)
	}
	ref := writeCase(t, dir, privateKey, publicKeyID(publicKey), "operator-7", "operation-17", "committed", "fact:"+strings.Repeat("e", 64), testUnix, "2026-10-01T17:20:00Z")
	first := make(chan error, 1)
	go func() {
		_, authorizeErr := authorizeOverUnix(t, admission, "operation-17", ref)
		first <- authorizeErr
	}()
	select {
	case <-keys.snapshotted:
	case <-time.After(2 * time.Second):
		t.Fatal("current-key snapshot was not reached")
	}
	keys.mu.Lock()
	keys.key.Revoked = true
	keys.mu.Unlock()
	close(keys.resume)
	select {
	case err := <-first:
		if err != nil {
			t.Fatalf("revocation after snapshot canceled admitted call: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("admitted call did not finish")
	}
	if _, err := authorizeOverUnix(t, admission, "operation-17", ref); !errors.Is(err, errDenied) {
		t.Fatalf("next admission ignored revocation: %v", err)
	}
}

func TestStrictCaseParserRejectsAliasesDuplicatesUnknownAndTrailing(t *testing.T) {
	base := `{"version":1,"key_id":"k","operator_id":"o","operation_id":"x","outcome":"committed","expected_canonical_ref":"fact:` + strings.Repeat("a", 64) + `","approved_at":"2026-10-01T17:00:00Z","expires_at":"2026-10-01T17:20:00Z","signature":"` + base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize)) + `"}`
	if _, err := parseEnvelope([]byte(base)); err != nil {
		t.Fatalf("canonical envelope fixture rejected: %v", err)
	}
	mutations := map[string]string{
		"duplicate":                 strings.Replace(base, `"version":1`, `"version":1,"version":1`, 1),
		"case_alias_alone":          strings.Replace(base, `"version":1`, `"Version":1`, 1),
		"case_alias_with_canonical": strings.Replace(base, `"version":1`, `"version":1,"Version":1`, 1),
		"unknown":                   strings.Replace(base, `"version":1`, `"version":1,"issuer":"x"`, 1),
		"trailing_document":         base + `{}`,
		"nested_value":              strings.Replace(base, `"operator_id":"o"`, `"operator_id":{"x":1}`, 1),
		"invalid_utf8":              base[:len(base)-1] + string([]byte{0xff}) + `"}`,
	}
	for name, value := range mutations {
		t.Run(name, func(t *testing.T) {
			if _, err := parseEnvelope([]byte(value)); err == nil {
				t.Fatal("invalid envelope accepted")
			}
		})
	}
}

func TestOperationOutcomeProofAndTimeBinding(t *testing.T) {
	dir := makePrivateDir(t)
	privateKey := ed25519.NewKeyFromSeed(fixedSeed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keys := &testKeys{key: validTrustedKey(publicKey, "operator-7")}
	admission, err := New(context.Background(), dir, keys, validPolicy())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, operationID, outcome, proof, approved, expires string
	}{
		{"wrong_operation", "operation-18", "committed", "fact:" + strings.Repeat("c", 64), testUnix, "2026-10-01T17:20:00Z"},
		{"wrong_outcome", "operation-17", "released", "fact:" + strings.Repeat("c", 64), testUnix, "2026-10-01T17:20:00Z"},
		{"wrong_proof_shape", "operation-17", "committed", "HEAD", testUnix, "2026-10-01T17:20:00Z"},
		{"future_approval", "operation-17", "committed", "fact:" + strings.Repeat("c", 64), "2026-10-01T17:01:00Z", "2026-10-01T17:20:00Z"},
		{"expired_case", "operation-17", "committed", "fact:" + strings.Repeat("c", 64), "2026-10-01T16:00:00Z", "2026-10-01T16:30:00Z"},
		{"too_old", "operation-17", "committed", "fact:" + strings.Repeat("c", 64), "2026-10-01T14:00:00Z", "2026-10-01T19:00:00Z"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ref := writeCase(t, dir, privateKey, publicKeyID(publicKey), "operator-7", test.operationID, test.outcome, test.proof, test.approved, test.expires)
			if _, err := authorizeOverUnix(t, admission, "operation-17", ref); !errors.Is(err, errDenied) {
				t.Fatalf("invalid case admitted: %v", err)
			}
		})
	}
}

func TestTrustMappingMustMatchSignedOperatorID(t *testing.T) {
	dir := makePrivateDir(t)
	privateKey := ed25519.NewKeyFromSeed(fixedSeed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keys := &testKeys{key: validTrustedKey(publicKey, "different-operator")}
	admission, err := New(context.Background(), dir, keys, validPolicy())
	if err != nil {
		t.Fatal(err)
	}
	ref := writeCase(t, dir, privateKey, publicKeyID(publicKey), "operator-7", "operation-17", "committed", "fact:"+strings.Repeat("f", 64), testUnix, "2026-10-01T17:20:00Z")
	if _, err := authorizeOverUnix(t, admission, "operation-17", ref); !errors.Is(err, errDenied) {
		t.Fatalf("mismatched trusted operator mapping accepted: %v", err)
	}
}

func TestMalformedKeyIDIsRejectedBeforeTrustLookup(t *testing.T) {
	dir := makePrivateDir(t)
	privateKey := ed25519.NewKeyFromSeed(fixedSeed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keys := &testKeys{key: validTrustedKey(publicKey, "operator-7")}
	admission, err := New(context.Background(), dir, keys, validPolicy())
	if err != nil {
		t.Fatal(err)
	}
	ref := writeCase(t, dir, privateKey, "bad-key-id", "operator-7", "operation-17", "committed", "fact:"+strings.Repeat("a", 64), testUnix, "2026-10-01T17:20:00Z")
	if _, err := authorizeOverUnix(t, admission, "operation-17", ref); !errors.Is(err, errDenied) {
		t.Fatalf("malformed key ID admitted: %v", err)
	}
	if got := keys.calls.Load(); got != 0 {
		t.Fatalf("malformed key ID reached trust source: calls=%d", got)
	}
}

func TestDurationBoundsDoNotSaturateAtMaxInt64(t *testing.T) {
	dir := makePrivateDir(t)
	privateKey := ed25519.NewKeyFromSeed(fixedSeed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keys := &testKeys{key: validTrustedKey(publicKey, "operator-7")}
	keys.key.NotBefore, _ = time.Parse(time.RFC3339, "0001-01-02T00:00:00Z")
	keys.key.NotAfter, _ = time.Parse(time.RFC3339, "9999-12-31T23:59:59Z")
	policy := validPolicy()
	policy.MaxCaseAge = time.Duration(math.MaxInt64)
	policy.MaxCaseLifetime = time.Duration(math.MaxInt64)
	admission, err := New(context.Background(), dir, keys, policy)
	if err != nil {
		t.Fatal(err)
	}
	ref := writeCase(t, dir, privateKey, publicKeyID(publicKey), "operator-7", "operation-17", "committed", "fact:"+strings.Repeat("a", 64), "1000-01-01T00:00:00Z", "9000-01-01T00:00:00Z")
	if _, err := authorizeOverUnix(t, admission, "operation-17", ref); !errors.Is(err, errDenied) {
		t.Fatalf("duration subtraction overflow admitted centuries-old case: %v", err)
	}
}

func TestZeroClockCannotAdmitCase(t *testing.T) {
	dir := makePrivateDir(t)
	privateKey := ed25519.NewKeyFromSeed(fixedSeed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keys := &testKeys{key: validTrustedKey(publicKey, "operator-7")}
	policy := validPolicy()
	policy.Now = func() time.Time { return time.Time{} }
	admission, err := New(context.Background(), dir, keys, policy)
	if err != nil {
		t.Fatal(err)
	}
	ref := writeCase(t, dir, privateKey, publicKeyID(publicKey), "operator-7", "operation-17", "committed", "fact:"+strings.Repeat("a", 64), testUnix, "2026-10-01T17:20:00Z")
	if _, err := authorizeOverUnix(t, admission, "operation-17", ref); !errors.Is(err, errDenied) {
		t.Fatalf("zero clock admitted case: %v", err)
	}
}

func makePrivateDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(os.Getenv("TMPDIR"), "operator-case-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func validPolicy() Policy {
	now, _ := time.Parse(time.RFC3339Nano, testUnix)
	return Policy{ExpectedPeerUID: uint32(os.Geteuid()), MaxCaseAge: time.Hour, MaxCaseLifetime: time.Hour, Now: func() time.Time { return now }}
}

func authorizeOverUnix(t *testing.T, admission *Admission, operationID, reviewRef string) (service.ApprovedOperatorReview, error) {
	t.Helper()
	dir := makePrivateDir(t)
	listener, err := admintransport.Listen(context.Background(), filepath.Join(dir, "peer.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	type result struct {
		approved service.ApprovedOperatorReview
		err      error
	}
	completed := make(chan result, 1)
	server, err := listener.HTTPServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		approved, authorizeErr := admission.Authorize(request.Context(), request, operationID, reviewRef)
		completed <- result{approved: approved, err: authorizeErr}
	}))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Close()
		if err := <-done; !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("Serve: %v", err)
		}
	})
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", listener.Addr().String())
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	response, err := client.Get("http://admin/auth-context")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	select {
	case answer := <-completed:
		return answer.approved, answer.err
	case <-time.After(2 * time.Second):
		t.Fatal("authenticated request did not complete")
		return service.ApprovedOperatorReview{}, context.DeadlineExceeded
	}
}

func validTrustedKey(publicKey ed25519.PublicKey, operator string) TrustedKey {
	notBefore, _ := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")
	notAfter, _ := time.Parse(time.RFC3339, "2027-01-01T00:00:00Z")
	return TrustedKey{PublicKey: append([]byte(nil), publicKey...), OperatorID: operator, NotBefore: notBefore, NotAfter: notAfter}
}

func publicKeyID(publicKey ed25519.PublicKey) string {
	digest := sha256.Sum256(publicKey)
	return "ed25519-sha256:" + hex.EncodeToString(digest[:])
}

func writeCase(t *testing.T, dir string, privateKey ed25519.PrivateKey, keyID, operator, operationID, outcome, proof, approvedAt, expiresAt string) string {
	t.Helper()
	envelope := signedEnvelope{Version: 1, KeyID: keyID, OperatorID: operator, OperationID: operationID, Outcome: outcome, ExpectedCanonicalRef: proof, ApprovedAt: approvedAt, ExpiresAt: expiresAt}
	payload, err := canonicalPayload(envelope)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, append([]byte(signatureDomain), payload...)))
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	ref := "sha256:" + hex.EncodeToString(digest[:])
	if err := os.WriteFile(filepath.Join(dir, strings.TrimPrefix(ref, "sha256:")+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	return ref
}

func writeUnsignedCase(t *testing.T, dir string, privateKey ed25519.PrivateKey, keyID string) string {
	t.Helper()
	envelope := signedEnvelope{Version: 1, KeyID: keyID, OperatorID: "operator-7", OperationID: "operation-17", Outcome: "committed", ExpectedCanonicalRef: "fact:" + strings.Repeat("d", 64), ApprovedAt: testUnix, ExpiresAt: "2026-10-01T17:20:00Z", Signature: base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	ref := "sha256:" + hex.EncodeToString(digest[:])
	if err := os.WriteFile(filepath.Join(dir, strings.TrimPrefix(ref, "sha256:")+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	return ref
}

var _ service.OperatorReviewAdmission = (*Admission)(nil)
