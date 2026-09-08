//go:build !unix

package gbrain

import (
	"fmt"
	"os"
)

func lockImport(_ *os.File) error {
	return fmt.Errorf("gbrain import: checkpoint locking requires a supported Unix platform")
}
