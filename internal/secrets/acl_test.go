package secrets

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestAddGenericPasswordCommandTrustsOnlyTheExecutable pins the SEC-L08
// fix on the command itself: the keychain item is created with an access
// list naming exactly the trusted executable (-T), and the secret is never
// placed on argv (it travels on `security -i` stdin, go-keyring's
// base64-prefixed encoding so keyring.Get still decodes it).
func TestAddGenericPasswordCommandTrustsOnlyTheExecutable(t *testing.T) {
	exe := "/Applications/Serenity Tools/serenity"
	cmd := addGenericPasswordCommand("", Service, daemonTokenKey, "s3cr'et", exe)

	if !strings.HasPrefix(cmd, "add-generic-password ") || !strings.HasSuffix(cmd, "\n") {
		t.Fatalf("not a single add-generic-password line: %q", cmd)
	}
	if strings.Contains(cmd, " -U") {
		t.Fatalf("command updates in place, keeping any existing wider ACL: %q", cmd)
	}
	if got := strings.Count(cmd, " -T "); got != 1 {
		t.Fatalf("want exactly one trusted application, got %d: %q", got, cmd)
	}
	if !strings.Contains(cmd, " -T '"+exe+"'") {
		t.Fatalf("access list does not name the executable %q: %q", exe, cmd)
	}
	if strings.Contains(cmd, "s3cr") {
		t.Fatalf("secret appears unencoded in the command: %q", cmd)
	}
	if !strings.Contains(cmd, " -w 'go-keyring-base64:czNjcidldA=='") {
		t.Fatalf("secret is not go-keyring base64 encoded: %q", cmd)
	}
	if strings.Contains(cmd, ".keychain") {
		t.Fatalf("default keychain requested but a keychain path was appended: %q", cmd)
	}

	withKC := addGenericPasswordCommand("/tmp/x.keychain-db", Service, "acct", "v", exe)
	if !strings.HasSuffix(withKC, " '/tmp/x.keychain-db'\n") {
		t.Fatalf("explicit keychain path not passed as the final argument: %q", withKC)
	}
}

// TestQuoteRoundTripsHostileInput checks the single-quote escaping used for
// every field on the `security -i` line: a profile name with quotes,
// spaces or shell metacharacters must stay one argument.
func TestQuoteRoundTripsHostileInput(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no POSIX sh to round-trip the quoting through")
	}
	for _, in := range []string{"", "plain", "it's", `a "b" $c; rm -rf / 'd'`, "x\ty"} {
		out, err := exec.Command("sh", "-c", "printf %s "+quote(in)).Output()
		if err != nil {
			t.Fatalf("sh rejected quote(%q) = %s: %v", in, quote(in), err)
		}
		if string(out) != in {
			t.Fatalf("quote(%q) round-tripped to %q", in, out)
		}
	}
}

// TestTrustedExecutableIsResolvedCurrentBinary: the ACL names the running
// binary by its symlink-resolved absolute path, which is what the keychain
// records for a trusted application.
func TestTrustedExecutableIsResolvedCurrentBinary(t *testing.T) {
	got, err := trustedExecutable()
	if err != nil {
		t.Fatalf("trustedExecutable: %v", err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatal(err)
	}
	if got != want || !filepath.IsAbs(got) {
		t.Fatalf("trustedExecutable() = %q, want absolute %q", got, want)
	}
}

// TestKeychainItemACLNamesCurrentExecutable is the SEC-L08 acceptance case:
// on macOS, an item created through setSecret's darwin path carries an
// access control list naming the current executable, so another process
// running as the same user (including /usr/bin/security) is prompted
// before it can read the secret. It uses a throwaway keychain, never the
// operator's login keychain, and never reads the secret back (a read
// through /usr/bin/security would itself be the prompted access).
func TestKeychainItemACLNamesCurrentExecutable(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("keychain access control lists exist only on macOS; the Linux Secret Service keyring is unchanged (T24.28 out of scope)")
	}
	kc := filepath.Join(t.TempDir(), "serenity-acl-test.keychain-db")
	const pw = "serenity-acl-test"
	if out, err := exec.Command(securityPath, "create-keychain", "-p", pw, kc).CombinedOutput(); err != nil {
		t.Fatalf("create-keychain: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command(securityPath, "delete-keychain", kc).Run() })
	if out, err := exec.Command(securityPath, "unlock-keychain", "-p", pw, kc).CombinedOutput(); err != nil {
		t.Fatalf("unlock-keychain: %v: %s", err, out)
	}

	exe, err := trustedExecutable()
	if err != nil {
		t.Fatalf("trustedExecutable: %v", err)
	}
	if err := addGenericPassword(kc, Service, daemonTokenKey, "token-value", exe); err != nil {
		t.Fatalf("addGenericPassword: %v", err)
	}

	out, err := exec.Command(securityPath, "dump-keychain", "-a", kc).CombinedOutput()
	if err != nil {
		t.Fatalf("dump-keychain -a: %v: %s", err, out)
	}
	dump := string(out)
	if !strings.Contains(dump, exe) {
		t.Fatalf("item ACL does not name the current executable %q:\n%s", exe, dump)
	}
	if strings.Contains(dump, securityPath+" ") || strings.Contains(dump, securityPath+"\n") {
		t.Fatalf("item ACL still trusts %s:\n%s", securityPath, dump)
	}
}
