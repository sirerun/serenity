package main

import (
	"errors"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/plans"
)

const frozenWorkload = "../workload.json"

func loadFrozen(t *testing.T) *Workload {
	t.Helper()
	wl, err := LoadWorkload(frozenWorkload)
	if err != nil {
		t.Fatal(err)
	}
	return wl
}

func TestFullPlanIsTheFrozenCardinalities(t *testing.T) {
	p, err := BuildPlan(loadFrozen(t), ProfileFull, 0)
	if err != nil {
		t.Fatal(err)
	}
	if p.TotalAccounts != 14 || p.TotalBrains != 29 || p.TotalFacts != 90000 {
		t.Fatalf("full plan is %d accounts, %d brains, %d facts", p.TotalAccounts, p.TotalBrains, p.TotalFacts)
	}
	if p.Reduced || !p.SatisfiesFullCardinality {
		t.Fatalf("full plan must not be reduced and must satisfy the cardinalities: %+v", p)
	}
	byPlan := map[string]int{}
	for _, a := range p.Accounts {
		byPlan[a.PlanID]++
		sum := 0
		for _, b := range a.Brains {
			sum += b.Facts
		}
		if def := plans.Get(a.PlanID); sum != int(def.Memories) || len(a.Brains) != int(def.Brains) || a.Memories != sum {
			t.Errorf("%s: %d facts over %d brains, plan table says %d over %d", a.Label, sum, len(a.Brains), def.Memories, def.Brains)
		}
	}
	if byPlan["scale"] != 1 || byPlan["builder"] != 3 || byPlan["free"] != 10 {
		t.Fatalf("account mix is %v, want 1 scale, 3 builder, 10 free", byPlan)
	}
	// Builder's 10,000 do not divide by 3: the remainder goes to the first brain.
	for _, a := range p.Accounts {
		if a.PlanID == "builder" && (a.Brains[0].Facts != 3334 || a.Brains[1].Facts != 3333 || a.Brains[2].Facts != 3333) {
			t.Errorf("%s brain split is %+v", a.Label, a.Brains)
		}
	}
}

func TestSmokePlanIsReducedAndNeverFull(t *testing.T) {
	p, err := BuildPlan(loadFrozen(t), ProfileSmoke, DefaultSmokeFactsPerBrain)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Reduced || p.SatisfiesFullCardinality || p.TotalFacts != 290 || p.TotalBrains != 29 || p.TotalAccounts != 14 {
		t.Fatalf("smoke plan must be reduced, keep all 14 accounts and 29 brains, and hold 290 facts: %+v", p)
	}
	for _, n := range []int{0, -1, MaxSmokeFactsPerBrain + 1} {
		if _, err = BuildPlan(loadFrozen(t), ProfileSmoke, n); err == nil {
			t.Errorf("smoke facts per brain %d must be refused", n)
		}
	}
	if _, err = BuildPlan(loadFrozen(t), "medium", 10); err == nil {
		t.Error("an unknown profile must be refused")
	}
}

func TestPlanRefusesDriftedWorkload(t *testing.T) {
	cases := map[string]func(*Workload){
		"total facts":  func(w *Workload) { w.Cardinalities.Paid.TotalFacts = 89999 },
		"scale count":  func(w *Workload) { w.Cardinalities.Paid.Accounts["scale"] = 2 },
		"unknown plan": func(w *Workload) { w.Cardinalities.Paid.Accounts["enterprise"] = 1 },
		"no accounts":  func(w *Workload) { w.Cardinalities.Paid.Accounts = map[string]int{} },
	}
	for name, mutate := range cases {
		w := loadFrozen(t)
		mutate(w)
		if _, err := BuildPlan(w, ProfileFull, 0); !errors.Is(err, ErrPlanDrift) {
			t.Errorf("%s: want ErrPlanDrift, got %v", name, err)
		}
	}
}

// The load client keeps its own copy of the plan table (harness.py PLAN_ALLOWANCES); this fixture must agree with both.
func TestPlanTableMatchesTheLoadClient(t *testing.T) {
	src, err := os.ReadFile("../harness.py")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range planOrder {
		re := regexp.MustCompile(`"` + id + `": \{[^}]*"brains": (\d+), "memories": (\d+),[^}]*"storage_bytes": ([\d_]+)\}`)
		m := re.FindSubmatch(src)
		if m == nil {
			t.Fatalf("harness.py has no PLAN_ALLOWANCES row for %s", id)
		}
		brains, _ := strconv.Atoi(string(m[1]))
		mem, _ := strconv.Atoi(string(m[2]))
		storage, _ := strconv.Atoi(regexp.MustCompile("_").ReplaceAllString(string(m[3]), ""))
		def := plans.Get(id)
		if int64(brains) != def.Brains || int64(mem) != def.Memories || int64(storage) != def.StorageBytes {
			t.Errorf("%s: harness.py has %d brains, %d memories, %d bytes; plans.go has %d, %d, %d", id, brains, mem, storage, def.Brains, def.Memories, def.StorageBytes)
		}
	}
}
