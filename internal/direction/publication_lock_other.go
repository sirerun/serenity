//go:build !darwin && !linux

package direction

import (
	"fmt"
	"os"
)

func lockPublication(*os.Root) (*os.File, error) {
	return nil, fmt.Errorf("direction publication requires macOS or Linux file locking")
}
