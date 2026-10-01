//go:build darwin

package recovery

import (
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestVerifyFilesystemOwnershipRejectsDisabledMount(t *testing.T) {
	path := os.Getenv("SERENITY_RECOVERY_UNOWNED_TEST_PATH")
	if path == "" {
		t.Skip("no ownership-disabled filesystem supplied")
	}
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		t.Fatal(err)
	}
	if stat.Flags&unix.MNT_IGNORE_OWNERSHIP == 0 {
		t.Skip("supplied filesystem honors ownership")
	}
	if err := verifyFilesystemOwnership(path); !errors.Is(err, ErrPlanUntrustedDir) {
		t.Fatalf("verifyFilesystemOwnership error = %v, want ErrPlanUntrustedDir", err)
	}
}
