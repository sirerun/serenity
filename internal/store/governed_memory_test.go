package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

func TestGovernedMalformedIdentityFailsWithoutPanic(t *testing.T) {
	request := json.RawMessage(`{"kind":"remember"}`)
	sum := sha256.Sum256(request)
	rec := GovernedRecord{Schema: GovernedMemorySchema, Request: request, RequestDigest: hex.EncodeToString(sum[:]), RecordedAt: time.Now().UTC(), Response: GovernedResponse{Schema: GovernedMemorySchema, BrainID: "00000000-0000-4000-8000-000000000001", CommandID: "00000000-0000-4000-8000-000000000002", Kind: "remember", Status: "completed", Claims: []GovernedClaim{{ID: "x", Version: 1, BrainID: "00000000-0000-4000-8000-000000000001", Freshness: time.Now().UTC(), Active: true}}}}
	s := NewSourceStore(t.TempDir())
	if _, err := s.PutGovernedMemory(rec); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadGovernedMemory(s); err == nil {
		t.Fatal("malformed governed claim accepted")
	}
}
