package mount

import (
	"github.com/containerd/containerd/mount"
	"github.com/deniswernert/go-fstab"
)

type mountOperation struct {
	FstabEntry      fstab.Mount
	MountOption     mount.Mount
	Target          string
	PrepareCallback func() error
}

func (m mountOperation) run() error {
	if m.PrepareCallback != nil {
		if err := m.PrepareCallback(); err != nil {
			return err
		}
	}

	return mount.All([]mount.Mount{m.MountOption}, m.Target)
}
