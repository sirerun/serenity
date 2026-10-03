package recovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

func goldenContractPlan() contracts.RecoveryPlan {
	return contracts.RecoveryPlan{
		SourceSnapshot: strings.Repeat("a", 64),
		JournalWatermark: contracts.DeletionWatermark{
			Generation: 2, SequenceID: 3, EntryHash: strings.Repeat("b", 64),
		},
		Generation: 4, ProviderTruthAt: time.Date(2026, 10, 3, 0, 0, 0, 123456789, time.UTC),
		Accounts: []string{"acct-a", "acct-z"},
	}
}

func TestCanonicalContractRecoveryPlanHashVersionOneGolden(t *testing.T) {
	plan := goldenContractPlan()
	got, err := CanonicalContractRecoveryPlanHash(plan)
	if err != nil {
		t.Fatal(err)
	}
	wantPayload := `{"domain_version":1,"source_snapshot":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","journal_watermark":{"generation":2,"sequence_id":3,"entry_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},"generation":4,"provider_truth_at":"2026-10-03T00:00:00.123456789Z","accounts":["acct-a","acct-z"]}`
	wantDigest := "3f7f340a4861aa51a89f75ccb950e3b917e8db0eb75b2f39caf5e7538851e2d3"
	sum := sha256.Sum256([]byte(wantPayload))
	if hex.EncodeToString(sum[:]) != wantDigest {
		t.Fatal("independent golden digest fixture is inconsistent")
	}
	if string(got) != wantDigest {
		t.Fatalf("hash = %s, want %s", got, wantDigest)
	}
	plan.PlanHash = "caller supplied value is excluded"
	gotWithInputHash, err := CanonicalContractRecoveryPlanHash(plan)
	if err != nil || gotWithInputHash != got {
		t.Fatalf("input PlanHash affected canonical hash: %s, %v", gotWithInputHash, err)
	}
}

func TestCanonicalContractRecoveryPlanHashCoversEverySemanticField(t *testing.T) {
	base, err := CanonicalContractRecoveryPlanHash(goldenContractPlan())
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*contracts.RecoveryPlan){
		"snapshot":             func(p *contracts.RecoveryPlan) { p.SourceSnapshot = strings.Repeat("c", 64) },
		"watermark generation": func(p *contracts.RecoveryPlan) { p.JournalWatermark.Generation++ },
		"watermark sequence":   func(p *contracts.RecoveryPlan) { p.JournalWatermark.SequenceID++ },
		"watermark entry":      func(p *contracts.RecoveryPlan) { p.JournalWatermark.EntryHash = strings.Repeat("d", 64) },
		"fence generation":     func(p *contracts.RecoveryPlan) { p.Generation++ },
		"provider time":        func(p *contracts.RecoveryPlan) { p.ProviderTruthAt = p.ProviderTruthAt.Add(time.Nanosecond) },
		"account set":          func(p *contracts.RecoveryPlan) { p.Accounts[0] = "acct-b" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			p := goldenContractPlan()
			mutate(&p)
			got, err := CanonicalContractRecoveryPlanHash(p)
			if err != nil {
				t.Fatal(err)
			}
			if got == base {
				t.Fatal("semantic field mutation did not change hash")
			}
		})
	}
}

func TestCanonicalContractRecoveryPlanHashRejectsNoncanonicalOrUnboundedInput(t *testing.T) {
	tests := map[string]func(*contracts.RecoveryPlan){
		"non UTC location":   func(p *contracts.RecoveryPlan) { p.ProviderTruthAt = p.ProviderTruthAt.In(time.FixedZone("UTC", 0)) },
		"unsorted accounts":  func(p *contracts.RecoveryPlan) { p.Accounts[0], p.Accounts[1] = p.Accounts[1], p.Accounts[0] },
		"duplicate accounts": func(p *contracts.RecoveryPlan) { p.Accounts[1] = p.Accounts[0] },
		"invalid utf8":       func(p *contracts.RecoveryPlan) { p.Accounts[0] = string([]byte{0xff}) },
		"too many accounts": func(p *contracts.RecoveryPlan) {
			p.Accounts = make([]string, maxVerifiedEligibleAccounts+1)
			for i := range p.Accounts {
				p.Accounts[i] = "account" + strings.Repeat("a", 1) + string(rune('a'+i%26))
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			p := goldenContractPlan()
			mutate(&p)
			if _, err := CanonicalContractRecoveryPlanHash(p); err == nil {
				t.Fatal("expected invalid input error")
			}
		})
	}
	// Exercise the maximum supported account count and identifier size.
	p := goldenContractPlan()
	p.Accounts = make([]string, maxVerifiedEligibleAccounts)
	for i := range p.Accounts {
		p.Accounts[i] = "acct-" + strings.Repeat("a", 240) + string(rune('a'+i/26)) + string(rune('a'+i%26))
	}
	// Ensure lexicographic ordering independently of generated numeric order.
	for i := 1; i < len(p.Accounts); i++ {
		for j := i; j > 0 && p.Accounts[j] < p.Accounts[j-1]; j-- {
			p.Accounts[j], p.Accounts[j-1] = p.Accounts[j-1], p.Accounts[j]
		}
	}
	if _, err := CanonicalContractRecoveryPlanHash(p); err != nil {
		t.Fatalf("maximum supported account set rejected: %v", err)
	}
}

func TestVerifiedEligiblePlanCreateLoadAndArtifactIdentity(t *testing.T) {
	dir := privatePlanDir(t)
	in := validPlanInput()
	in.Accounts = []string{"account-a", "account-z"}
	token, err := createVerifiedEligiblePlan(context.Background(), dir, in)
	if err != nil {
		t.Fatal(err)
	}
	if token.artifactHash == "" || token.contractHash == "" || string(token.artifactHash) == string(token.contractHash) {
		t.Fatalf("artifact and contract identities were not distinct: %q %q", token.artifactHash, token.contractHash)
	}
	path := filepath.Join(dir, string(token.artifactHash)+".json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	legacyDir := privatePlanDir(t)
	legacyArtifact, err := CreatePlan(context.Background(), legacyDir, in)
	if err != nil {
		t.Fatal(err)
	}
	legacyBytes, err := os.ReadFile(filepath.Join(legacyDir, legacyArtifact.PlanHash+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, legacyBytes) {
		t.Fatal("verified creation changed legacy artifact serialization")
	}
	loaded, err := loadVerifiedEligiblePlan(context.Background(), dir, token.artifactHash)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("read-only contract mapping changed legacy artifact bytes")
	}
	if loaded.artifactHash != token.artifactHash || loaded.contractHash != token.contractHash || !reflect.DeepEqual(loaded.contractPlan, token.contractPlan) {
		t.Fatal("loaded verified plan differs from created token")
	}
	if loaded.artifact.PlanHash != string(loaded.artifactHash) {
		t.Fatal("legacy artifact identity was not preserved")
	}
	if loaded.contractPlan.SourceSnapshot != loaded.artifact.SnapshotSHA256 || loaded.contractPlan.JournalWatermark != loaded.artifact.JournalWatermark || loaded.contractPlan.Generation != loaded.artifact.FenceGeneration || loaded.contractPlan.ProviderTruthAt.Format(time.RFC3339Nano) != loaded.artifact.ProviderObserved || !reflect.DeepEqual(loaded.contractPlan.Accounts, loaded.artifact.Accounts) {
		t.Fatal("contract plan does not preserve the frozen artifact mapping")
	}
	if loaded.contractPlan.SourceSnapshot == loaded.artifact.PlanHash {
		t.Fatal("legacy artifact identity was copied into contract snapshot identity")
	}
}

func TestVerifiedEligiblePlanDefensiveCopyAndFailures(t *testing.T) {
	dir := privatePlanDir(t)
	in := validPlanInput()
	original := append([]string(nil), in.Accounts...)
	token, err := createVerifiedEligiblePlan(context.Background(), dir, in)
	if err != nil {
		t.Fatal(err)
	}
	in.Accounts[0] = "caller-mutated"
	if !reflect.DeepEqual(token.contractPlan.Accounts, []string{"account-a", "account-z"}) || !reflect.DeepEqual(token.artifact.Accounts, []string{"account-a", "account-z"}) {
		t.Fatal("token aliases caller account slice")
	}
	if !reflect.DeepEqual(original, []string{"account-z", "account-a"}) {
		t.Fatal("unexpected fixture mutation")
	}
	if _, err := loadVerifiedEligiblePlan(context.Background(), dir, "wrong-hash"); err == nil {
		t.Fatal("wrong artifact hash accepted")
	}
	artifactPath := filepath.Join(dir, string(token.artifactHash)+".json")
	artifactBytes, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, append(artifactBytes, ' '), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadVerifiedEligiblePlan(context.Background(), dir, token.artifactHash); err == nil {
		t.Fatal("tampered artifact accepted")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := createVerifiedEligiblePlan(canceled, privatePlanDir(t), validPlanInput()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled create error = %v", err)
	}
	if _, err := loadVerifiedEligiblePlan(canceled, dir, token.artifactHash); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled load error = %v", err)
	}
	tooMany := validPlanInput()
	tooMany.Accounts = make([]string, maxVerifiedEligibleAccounts+1)
	for i := range tooMany.Accounts {
		tooMany.Accounts[i] = "acct" + string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	noWriteDir := privatePlanDir(t)
	if _, err := createVerifiedEligiblePlan(context.Background(), noWriteDir, tooMany); err == nil {
		t.Fatal("oversized plan accepted")
	}
	entries, err := os.ReadDir(noWriteDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("oversized create wrote artifacts: %v", entries)
	}
}
