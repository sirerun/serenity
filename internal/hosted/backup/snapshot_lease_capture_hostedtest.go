//go:build hostedtest

package backup

import (
	"os"
	"sync"
)

type snapshotLeaseTempCapture struct {
	Kind       string
	Path       string
	Name       string
	Raw        []byte
	Record     leaseDiskRecord
	DirDevice  uint64
	DirInode   uint64
	TempDevice uint64
	TempInode  uint64
}

var snapshotLeaseTempCaptureMu sync.Mutex
var snapshotLeaseTempCaptureObserver func(snapshotLeaseTempCapture)

func setSnapshotLeaseTempCaptureObserverForTest(observer func(snapshotLeaseTempCapture)) func() {
	snapshotLeaseTempCaptureMu.Lock()
	previous := snapshotLeaseTempCaptureObserver
	snapshotLeaseTempCaptureObserver = observer
	snapshotLeaseTempCaptureMu.Unlock()
	return func() {
		snapshotLeaseTempCaptureMu.Lock()
		snapshotLeaseTempCaptureObserver = previous
		snapshotLeaseTempCaptureMu.Unlock()
	}
}

func snapshotLeaseCaptureTemp(path, name string, record leaseDiskRecord, raw []byte, root *os.Root, file *os.File) {
	snapshotLeaseTempCaptureMu.Lock()
	observer := snapshotLeaseTempCaptureObserver
	snapshotLeaseTempCaptureMu.Unlock()
	if observer == nil {
		return
	}
	dirInfo, err := root.Stat(".")
	if err != nil {
		return
	}
	tempInfo, err := file.Stat()
	if err != nil {
		return
	}
	dirDevice, dirInode, err := fileIdentity(dirInfo)
	if err != nil {
		return
	}
	tempDevice, tempInode, err := fileIdentity(tempInfo)
	if err != nil {
		return
	}
	kind := "lease"
	if len(name) >= len(".release.tmp-") && name[:len(".release.tmp-")] == ".release.tmp-" {
		kind = "tombstone"
	}
	observer(snapshotLeaseTempCapture{Kind: kind, Path: path, Name: name, Raw: append([]byte(nil), raw...), Record: record, DirDevice: dirDevice, DirInode: dirInode, TempDevice: tempDevice, TempInode: tempInode})
}
