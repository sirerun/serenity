//go:build darwin

package runtimebroker

import (
	"io/fs"
	"syscall"
)

func sameInventoryCustody(a, b fs.FileInfo) bool {
	as, aok := a.Sys().(*syscall.Stat_t)
	bs, bok := b.Sys().(*syscall.Stat_t)
	return aok && bok && as.Uid == bs.Uid && as.Gid == bs.Gid && as.Nlink == bs.Nlink && as.Ctimespec == bs.Ctimespec
}
