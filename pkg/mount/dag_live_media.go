package mount

import (
	internalUtils "github.com/kairos-io/immucore/internal/utils"
	"github.com/spectrocloud-labs/herd"
)

// RegisterLiveMedia registers the dag for booting from live media/netboot
// This sets the sentinel
func (s *State) RegisterLiveMedia(g *herd.Graph) error {
	var err error
	// Maybe LogIfErrorAndPanic ? If no sentinel, a lot of config files are not going to run
	err = s.LogIfErrorAndReturn(s.WriteSentinelDagStep(g), "write sentinel")
	internalUtils.CloseLogFiles()
	return err
}
