package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

func TestOwnerSealBytesAreRetryStable(t *testing.T) {
	at := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	key, a, err := approvedSeal(1, 1, "writer", "genesis", at)
	if err != nil {
		t.Fatal(err)
	}
	_, b, err := approvedSeal(1, 1, "writer", "genesis", at)
	if err != nil || !bytes.Equal(a, b) {
		t.Fatal("seal retry changed bytes")
	}
	var o contracts.JournalObject
	if err = json.Unmarshal(a, &o); err != nil {
		t.Fatal(err)
	}
	if key != contracts.JournalKey(1, 1) || o.Kind != contracts.JournalKindSeal || o.PrevHash != "" {
		t.Fatal("seal coordinates mismatch")
	}
	for _, tc := range []struct {
		generation, sequence int64
		hash                 string
	}{{2, 1, "genesis"}, {1, 2, "genesis"}, {1, 0, strings.Repeat("a", 64)}, {1, 1, "wrong"}} {
		if _, _, err = approvedSeal(tc.generation, tc.sequence, "writer", tc.hash, at); err == nil {
			t.Fatal("invalid exact seal accepted")
		}
	}
}
func TestOwnerCloseRequiresExplicitCapability(t *testing.T) {
	cmd := newHostedAuthorityCmd()
	cmd.SetArgs([]string{"close", "--operation-id", strings.Repeat("a", 64)})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("blind close not rejected before credentials: %v", err)
	}
}
func TestOwnerOperationIDNeedsNoProvider(t *testing.T) {
	cmd := newHostedAuthorityCmd()
	cmd.SetArgs([]string{"new-operation-id"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(strings.TrimSpace(out.String())) != 64 {
		t.Fatal("invalid operation ID")
	}
}
