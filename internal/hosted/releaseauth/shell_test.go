package releaseauth

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestShellPolicySelection(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "cosign-called")
	stub := filepath.Join(dir, "cosign")
	writeTest(t, stub, []byte("#!/bin/sh\nprintf called > \"$TEST_MARKER\"\nexit 0\n"))
	if err := os.Chmod(stub, 0700); err != nil {
		t.Fatal(err)
	}
	_, source, _, _ := runtime.Caller(0)
	script := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../deploy/hosted/verify-release.sh"))
	for _, policy := range []string{"", "sigstore", "unknown", "owner-kms-manifest-v1"} {
		t.Run(policy, func(t *testing.T) {
			_ = os.Remove(marker)
			cmd := exec.Command("bash", script, "archive", "signature", "v1.2.3")
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "TEST_MARKER="+marker, "SERENITY_RELEASE_POLICY="+policy, "SERENITY_RELEASE_VERIFIER=/does-not-exist")
			output, err := cmd.CombinedOutput()
			_, statErr := os.Stat(marker)
			if policy == "" || policy == "sigstore" {
				if err != nil || statErr != nil {
					t.Fatalf("default route failed %s %v", output, err)
				}
			} else if err == nil || !os.IsNotExist(statErr) {
				t.Fatalf("invalid KMS route fell back: %s %v", output, err)
			}
		})
	}
}
func TestRootShellKMSRoute(t *testing.T) {
	p, _ := rootFixture(t)
	dir := filepath.Dir(p.State)
	verifier := filepath.Join(dir, "verifier")
	marker := filepath.Join(dir, "arguments")
	writeTest(t, verifier, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TEST_MARKER\"\nexit 19\n"))
	if err := os.Chmod(verifier, 0700); err != nil {
		t.Fatal(err)
	}
	_, source, _, _ := runtime.Caller(0)
	script := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../deploy/hosted/verify-release.sh"))
	cmd := exec.Command("bash", script, p.Archive, p.Signature, p.Tag)
	cmd.Env = append(os.Environ(), "TEST_MARKER="+marker, "SERENITY_RELEASE_POLICY="+PolicyID, "SERENITY_RELEASE_VERIFIER="+verifier, "SERENITY_RELEASE_MANIFEST="+p.Manifest, "SERENITY_RELEASE_POLICY_FILE="+p.Policy, "SERENITY_RELEASE_APPROVAL_FILE="+p.Approval, "SERENITY_RELEASE_STATE_FILE="+p.State, "SERENITY_RELEASE_LOCK_FD=9")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("ignored verifier failure")
	}
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 19 {
		t.Fatalf("verifier not reached: %s %v", output, err)
	}
	args, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "verify\n--archive\n") || !strings.Contains(string(args), "--lock-fd\n9\n") {
		t.Fatalf("unexpected route args %s", args)
	}
}
