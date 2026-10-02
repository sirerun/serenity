//go:build darwin || linux

package backup

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func openSnapshotRegular(root *os.Root, name string) (*os.File, error) {
	if name == "" || name == "." || name == ".." || name != filepathBase(name) {
		return nil, errors.New("hosted/backup: snapshot artifact must be one path element")
	}
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("snapshot entry is not a regular file: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errors.New("hosted/backup: snapshot entry is not a regular file")
	}
	return file, nil
}

func filepathBase(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' || name[i] == '\\' {
			return name[i+1:]
		}
	}
	return name
}
