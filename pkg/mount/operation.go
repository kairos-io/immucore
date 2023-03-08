package mount

import (
	"github.com/containerd/containerd/mount"
	"github.com/deniswernert/go-fstab"
	"github.com/kairos-io/immucore/internal/constants"
	internalUtils "github.com/kairos-io/immucore/internal/utils"
	"github.com/moby/sys/mountinfo"
	"github.com/rs/zerolog"
)

type mountOperation struct {
	FstabEntry      fstab.Mount
	MountOption     mount.Mount
	Target          string
	PrepareCallback func() error
}

func (m mountOperation) run() error {
	// Add context to sublogger
	l := internalUtils.Log.With().Str("what", m.MountOption.Source).Str("where", m.Target).Str("type", m.MountOption.Type).Strs("options", m.MountOption.Options).Logger().Level(zerolog.InfoLevel)
	// Not sure why this defaults to debuglevel when creating a sublogger, so make sure we set it properly
	debug := len(internalUtils.ReadCMDLineArg("rd.immucore.debug")) > 0
	if debug {
		l.Level(zerolog.DebugLevel)
	}

	if m.PrepareCallback != nil {
		if err := m.PrepareCallback(); err != nil {
			l.Err(err).Msg("executing mount callback")
			return err
		}
	}
	//TODO: not only check if mounted but also if the type,options and source are the same?
	mounted, err := mountinfo.Mounted(m.Target)
	if err != nil {
		l.Err(err).Msg("checking mount status")
		return err
	}
	if mounted {
		l.Debug().Msg("Already mounted")
		return constants.ErrAlreadyMounted
	}
	l.Debug().Msg("mount ready")
	return mount.All([]mount.Mount{m.MountOption}, m.Target)
}
