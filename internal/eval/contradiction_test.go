package eval

import "testing"

// TestContradictionRecallExact builds 5 golden contradiction pairs (e.g.
// two has_balance claims for the same account with incompatible values in
// an overlapping valid window) and a fake detector (test-only) that
// correctly flags 4, misses 1 (a real false negative), and also raises one
// false alarm on a pair that is not actually contradictory.
func TestContradictionRecallExact(t *testing.T) {
	pairs := []ContradictionPair{
		{ID: "c1", ClaimA: "claim-a1", ClaimB: "claim-b1"},
		{ID: "c2", ClaimA: "claim-a2", ClaimB: "claim-b2"},
		{ID: "c3", ClaimA: "claim-a3", ClaimB: "claim-b3"},
		{ID: "c4", ClaimA: "claim-a4", ClaimB: "claim-b4"},
		{ID: "c5", ClaimA: "claim-a5", ClaimB: "claim-b5"},
	}
	detected := map[string]bool{
		"c1": true,
		"c2": true,
		"c3": true,
		"c4": true,
		// c5 missed -- a real false negative.
	}
	falsePositives := 1 // the detector also flagged one non-contradictory pair.

	// TP=4, FN=1, FP=1.
	//   Recall    = 4/(4+1) = 0.8
	//   Precision = 4/(4+1) = 0.8
	//   F1        = 2*0.8*0.8/(0.8+0.8) = 0.8
	wantTP, wantFP, wantFN := 4, 1, 1
	wantPrecision := float64(wantTP) / float64(wantTP+wantFP)
	wantRecall := float64(wantTP) / float64(wantTP+wantFN)
	wantF1 := 2 * wantPrecision * wantRecall / (wantPrecision + wantRecall)
	want := PRF1{TP: wantTP, FP: wantFP, FN: wantFN, Precision: wantPrecision, Recall: wantRecall, F1: wantF1}

	got := ContradictionRecall(pairs, detected, falsePositives)
	if got != want {
		t.Errorf("ContradictionRecall = %+v, want %+v", got, want)
	}
}

func TestContradictionRecallNoDetections(t *testing.T) {
	pairs := []ContradictionPair{{ID: "c1"}, {ID: "c2"}}
	got := ContradictionRecall(pairs, map[string]bool{}, 0)
	want := PRF1{TP: 0, FP: 0, FN: 2, Precision: 0, Recall: 0, F1: 0}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestContradictionRecallAllDetected(t *testing.T) {
	pairs := []ContradictionPair{{ID: "c1"}, {ID: "c2"}, {ID: "c3"}}
	detected := map[string]bool{"c1": true, "c2": true, "c3": true}
	got := ContradictionRecall(pairs, detected, 0)
	want := PRF1{TP: 3, FP: 0, FN: 0, Precision: 1, Recall: 1, F1: 1}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
