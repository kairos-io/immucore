package mount

import (
	cnst "github.com/kairos-io/immucore/internal/constants"
	"github.com/spectrocloud-labs/herd"
)

// RegisterUKI registers the dag for booting from UKI
func (s *State) RegisterUKI(g *herd.Graph) error {
	s.LogIfError(s.RootfsStageDagStep(g), "uki rootfs")
	s.LogIfError(s.InitramfsStageDagStep(g, cnst.OpRootfsHook), "uki rootfs")
	_ = s.UKIBootInitDagStep(g, cnst.OpRootfsHook, cnst.OpInitramfsHook)
	return nil
}
