package cli

import (
	"fmt"
	"regexp"

	"github.com/spf13/cobra"
)

// credentialProfileFlagName is the flag every credential-lifecycle command
// (`connect`) and `serve --http` share, so the same three outcomes --
// absent, explicit-empty-or-malformed, or a valid name -- are decided
// identically everywhere (RFC-BRAIN-AUTH-02). It is never read from brain
// files, serenity.yml, .serenity, or the environment: only this flag, typed
// by the operator at each invocation, selects a profile.
const credentialProfileFlagName = "credential-profile"

// profileNamePattern is the bounded grammar RFC-BRAIN-AUTH-02 requires:
// 1-64 lowercase ASCII letters, digits and hyphens, not starting or ending
// with a hyphen. An explicitly empty value (`--credential-profile ""`) does
// not match this and is treated identically to any other malformed name --
// both fail closed, never silently falling back to the legacy shared
// credential.
var profileNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$`)

func addCredentialProfileFlag(cmd *cobra.Command) {
	cmd.Flags().String(credentialProfileFlagName, "", "operator-selected credential profile name, isolating this daemon's bearer token from the legacy shared one and from every other profile (never read from brain files, config, .serenity or the environment)")
}

// resolveCredentialProfile reads --credential-profile from cmd. Three
// outcomes only: the flag was never passed at all (ok=false, err=nil --
// callers take the exact legacy path, unchanged); it was passed with a
// name that fails the bounded grammar, including an explicit empty string
// (ok=false, err!=nil -- callers must fail closed, never fall back to the
// legacy credential); or it was passed with a valid name (ok=true).
func resolveCredentialProfile(cmd *cobra.Command) (name string, ok bool, err error) {
	if !cmd.Flags().Changed(credentialProfileFlagName) {
		return "", false, nil
	}
	name, err = cmd.Flags().GetString(credentialProfileFlagName)
	if err != nil {
		return "", false, err
	}
	if !profileNamePattern.MatchString(name) {
		return "", false, fmt.Errorf("--%s %q: must be 1-64 lowercase letters, digits or hyphens, not starting or ending with a hyphen", credentialProfileFlagName, name)
	}
	return name, true, nil
}
