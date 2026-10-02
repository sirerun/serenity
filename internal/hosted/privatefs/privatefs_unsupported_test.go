//go:build !darwin && !linux

package privatefs

import (
	"context"
	"errors"
	"testing"
)

func TestUnsupportedPlatformFailsClosed(t *testing.T) {
	if err := ValidateDirectory(context.Background(), "/private"); !errors.Is(err, errUnsupported) {
		t.Fatalf("ValidateDirectory = %v", err)
	}
	if _, err := ReadFile(context.Background(), "/private", "case.json", 10); !errors.Is(err, errUnsupported) {
		t.Fatalf("ReadFile = %v", err)
	}
}
