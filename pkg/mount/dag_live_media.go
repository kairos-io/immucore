package mount

import (
	"github.com/spectrocloud-labs/herd"
)

// RegisterLiveMedia registers the dag for booting from live media/netboot
// This mounts /tmp
func (s *State) RegisterLiveMedia(g *herd.Graph) error {
	var err error
	s.LogIfError(s.MountTmpfsDagStep(g), "tmpfs mount")
	// Maybe LogIfErrorAndPanic ? If no sentinel, a lot of config files are not going to run
	err = s.LogIfErrorAndReturn(s.WriteSentinelDagStep(g), "write sentinel")
	return err
}
