package partner

import (
	"testing"
	"time"
)

func TestReturnURLMustExtendPrefixAtABoundary(t *testing.T) {
	prefix := "blink://serenity-linked"
	for raw, want := range map[string]bool{
		"blink://serenity-linked":              true,
		"blink://serenity-linked?x=1":          true,
		"blink://serenity-linked/done":         true,
		"blink://serenity-linked.evil":         false,
		"blink://serenity-linkedx":             false,
		"blink://serenity-linked#f":            false,
		"https://serenity-linked":              false,
		"blink://serenity-linked/\\evil":       false,
		"blink://serenity-linked?x=1 y":        false,
		"":                                     false,
		"BLINK://serenity-linked":              false,
		"blink://serenity-linked@evil.example": false,
	} {
		if got := returnURLAllowed(prefix, raw); got != want {
			t.Errorf("returnURLAllowed(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestRedirectPrefixValidation(t *testing.T) {
	for prefix, ok := range map[string]bool{
		"blink://serenity-linked":       true,
		"https://app.example/cb":        true,
		"http://app.example/cb":         false,
		"javascript://x":                false,
		"blink://serenity-linked?x=1":   false,
		"blink://user@serenity-linked":  false,
		"blink:serenity-linked":         false,
		"blink://serenity-linked#frag":  false,
		"blink://serenity-linked/a b":   false,
		"blink://serenity-linked/a'b":   false,
		"":                              false,
		"https://app.example/cb;inject": false,
	} {
		if err := ValidateRedirectPrefix(prefix); (err == nil) != ok {
			t.Errorf("ValidateRedirectPrefix(%q) = %v, want ok=%v", prefix, err, ok)
		}
	}
}

func TestPartnerIDs(t *testing.T) {
	for id, ok := range map[string]bool{"blink": true, "a-b_c9": true, "": false, "Blink": false, "a:b": false, "a/b": false, "0123456789012345678901234567890123": false} {
		if ValidID(id) != ok {
			t.Errorf("ValidID(%q) != %v", id, ok)
		}
	}
}

func TestLimiterIsPerPartnerAndResetsEachMinute(t *testing.T) {
	var l limiter
	now := time.Unix(1_700_000_000, 0)
	for i := 0; i < RateLimit; i++ {
		if !l.allow("blink", RateLimit, now) {
			t.Fatalf("request %d refused", i)
		}
	}
	if l.allow("blink", RateLimit, now.Add(59*time.Second)) {
		t.Fatal("601st request in the window allowed")
	}
	if !l.allow("other", RateLimit, now) {
		t.Fatal("limit leaked across partners")
	}
	if !l.allow("blink", RateLimit, now.Add(time.Minute)) {
		t.Fatal("window did not reset")
	}
}
