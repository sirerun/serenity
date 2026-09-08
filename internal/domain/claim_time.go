package domain

import "time"

// CurrentAt reports whether a claim's validity window includes now. Lifecycle
// and visibility are separate checks. Invalid dates fail closed; valid_until
// is exclusive, matching the canonical claim composition policy.
func (c Claim) CurrentAt(now time.Time) bool {
	parse := func(value string) (time.Time, bool) {
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02", "2006-01", "2006"} {
			if t, err := time.Parse(layout, value); err == nil {
				return t, true
			}
		}
		return time.Time{}, false
	}
	if c.ValidFrom != "" {
		from, ok := parse(c.ValidFrom)
		if !ok || now.Before(from) {
			return false
		}
	}
	if c.ValidTo != "" {
		until, ok := parse(c.ValidTo)
		if !ok || !now.Before(until) {
			return false
		}
	}
	return true
}
