package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/sirerun/serenity/internal/hosted/plans"
)

// Profile names. Only "full" can satisfy the frozen cardinalities; "smoke" is a reduced
// preparation that keeps every account and brain but a handful of facts per brain.
const (
	ProfileFull  = "full"
	ProfileSmoke = "smoke"
)

// DefaultSmokeFactsPerBrain is the reduced smoke size: 29 brains x 10 facts = 290 memories.
const DefaultSmokeFactsPerBrain = 10

// MaxSmokeFactsPerBrain keeps a smoke plan visibly smaller than the smallest full brain share (Free: 1,000).
const MaxSmokeFactsPerBrain = 100

// ErrPlanDrift means the frozen workload no longer matches the plan table this tool derives from.
var ErrPlanDrift = errors.New("fixtureprep: frozen workload cardinalities drifted from the plan table")

// Workload is the subset of evals/hosted-load/workload.json this tool reads. Nothing is written back.
type Workload struct {
	Cardinalities struct {
		Paid struct {
			Accounts   map[string]int `json:"accounts"`
			TotalFacts int            `json:"total_facts"`
		} `json:"paid"`
	} `json:"cardinalities"`
	FactTokens struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"fact_tokens"`
	TrafficMix map[string]float64 `json:"traffic_mix"`
	SHA256     string             `json:"-"`
}

// LoadWorkload reads the frozen workload and records its sha256 so a receipt can pin the exact file.
func LoadWorkload(path string) (*Workload, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read frozen workload: %w", err)
	}
	var w Workload
	if err = json.Unmarshal(raw, &w); err != nil {
		return nil, fmt.Errorf("decode frozen workload: %w", err)
	}
	sum := sha256.Sum256(raw)
	w.SHA256 = hex.EncodeToString(sum[:])
	return &w, nil
}

// BrainSpec is one planned brain. Index 0 is the account's default brain.
type BrainSpec struct {
	Index int `json:"index"`
	Facts int `json:"facts"`
}

// AccountSpec is one planned account, labelled the way the load client labels it (scale-0, builder-1, free-9).
type AccountSpec struct {
	Label    string      `json:"label"`
	PlanID   string      `json:"plan_id"`
	Memories int         `json:"memories"`
	Brains   []BrainSpec `json:"brains"`
}

// Plan is the deterministic account, brain and fact-count layout. It is derived from the frozen workload and the
// versioned plan table only, so the verifier recomputes it and never trusts a manifest.
type Plan struct {
	Profile                  string        `json:"profile"`
	Reduced                  bool          `json:"reduced"`
	SmokeFactsPerBrain       int           `json:"smoke_facts_per_brain,omitempty"`
	SatisfiesFullCardinality bool          `json:"satisfies_full_cardinality"`
	Accounts                 []AccountSpec `json:"accounts"`
	TotalAccounts            int           `json:"total_accounts"`
	TotalBrains              int           `json:"total_brains"`
	TotalFacts               int           `json:"total_facts"`
}

// planOrder is the order the load client lists accounts in (Scale, then Builder, then Free).
var planOrder = []string{"scale", "builder", "free"}

// BuildPlan derives the layout for a profile. For "full" every account holds exactly its plan's memory cap, spread
// over exactly its plan's brains (remainder to the first brains), and the total must equal the frozen
// cardinalities.paid.total_facts. For "smoke" every brain holds smokeFacts facts and the plan is marked reduced.
func BuildPlan(w *Workload, profile string, smokeFacts int) (*Plan, error) {
	if profile != ProfileFull && profile != ProfileSmoke {
		return nil, fmt.Errorf("fixtureprep: profile must be %q or %q, got %q", ProfileFull, ProfileSmoke, profile)
	}
	if profile == ProfileSmoke && (smokeFacts < 1 || smokeFacts > MaxSmokeFactsPerBrain) {
		return nil, fmt.Errorf("fixtureprep: smoke facts per brain must be 1..%d, got %d", MaxSmokeFactsPerBrain, smokeFacts)
	}
	counts := w.Cardinalities.Paid.Accounts
	known := map[string]bool{}
	for _, id := range planOrder {
		known[id] = true
	}
	var extra []string
	for id := range counts {
		if !known[id] {
			extra = append(extra, id)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return nil, fmt.Errorf("%w: unknown plan ids %v", ErrPlanDrift, extra)
	}
	p := &Plan{Profile: profile, Reduced: profile == ProfileSmoke}
	if p.Reduced {
		p.SmokeFactsPerBrain = smokeFacts
	}
	for _, id := range planOrder {
		def := plans.Get(id)
		if def.ID != id {
			return nil, fmt.Errorf("%w: plan %q missing from the plan table", ErrPlanDrift, id)
		}
		for i := 0; i < counts[id]; i++ {
			acct := AccountSpec{Label: fmt.Sprintf("%s-%d", id, i), PlanID: id}
			total := int(def.Memories)
			if p.Reduced {
				total = smokeFacts * int(def.Brains)
			}
			base, rem := total/int(def.Brains), total%int(def.Brains)
			for b := 0; b < int(def.Brains); b++ {
				n := base
				if b < rem {
					n++
				}
				acct.Brains = append(acct.Brains, BrainSpec{Index: b, Facts: n})
			}
			acct.Memories = total
			p.Accounts = append(p.Accounts, acct)
			p.TotalAccounts++
			p.TotalBrains += len(acct.Brains)
			p.TotalFacts += total
		}
	}
	if p.TotalAccounts == 0 {
		return nil, fmt.Errorf("%w: no accounts", ErrPlanDrift)
	}
	if profile == ProfileFull {
		if p.TotalFacts != w.Cardinalities.Paid.TotalFacts {
			return nil, fmt.Errorf("%w: plan table gives %d facts, frozen total_facts is %d", ErrPlanDrift, p.TotalFacts, w.Cardinalities.Paid.TotalFacts)
		}
		p.SatisfiesFullCardinality = p.TotalAccounts == 14 && p.TotalBrains == 29 && p.TotalFacts == 90000
		if !p.SatisfiesFullCardinality {
			return nil, fmt.Errorf("%w: full profile gives %d accounts, %d brains, %d facts, want 14, 29, 90000", ErrPlanDrift, p.TotalAccounts, p.TotalBrains, p.TotalFacts)
		}
	}
	return p, nil
}
