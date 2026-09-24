package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestResolveCredentialProfile covers RFC-BRAIN-AUTH-02's three outcomes:
// the flag entirely absent (legacy path), an explicit empty or otherwise
// malformed name (must fail closed), and a valid name.
func TestResolveCredentialProfile(t *testing.T) {
	cases := []struct {
		name    string
		set     bool
		value   string
		wantOK  bool
		wantErr bool
	}{
		{name: "absent", set: false, wantOK: false, wantErr: false},
		{name: "valid", set: true, value: "team-a", wantOK: true},
		{name: "explicit_empty", set: true, value: "", wantErr: true},
		{name: "uppercase", set: true, value: "Team-A", wantErr: true},
		{name: "underscore", set: true, value: "team_a", wantErr: true},
		{name: "leading_hyphen", set: true, value: "-team", wantErr: true},
		{name: "trailing_hyphen", set: true, value: "team-", wantErr: true},
		{name: "too_long", set: true, value: strings.Repeat("a", 65), wantErr: true},
		{name: "single_char", set: true, value: "a", wantOK: true},
		{name: "max_length", set: true, value: strings.Repeat("a", 64), wantOK: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			addCredentialProfileFlag(cmd)
			if tc.set {
				if err := cmd.Flags().Set(credentialProfileFlagName, tc.value); err != nil {
					t.Fatal(err)
				}
			}
			name, ok, err := resolveCredentialProfile(cmd)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (name=%q err=%v)", ok, tc.wantOK, name, err)
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if ok && name != tc.value {
				t.Fatalf("name = %q, want %q", name, tc.value)
			}
			if err != nil && strings.Contains(err.Error(), "fallback") {
				t.Fatal("error text should never suggest a fallback exists")
			}
		})
	}
}
