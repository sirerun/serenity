package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

func TestFactsAreDeterministicAndDistinct(t *testing.T) {
	a, b := factFor("scale-0", 3, 17, 32, 512), factFor("scale-0", 3, 17, 32, 512)
	if a.Payload.Fact != b.Payload.Fact || a.Payload.Kind != b.Payload.Kind || !a.Payload.CreatedAt.Equal(b.Payload.CreatedAt) {
		t.Fatal("the same label, brain and index must render the same fact")
	}
	seen := map[string]bool{}
	ids := map[int64]bool{}
	for i := 0; i < 5000; i++ {
		enc, err := store.EncodeMemoryFact(factFor("scale-0", 0, i, 32, 512).Payload)
		if err != nil {
			t.Fatalf("fact %d does not encode: %v", i, err)
		}
		sum := sha256.Sum256(enc)
		sha := hex.EncodeToString(sum[:])
		if seen[sha] {
			t.Fatalf("fact %d repeats an earlier fact", i)
		}
		seen[sha] = true
		id := factFor("scale-0", 0, i, 32, 512).Payload.LegacyID
		if id != int64(i)+1 || ids[id] {
			t.Fatalf("fact %d legacy id %d is not sequential and unique", i, id)
		}
		ids[id] = true
	}
	if factFor("scale-0", 0, 1, 32, 512).Payload.Fact == factFor("scale-0", 1, 1, 32, 512).Payload.Fact || factFor("scale-0", 0, 1, 32, 512).Payload.Fact == factFor("free-0", 0, 1, 32, 512).Payload.Fact {
		t.Fatal("a different brain or account must render different text")
	}
}

func TestFactsStayUnderTheGatewayCapAtTheLongestSize(t *testing.T) {
	if got := maxRenderedBytes(512); got > MaxFactBytes {
		t.Fatalf("512 words can render %d bytes, over the %d byte cap", got, MaxFactBytes)
	}
	longest := 0
	for i := 0; i < 2000; i++ {
		f := factFor("free-0", 0, i, 512, 512)
		if n := len(f.Payload.Fact); n > longest {
			longest = n
		}
		if len(strings.Fields(f.Payload.Fact)) != 512 {
			t.Fatalf("fact %d has %d words, want the nominal 512", i, len(strings.Fields(f.Payload.Fact)))
		}
	}
	if longest > maxRenderedBytes(512) || longest > MaxFactBytes {
		t.Fatalf("longest rendered fact is %d bytes", longest)
	}
}

func TestFactFieldsAreSyntheticAndInThePast(t *testing.T) {
	kinds := map[store.MemoryFactKind]bool{}
	for i := 0; i < 200; i++ {
		f := factFor("builder-1", 2, i, 32, 512)
		p := f.Payload
		if p.Provenance != Provenance || !strings.Contains(p.Provenance, "synthetic") || p.Visibility != store.MemoryVisibilityWorld {
			t.Fatalf("fact %d provenance %q visibility %q", i, p.Provenance, p.Visibility)
		}
		if !p.CreatedAt.Before(time.Now()) || !p.CreatedAt.Equal(factEpoch.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("fact %d created_at %v must be a fixed past time", i, p.CreatedAt)
		}
		if f.Words < 32 || f.Words > 512 || len(strings.Fields(p.Fact)) != f.Words {
			t.Fatalf("fact %d has %d words, want 32..512", i, len(strings.Fields(p.Fact)))
		}
		kinds[p.Kind] = true
	}
	if len(kinds) != 5 {
		t.Fatalf("expected all five fact kinds, got %d", len(kinds))
	}
}

// The word list is copied from the load client; a drift would make fixture facts a different shape from offered ones.
func TestVocabMatchesTheLoadClient(t *testing.T) {
	src, err := os.ReadFile("../harness.py")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)_TEXT_VOCAB = \[(.*?)\]`).FindSubmatch(src)
	if block == nil {
		t.Fatal("harness.py has no _TEXT_VOCAB")
	}
	var want []string
	for _, m := range regexp.MustCompile(`"([a-z]+)"`).FindAllSubmatch(block[1], -1) {
		want = append(want, string(m[1]))
	}
	if strings.Join(want, ",") != strings.Join(vocab, ",") {
		t.Fatalf("vocab drifted from harness.py _TEXT_VOCAB:\n go %v\n py %v", vocab, want)
	}
	for _, w := range vocab {
		if len(w) > 7 {
			t.Errorf("word %q is longer than 7 bytes", w)
		}
	}
}
