//go:build !linux

package runtimebroker

import (
	"context"
	"errors"
	"net"
)

func Serve(context.Context, Config) error {
	return errors.New("broker: Linux process authority required")
}
func dialBroker(context.Context, string) (net.Conn, error) {
	return nil, errors.New("broker: Linux process authority required")
}
