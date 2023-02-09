package mount

import (
	"github.com/containerd/containerd/mount"
	"github.com/deniswernert/go-fstab"
	"github.com/kairos-io/immucore/internal/constants"
	"github.com/moby/sys/mountinfo"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
)

type mountOperation struct {
	FstabEntry      fstab.Mount
	MountOption     mount.Mount
	Target          string
	PrepareCallback func() error
}

func (m mountOperation) run() error {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
	if m.PrepareCallback != nil {
		if err := m.PrepareCallback(); err != nil {
			log.Logger.Err(err).Str("what", m.MountOption.Source).Str("where", m.Target).Str("type", m.MountOption.Type).Strs("options", m.MountOption.Options).Msg("executing mount callback")
			return err
		}
	}
	//TODO: not only check if mounted but also if the type,options and source are the same?
	mounted, err := mountinfo.Mounted(m.Target)
	if err != nil {
		log.Logger.Err(err).Str("what", m.MountOption.Source).Str("where", m.Target).Str("type", m.MountOption.Type).Strs("options", m.MountOption.Options).Msg("checking mount status")
		return err
	}
	if mounted {
		log.Logger.Debug().Str("what", m.MountOption.Source).Str("where", m.Target).Str("type", m.MountOption.Type).Strs("options", m.MountOption.Options).Msg("Already mounted")
		return constants.ErrAlreadyMounted
	}
	return mount.All([]mount.Mount{m.MountOption}, m.Target)
}
