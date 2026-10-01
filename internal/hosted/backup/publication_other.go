//go:build !darwin && !linux

package backup

func renameNoReplace(oldPath, newPath string) error {
	return publicationRenameError(oldPath, newPath, ErrAtomicPublicationUnavailable)
}
