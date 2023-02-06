package utils

import (
	"github.com/kairos-io/kairos/sdk/state"
	"github.com/twpayne/go-vfs"
)

func BootedFromCD(fs vfs.FS) (bool, error) {
	runtime, err := state.NewRuntime()
	if err != nil {
		return false, err
	}

	return runtime.BootState == state.LiveCD, nil
}
