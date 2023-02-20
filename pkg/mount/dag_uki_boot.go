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
	s.LogIfError(s.InitramfsStageDagStep(g, cnst.OpSentinel, cnst.OpRootfsHook), "uki initramfs")

	// Remount root RO
	s.LogIfError(s.UKIRemountRootRODagStep(g, cnst.OpInitramfsHook, cnst.OpRootfsHook), "remount root")

	// Mount base overlay under /run/overlay
	s.LogIfError(s.MountBaseOverlayDagStep(g), "base overlay")

	// Populate state bind mounts, overlay mounts, custom-mounts from /run/cos/cos-layout.env
	// Requires stage rootfs to have run, which usually creates the cos-layout.env file
	s.LogIfError(s.LoadEnvLayoutDagStep(g, cnst.OpRootfsHook), "loading cos-layout.env")

	// Mount custom overlays loaded from the /run/cos/cos-layout.env file
	s.LogIfError(s.MountCustomOverlayDagStep(g), "custom overlays mount")

	// Mount custom mounts loaded from the /run/cos/cos-layout.env file
	s.LogIfError(s.MountCustomMountsDagStep(g), "custom mounts mount")

	// Mount custom binds loaded from the /run/cos/cos-layout.env file
	// Depends on mount binds as that usually mounts COS_PERSISTENT
	s.LogIfError(s.MountCustomBindsDagStep(g), "custom binds mount")

	// Handover to /sbin/init
	_ = s.UKIBootInitDagStep(g, cnst.OpRemountRootRO, cnst.OpRootfsHook, cnst.OpInitramfsHook)
	return nil
}
