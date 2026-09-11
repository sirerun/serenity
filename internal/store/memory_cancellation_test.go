package store_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

func TestMemoryCancellationCodecVersionAndTargetBoundary(t *testing.T) {
	now := time.Now().UTC()
	valid := store.MemoryExpiryPayload{OperationKey: "publication:1", ExpiredAt: now}
	bytes, err := store.EncodeMemoryExpiry(valid)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := store.DecodeMemoryExpiry(bytes)
	if err != nil || decoded.FormatVersion != 2 || decoded.OperationKey != valid.OperationKey {
		t.Fatal("v2 cancellation codec", err)
	}
	invalid := []store.MemoryExpiryPayload{
		{FormatVersion: 1, OperationKey: "publication:1", ExpiredAt: now},
		{FormatVersion: 2, OperationKey: "publication:1", TargetSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ExpiredAt: now},
		{FormatVersion: 2, TargetSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ExpiredAt: now},
		{OperationKey: "invalid key", ExpiredAt: now},
		{OperationKey: "publication:1"},
	}
	for _, p := range invalid {
		if _, err := store.EncodeMemoryExpiry(p); err == nil {
			t.Fatalf("invalid cancellation accepted: %+v", p)
		}
	}
	// A v1-only decoder sees an unsupported version, never an inert valid v1 expiry.
	var envelope struct {
		FormatVersion int `json:"format_version"`
	}
	if json.Unmarshal(bytes, &envelope) != nil || envelope.FormatVersion == 1 {
		t.Fatal("unsafe old-reader compatibility")
	}
}

func TestMemoryCancellationHidesFactFromMergedHistory(t *testing.T) {
	sources := store.NewSourceStore(t.TempDir())
	now := time.Now().UTC()
	cancellation, err := sources.WriteMemoryExpiry(store.MemoryExpiryPayload{OperationKey: "merged", Reason: "withdrawn", ExpiredAt: now})
	if err != nil {
		t.Fatal(err)
	}
	// Bypass the writer only to model independently merged canonical history.
	payload := independentMemoryPayload("late source from another history", 1)
	payload.OperationKey = "merged"
	written, err := sources.WriteMemoryFact(payload)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := store.LoadMemoryProjection(sources)
	if err != nil {
		t.Fatal(err)
	}
	record, ok := projection.Get(written.SHA256)
	if !ok || !record.Expired(now) || record.ExpirySHA256 != cancellation.SHA256 {
		t.Fatal("merged fact escaped cancellation")
	}
	if !projection.IsLifecycle(cancellation.SHA256) || store.MemoryEligible(projection, cancellation.SHA256, true, now) || store.MemoryEligible(projection, written.SHA256, true, now) {
		t.Fatal("cancellation or canceled fact entered remote retrieval")
	}
}
