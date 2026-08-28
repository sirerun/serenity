package eval

// ContradictionPair is two claims a correctly-working reconciler should
// flag as mutually contradictory (RFC 0001 §16/§17, e.g. two has_balance
// claims for the same account with incompatible values in an overlapping
// valid window). ID is the pair's stable key in the detected map
// ContradictionRecall reads.
type ContradictionPair struct {
	ID     string
	ClaimA string
	ClaimB string
}

// ContradictionRecall reports precision/recall/F1 for contradiction
// detection: recall is the fraction of golden pairs a detector actually
// flagged (detected[pair.ID] == true); falsePositives is the count of
// additional pairs the detector flagged that are NOT in the golden set,
// supplied by the caller since ContradictionRecall only receives the
// golden pairs. RFC §16/§17 require contradiction-detection recall to be
// reported explicitly, as its own metric distinct from per-family P/R/F1.
func ContradictionRecall(pairs []ContradictionPair, detected map[string]bool, falsePositives int) PRF1 {
	var tp, fn int
	for _, p := range pairs {
		if detected[p.ID] {
			tp++
		} else {
			fn++
		}
	}
	return prf1(tp, falsePositives, fn)
}
