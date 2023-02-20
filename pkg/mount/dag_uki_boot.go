package mount

import (
	cnst "github.com/kairos-io/immucore/internal/constants"
	"github.com/spectrocloud-labs/herd"
)

// RegisterUKI registers the dag for booting from UKI
func (s *State) RegisterUKI(g *herd.Graph) error {
	// Write sentinel
	s.LogIfError(s.WriteSentinelDagStep(g), "sentinel")
	// Run rootfs stage
	s.LogIfError(s.RootfsStageDagStep(g, cnst.OpSentinel), "uki rootfs")
	// run initramfs stage
	s.LogIfError(s.InitramfsStageDagStep(g, cnst.OpRootfsHook), "uki initramfs")
	// Handover to /sbin/init
	_ = s.UKIBootInitDagStep(g, cnst.OpRootfsHook, cnst.OpInitramfsHook)
	return nil
}
