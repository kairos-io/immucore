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

	// Try to mount oem ONLY if we are on recovery squash
	// The check to see if its enabled its on the DAG step itself
	s.LogIfError(s.MountOemDagStep(g, cnst.OpWaitForSysroot), "oem mount")

	s.LogIfError(s.RootfsStageDagStep(g, herd.WithDeps(cnst.OpSentinel, cnst.OpWaitForSysroot), herd.WithWeakDeps(cnst.OpMountOEM)), "rootfs stage")
	// Run initramfs inside the /sysroot chroot!
	s.LogIfError(s.InitramfsStageDagStep(g, herd.WithDeps(cnst.OpSentinel, cnst.OpWaitForSysroot, cnst.OpRootfsHook), herd.WithWeakDeps(cnst.OpMountOEM)), "initramfs stage")
	return err
}
