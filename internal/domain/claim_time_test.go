package domain

import (
	"testing"
	"time"
)

func TestClaimCurrentAtValidityBoundaries(t *testing.T) {
	now := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, from, to string
		want           bool
	}{{"unbounded", "", "", true}, {"inclusive-start", "2026-09-08", "", true}, {"exclusive-end", "", "2026-09-08", false}, {"month", "2026-09", "2026-10", true}, {"year", "2026", "2027", true}, {"future", "2027", "", false}, {"expired", "", "2025", false}, {"bad-start", "tomorrow", "", false}, {"bad-end", "", "eventually", false}} {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Claim{ValidFrom: tc.from, ValidTo: tc.to}).CurrentAt(now); got != tc.want {
				t.Fatalf("CurrentAt=%v", got)
			}
		})
	}
}
