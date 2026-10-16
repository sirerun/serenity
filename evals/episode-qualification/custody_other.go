//go:build !linux

package main

import "errors"

func executeCustody([]string) error { return errors.New("Linux offline custody required") }
