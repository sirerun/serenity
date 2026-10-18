//go:build !linux && !darwin

package episodeinstall

func renameExclusive(string, string) error { return ErrIncomplete }
