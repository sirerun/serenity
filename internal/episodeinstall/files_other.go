//go:build !unix

package episodeinstall

import "os"

func ownedInfo(os.FileInfo) bool  { return false }
func singleLink(os.FileInfo) bool { return false }
