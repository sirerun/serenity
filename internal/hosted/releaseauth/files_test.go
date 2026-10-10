package releaseauth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func writeTest(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}
func rootFixture(t *testing.T) (Paths, Manifest) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("root-owned filesystem integration requires root test binary")
	}
	dir, err := os.MkdirTemp("/var/lib", "serenity-release-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	m, p, archive, sig, _ := fixture(t)
	paths := Paths{Archive: filepath.Join(dir, "archive"), Manifest: filepath.Join(dir, "manifest"), Signature: filepath.Join(dir, "signature"), Policy: filepath.Join(dir, "policy"), Approval: filepath.Join(dir, "approval"), State: filepath.Join(dir, "state"), Tag: m.Tag, LockFD: -1}
	writeTest(t, paths.Archive, archive)
	writeTest(t, paths.Manifest, encode(t, m))
	writeTest(t, paths.Signature, sig)
	writeTest(t, paths.Policy, encode(t, p))
	s := genesis()
	writeTest(t, paths.State, encode(t, s))
	writeTest(t, paths.Approval, encode(t, approvalFixture(t, m, s)))
	return paths, m
}
func TestRootTransactionLifecycle(t *testing.T) {
	p, m := rootFixture(t)
	tx, err := OpenTransaction(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err = tx.Verify(testNow); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Commit(testNow); err == nil {
		t.Fatal("commit accepted without pending")
	}
	if _, err = tx.Prepare(testNow); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Verify(testNow); err == nil {
		t.Fatal("verify accepted pending")
	}
	if _, err = tx.Prepare(testNow); err == nil {
		t.Fatal("double prepare accepted")
	}
	if _, err = tx.Recover(testNow, strings.Repeat("0", 64)); err == nil {
		t.Fatal("wrong installed recovery pin accepted")
	}
	if _, err = tx.Commit(testNow); err != nil {
		t.Fatal(err)
	}
	b, err := ReadTrusted(p.State)
	if err != nil {
		t.Fatal(err)
	}
	var state State
	if err = DecodeCanonical(b, &state); err != nil {
		t.Fatal(err)
	}
	if state.InstalledSequence != m.Sequence {
		t.Fatal("state not committed")
	}
	if _, err = tx.Verify(testNow); err == nil {
		t.Fatal("replay accepted")
	}
	if _, err = os.Lstat(p.State + ".pending"); !os.IsNotExist(err) {
		t.Fatal("pending not removed")
	}
}
func TestRootCrashRecovery(t *testing.T) {
	for _, afterState := range []bool{false, true} {
		t.Run(map[bool]string{false: "after-prepare", true: "after-state-write"}[afterState], func(t *testing.T) {
			p, m := rootFixture(t)
			tx, err := OpenTransaction(p)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := tx.Close(); err != nil {
					t.Error(err)
				}
			})
			if _, err = tx.Prepare(testNow); err != nil {
				t.Fatal(err)
			}
			if afterState {
				next := State{Version: 1, HighestSequence: 1, InstalledSequence: 1, ManifestSHA256: Digest(encode(t, m))}
				if err = replaceState(p.State, encode(t, next)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = tx.Verify(testNow); err == nil {
				t.Fatal("pending allowed normal verify")
			}
			if _, err = tx.Recover(testNow, Digest(encode(t, m))); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestRootTamperedPendingAndCurrentState(t *testing.T) {
	p, m := rootFixture(t)
	tx, err := OpenTransaction(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err = tx.Prepare(testNow); err != nil {
		t.Fatal(err)
	}
	writeTest(t, p.State, encode(t, State{Version: 1, HighestSequence: 9, InstalledSequence: 9, ManifestSHA256: strings.Repeat("a", 64)}))
	if _, err = tx.Commit(testNow); err == nil {
		t.Fatal("accepted changed current state")
	}
	if _, err = tx.Recover(testNow, Digest(encode(t, m))); err == nil {
		t.Fatal("recovered unrelated state")
	}
}
func TestRootLockExclusionAndInheritance(t *testing.T) {
	p, _ := rootFixture(t)
	tx, err := OpenTransaction(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Close(); err != nil {
			t.Error(err)
		}
	})
	if other, err := OpenTransaction(p); err == nil {
		if err := other.Close(); err != nil {
			t.Error(err)
		}
		t.Fatal("concurrent independent lock accepted")
	}
	p.LockFD = int(tx.lock.Fd())
	child, err := OpenTransaction(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = child.Verify(testNow); err != nil {
		t.Fatal(err)
	}
	if err = child.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Verify(testNow); err != nil {
		t.Fatal("closing inherited wrapper damaged original lock", err)
	}
	wrong, err := os.Open(p.Policy)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := wrong.Close(); err != nil {
			t.Error(err)
		}
	})
	p.LockFD = int(wrong.Fd())
	if child, err = OpenTransaction(p); err == nil {
		if err := child.Close(); err != nil {
			t.Error(err)
		}
		t.Fatal("wrong inherited descriptor accepted")
	}
}
func TestRootTrustFileSafety(t *testing.T) {
	for _, name := range []string{"symlink", "writable", "owner", "parent", "missing-state", "fifo"} {
		t.Run(name, func(t *testing.T) {
			p, _ := rootFixture(t)
			switch name {
			case "symlink":
				dest := p.Policy + ".original"
				if err := os.Rename(p.Policy, dest); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(dest, p.Policy); err != nil {
					t.Fatal(err)
				}
			case "writable":
				if err := os.Chmod(p.Policy, 0666); err != nil {
					t.Fatal(err)
				}
			case "owner":
				if err := os.Chown(p.Policy, 65534, 65534); err != nil {
					t.Fatal(err)
				}
			case "parent":
				if err := os.Chmod(filepath.Dir(p.Policy), 0777); err != nil {
					t.Fatal(err)
				}
			case "missing-state":
				if err := os.Remove(p.State); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := os.Remove(p.Policy); err != nil {
					t.Fatal(err)
				}
				if err := syscall.Mkfifo(p.Policy, 0600); err != nil {
					t.Fatal(err)
				}
			}
			tx, err := OpenTransaction(p)
			if err != nil {
				if name == "parent" {
					return
				}
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := tx.Close(); err != nil {
					t.Error(err)
				}
			})
			if _, err = tx.Verify(testNow); err == nil {
				t.Fatal("unsafe trusted input accepted")
			}
		})
	}
}
func TestRootCanonicalPolicyAndApproval(t *testing.T) {
	p, _ := rootFixture(t)
	b, err := os.ReadFile(p.Approval)
	if err != nil {
		t.Fatal(err)
	}
	var a map[string]any
	if err = json.Unmarshal(b, &a); err != nil {
		t.Fatal(err)
	}
	a["extra"] = true
	writeTest(t, p.Approval, encode(t, a))
	tx, err := OpenTransaction(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err = tx.Verify(testNow); err == nil {
		t.Fatal("accepted noncanonical approval")
	}
}
func TestSignerCredentialIsolation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "credentials")
	writeTest(t, file, []byte("[selected]\naws_access_key_id = synthetic\naws_secret_access_key = synthetic\n"))
	client, err := NewKMSSigner("us-east-1", file, "selected")
	if err != nil || client == nil {
		t.Fatal(err)
	}
	writeTest(t, file, []byte("[selected]\ncredential_process = forbidden\n"))
	if _, err = NewKMSSigner("us-east-1", file, "selected"); err == nil {
		t.Fatal("accepted process credentials")
	}
	if _, err = NewKMSSigner("us-east-1", file, ""); err == nil {
		t.Fatal("accepted ambient profile")
	}
}
