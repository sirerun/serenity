//go:build !linux

package main

import "io"

func snapshotCLI([]string, io.Writer) int { return 1 }
