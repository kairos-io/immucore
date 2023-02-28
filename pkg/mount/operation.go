package mount

import (
	"github.com/containerd/containerd/mount"
	"github.com/deniswernert/go-fstab"
	"github.com/kairos-io/immucore/internal/constants"
	internalUtils "github.com/kairos-io/immucore/internal/utils"
	"github.com/moby/sys/mountinfo"
)

type mountOperation struct {
	FstabEntry      fstab.Mount
	MountOption     mount.Mount
	Target          string
	PrepareCallback func() error
}

func (m mountOperation) run() error {
	internalUtils.Log.With().Str("what", m.MountOption.Source).Str("where", m.Target).Str("type", m.MountOption.Type).Strs("options", m.MountOption.Options).Logger()
	if m.PrepareCallback != nil {
		if err := m.PrepareCallback(); err != nil {
			internalUtils.Log.Err(err).Msg("executing mount callback")
			return err
		}
	}
	//TODO: not only check if mounted but also if the type,options and source are the same?
	mounted, err := mountinfo.Mounted(m.Target)
	if err != nil {
		internalUtils.Log.Err(err).Msg("checking mount status")
		return err
	}
	if mounted {
		internalUtils.Log.Debug().Msg("Already mounted")
		return constants.ErrAlreadyMounted
	}
	internalUtils.Log.Debug().Msg("mount ready")
	return mount.All([]mount.Mount{m.MountOption}, m.Target)
}
