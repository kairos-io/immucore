package mount

import (
	cnst "github.com/kairos-io/immucore/internal/constants"
	"github.com/spectrocloud-labs/herd"
)

// RegisterLiveMedia registers the dag for booting from live media/netboot
// This sets the sentinel.
func (s *State) RegisterLiveMedia(g *herd.Graph) error {
	// Maybe LogIfErrorAndPanic ? If no sentinel, a lot of config files are not going to run
	err := s.LogIfErrorAndReturn(s.WriteSentinelDagStep(g), "write sentinel")

	// Waits for sysroot to be there, just in case
	s.LogIfError(s.WaitForSysrootDagStep(g), "Waiting for sysroot")
	// Run rootfs
	s.LogIfError(s.RootfsStageDagStep(g, cnst.OpSentinel, cnst.OpWaitForSysroot), "rootfs stage")
	// Run initramfs inside the /sysroot chroot!
	s.LogIfError(s.InitramfsStageDagStep(g, herd.WithDeps(cnst.OpSentinel, cnst.OpWaitForSysroot, cnst.OpRootfsHook), herd.WithWeakDeps()), "initramfs stage")
	return err
}
