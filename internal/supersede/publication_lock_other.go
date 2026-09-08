//go:build !darwin && !linux

package supersede

import (
	"fmt"
	"os"
)

func lockPublication(*os.Root) (*os.File, error) {
	return nil, fmt.Errorf("reconcile publication requires macOS or Linux file locking")
}
