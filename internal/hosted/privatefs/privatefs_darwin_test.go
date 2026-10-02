//go:build darwin

package privatefs

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestOwnershipDisabledMountRejected(t *testing.T) {
	path := os.Getenv("SERENITY_PRIVATEFS_UNOWNED_TEST_PATH")
	if path == "" {
		t.Skip("SERENITY_PRIVATEFS_UNOWNED_TEST_PATH is unset; ownership-disabled-mount control not run")
	}
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		t.Fatalf("inspect configured ownership-disabled mount %q: %v", path, err)
	}
	if fs.Flags&unix.MNT_IGNORE_OWNERSHIP == 0 {
		t.Skipf("configured path %q does not expose MNT_IGNORE_OWNERSHIP; ownership-disabled-mount control not qualified", path)
	}
	if err := ownershipEnforced(path); err == nil {
		t.Fatal("ownership-disabled mount accepted")
	}
}
