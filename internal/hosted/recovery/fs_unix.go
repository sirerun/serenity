//go:build darwin || linux

package recovery

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func privateDirectoryPath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: directory path must be absolute", ErrPlanUntrustedDir)
	}
	requested := filepath.Clean(path)
	requestedInfo, err := os.Lstat(requested)
	if err != nil || !requestedInfo.IsDir() || requestedInfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%w: plan directory must be a real directory", ErrPlanUntrustedDir)
	}
	clean, err := filepath.EvalSymlinks(requested)
	if err != nil {
		return "", fmt.Errorf("%w: directory path cannot be resolved", ErrPlanUntrustedDir)
	}
	clean = filepath.Clean(clean)
	volume := filepath.VolumeName(clean)
	root := volume + string(filepath.Separator)
	if volume == "" {
		root = string(filepath.Separator)
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%w: unsafe filesystem root", ErrPlanUntrustedDir)
	}
	if err = validateTrustedAncestor(root, info); err != nil {
		return "", err
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(clean, volume), string(filepath.Separator))
	current := root
	if rest != "" {
		for _, component := range strings.Split(rest, string(filepath.Separator)) {
			if component == "" || component == "." || component == ".." {
				return "", fmt.Errorf("%w: invalid directory component", ErrPlanUntrustedDir)
			}
			current = filepath.Join(current, component)
			info, err = os.Lstat(current)
			if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("%w: unsafe directory component", ErrPlanUntrustedDir)
			}
			if err = validateTrustedAncestor(current, info); err != nil {
				return "", err
			}
		}
	}
	if info.Mode().Perm() != 0700 {
		return "", fmt.Errorf("%w: directory mode %04o must be 0700", ErrPlanUntrustedDir, info.Mode().Perm())
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return "", fmt.Errorf("%w: directory is not owned by the current user", ErrPlanUntrustedDir)
	}
	return clean, nil
}

func validateTrustedAncestor(path string, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || (int(stat.Uid) != 0 && int(stat.Uid) != os.Geteuid()) {
		return fmt.Errorf("%w: path ancestor %q is not owned by root or the current user", ErrPlanUntrustedDir, path)
	}
	if info.Mode().Perm()&0022 != 0 && info.Mode()&os.ModeSticky == 0 {
		return fmt.Errorf("%w: path ancestor %q is group/world writable without sticky protection", ErrPlanUntrustedDir, path)
	}
	if err := verifyFilesystemOwnership(path); err != nil {
		return err
	}
	return nil
}

func openPlanNoFollow(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	var stat unix.Stat_t
	if err = unix.Fstat(fd, &stat); err != nil {
		_ = file.Close()
		return nil, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || int(stat.Uid) != os.Geteuid() || stat.Mode&0077 != 0 {
		_ = file.Close()
		return nil, errors.New("plan file is not a private, owned regular file")
	}
	return file, nil
}

func linkPlanNoReplace(source, target string) error {
	return os.Link(source, target)
}

func syncPlanDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
