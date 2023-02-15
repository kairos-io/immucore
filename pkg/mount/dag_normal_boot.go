package mount

import (
	cnst "github.com/kairos-io/immucore/internal/constants"
	"github.com/spectrocloud-labs/herd"
)

// RegisterNormalBoot registers a dag for a normal boot, where we want to mount all the pieces that make up the
// final system. This mounts root, oem, runs rootfs, loads the cos-layout.env file and mounts custom stuff from that file
// and finally writes the fstab.
// This is all done on initramfs, very early, and ends up pivoting to the final system, usually under /sysroot
func (s *State) RegisterNormalBoot(g *herd.Graph) error {
	var err error

	// TODO: add hooks, fstab (might have missed some), systemd compat, fsck
	s.LogIfError(s.MountTmpfsDagStep(g), "tmpfs mount")

	// Mount Root (COS_STATE or COS_RECOVERY and then the image active/passive/recovery under s.Rootdir)
	s.LogIfError(s.MountRootDagStep(g), "running mount root stage")

	// Mount COS_OEM (After root as it mounts under s.Rootdir/oem)
	s.LogIfError(s.MountOemDagStep(g, cnst.OpMountRoot), "oem mount")

	// Run yip stage rootfs. Requires root+oem to be mounted
	s.LogIfError(s.RootfsStageDagStep(g, cnst.OpMountRoot, cnst.OpMountOEM), "running rootfs stage")

	// Populate state bindmounts, overlaymounts, custommounts from /run/cos/cos-layout.env
	// Requires stage rootfs to have run, which usually creates the cos-layout.env file
	s.LogIfError(s.LoadEnvLayoutDagStep(g), "loading cos-layout.env")

	// Mount base overlay under /run/overlay
	s.LogIfError(s.MountBaseOverlayDagStep(g), "base overlay mount")

	// Note(Itxaka): This was a dependency for overlayMount, opCustomMounts and opMountBind steps
	// But I don't see how the s.Rootdir could ever be an overlay as we mount COS_STATE on it
	// overlayCondition := herd.ConditionalOption(func() bool { return internalUtils.DiskFSType(s.Rootdir) != "overlay" }, herd.WithDeps(opMountBaseOverlay))

	// Mount custom overlays loaded from the /run/cos/cos-layout.env file
	s.LogIfError(s.MountCustomOverlayDagStep(g), "custom overlays mount")

	s.LogIfError(s.MountCustomMountsDagStep(g), "custom mounts mount")

	// Mount custom binds loaded from the /run/cos/cos-layout.env file
	// Depends on mount binds as that usually mounts COS_PERSISTENT
	s.LogIfError(s.MountCustomBindsDagStep(g), "custom binds mount")

	// Write fstab file
	s.LogIfError(s.WriteFstabDagStep(g), "write fstab")

	return err
}
