package episodejson

import "testing"

func TestProtocolKeysAndAmbiguity(t *testing.T) {
	for _, raw := range []string{
		`{"source_sha256":"abc","fences":{"0123456789abcdef":1}}`,
		`{"text":"\ud83d\ude00"}`,
	} {
		if !Valid([]byte(raw)) {
			t.Fatalf("valid protocol JSON rejected: %s", raw)
		}
	}
	for _, raw := range []string{
		`{"generation":1,"generation":2}`,
		`{"generation":1,"Generation":2}`,
		`{"ſchema":"alias"}`,
		`{"text":"\ud83d"}`,
		`{"text":"\ude00"}`,
		`{"text":"ok"} {}`,
	} {
		if Valid([]byte(raw)) {
			t.Fatalf("ambiguous JSON accepted: %s", raw)
		}
	}
	if Valid([]byte{'{', '"', 't', '"', ':', '"', 0xff, '"', '}'}) {
		t.Fatal("invalid UTF8 accepted")
	}
}
