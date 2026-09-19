package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

// Provenance labels every fixture fact so it can never pass for customer data.
const Provenance = "fixture:T23.60 synthetic infrastructure-only"

// MaxFactBytes is the gateway's remember input cap (internal/hosted/gateway/gateway.go, len(input.Fact) > 4096).
const MaxFactBytes = 4096

// vocab is the same word list the load client renders text from (evals/hosted-load/harness.py _TEXT_VOCAB); a test
// compares the two so they cannot drift. Every word is at most 7 bytes.
var vocab = []string{
	"intake", "latency", "budget", "origin", "brain", "recall", "memory", "fact",
	"account", "plan", "upgrade", "cutover", "outage", "runbook",
	"review", "limit", "quota", "gateway", "pool", "session", "token", "retain",
	"backup", "index", "queue", "worker", "deploy", "rebuild", "signal", "metric", "alarm", "region",
	"server", "ticket", "vendor", "invoice", "renewal", "policy", "export", "import", "tenant", "audit",
}

// markerBytes is the longest unique marker word (an "f" plus four base-36 digits).
const markerBytes = 5

// factEpoch is the fixed, past creation time of fact 0. Facts are one second apart so the content, and therefore the
// content-addressed identity of every fact, is identical across runs.
var factEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

var factKinds = []store.MemoryFactKind{store.MemoryFactKindEvent, store.MemoryFactKindPreference, store.MemoryFactKindCommitment, store.MemoryFactKindBelief, store.MemoryFactKindFact}

// FactSpec is one synthetic fact for a (account label, brain index, fact index).
type FactSpec struct {
	Payload store.MemoryFactPayload
	Words   int
}

// maxRenderedBytes is the longest text factFor can render for a nominal word count.
func maxRenderedBytes(words int) int {
	return markerBytes + (words-1)*(7+1)
}

// factFor renders fact `index` of a brain deterministically. The text is a unique marker word followed by
// vocabulary words, sized uniformly in [minWords, maxWords] (the frozen workload's fact_tokens range, as a nominal
// word count and never a token count). The legacy id is index+1, so ids are unique in a brain by construction.
func factFor(label string, brain, index, minWords, maxWords int) FactSpec {
	seed := sha256.Sum256([]byte(fmt.Sprintf("serenity/fixtureprep/v1|%s|%d|%d", label, brain, index)))
	rng := rand.New(rand.NewPCG(binary.LittleEndian.Uint64(seed[0:8]), binary.LittleEndian.Uint64(seed[8:16])))
	n := minWords + rng.IntN(maxWords-minWords+1)
	words := make([]string, 0, n)
	marker := strconv.FormatInt(int64(index), 36)
	words = append(words, "f"+strings.Repeat("0", 4-len(marker))+marker)
	for len(words) < n {
		words = append(words, vocab[rng.IntN(len(vocab))])
	}
	return FactSpec{Words: n, Payload: store.MemoryFactPayload{
		FormatVersion: store.MemoryFactFormatVersion,
		RecordType:    store.SourceKindMemoryFact,
		LegacyID:      int64(index) + 1,
		Fact:          strings.Join(words, " "),
		Provenance:    Provenance,
		Kind:          factKinds[rng.IntN(len(factKinds))],
		Visibility:    store.MemoryVisibilityWorld,
		CreatedAt:     factEpoch.Add(time.Duration(index) * time.Second),
	}}
}
