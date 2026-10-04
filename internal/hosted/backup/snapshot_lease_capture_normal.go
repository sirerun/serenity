//go:build !hostedtest

package backup

import "os"

// snapshotLeaseCaptureTemp is an ordinary-build no-op. The hostedtest build
// supplies the same private function so crash fixtures can capture the exact
// canonical payload and inode tuple at the real temp-file creation boundary.
func snapshotLeaseCaptureTemp(path, name string, record leaseDiskRecord, raw []byte, root *os.Root, file *os.File) {
}
