//go:build !darwin && !linux

package episodeinstall

func prepareCapacity(string, int64) error { return ErrUnqualified }
