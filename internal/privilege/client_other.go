//go:build !linux

package privilege

import (
	"context"
	"errors"
)

func Apply(context.Context, ApplyRequest) (ApplyResponse, error) {
	return ApplyResponse{}, errors.New("privileged nginx mutations are supported on Linux servers only")
}

func requireRoot() error {
	return errors.New("privileged apply is supported on Linux only")
}

func RefuseRootServer() error {
	return nil
}
