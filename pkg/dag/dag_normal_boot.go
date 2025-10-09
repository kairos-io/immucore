package dag

import (
	cnst "github.com/kairos-io/immucore/internal/constants"
	"github.com/kairos-io/immucore/pkg/state"
	"github.com/spectrocloud-labs/herd"
)

// RegisterNormalBoot registers a dag for a normal boot, where we want to mount all the pieces that make up the
// final system. This mounts root, oem, runs rootfs, loads the cos-layout.env file and mounts custom stuff from that file
// and finally writes the fstab.
// This is all done on initramfs, very early, and ends up pivoting to the final system, usually under /sysroot.
func RegisterNormalBoot(s *state.State, g *herd.Graph) error {
	var err error

	s.LogIfError(s.LVMActivation(g), "lvm activation")

	// Maybe LogIfErrorAndPanic ? If no sentinel, a lot of config files are not going to run
	if err = s.LogIfErrorAndReturn(s.WriteSentinelDagStep(g), "write sentinel"); err != nil {
		return err
	}

	s.LogIfError(s.MountTmpfsDagStep(g), "tmpfs mount")

	// Mount Root (COS_STATE or COS_RECOVERY and then the image active/passive/recovery under s.Rootdir)
	s.LogIfError(s.MountRootDagStep(g), "running mount root stage")

	// Upgrade kcrypt partitions to kcrypt 0.6.0 if any
	// Depend on LVM in case the LVM is encrypted somehow? Not sure if possible.
	s.LogIfError(s.RunKcryptUpgrade(g, herd.WithDeps(cnst.OpLvmActivate)), "upgrade kcrypt partitions")

	// Mount COS_OEM (After root as it mounts under s.Rootdir/oem)
	// NOTE: We solved this with the cmdline now
	//s.LogIfError(s.MountOemDagStep(g, herd.WithDeps(cnst.OpMountRoot, cnst.OpLvmActivate)), "oem mount")

	// Run unlock.
	// Depends on mount root because it needs the kcrypt-discovery-challenger available under /sysroot
	// Depends on OpKcryptUpgrade until we don't support upgrading from 1.X to the current version
	// Depends on mount oem to read the server configuration - Not anymore
	s.LogIfError(s.RunKcrypt(g, herd.WithDeps(cnst.OpMountRoot, cnst.OpKcryptUpgrade)), "kcrypt unlock")

	// Run yip stage rootfs. Requires root+oem+sentinel to be mounted
	s.LogIfError(s.RootfsStageDagStep(g, herd.WithDeps(cnst.OpMountRoot, cnst.OpMountOEM, cnst.OpSentinel)), "running rootfs stage")

	// Populate state bind mounts, overlay mounts, custom-mounts from /run/cos/cos-layout.env
	// Requires stage rootfs to have run, which usually creates the cos-layout.env file
	s.LogIfError(s.LoadEnvLayoutDagStep(g), "loading cos-layout.env")

	// Mount base overlay under /run/overlay
	s.LogIfError(s.MountBaseOverlayDagStep(g), "base overlay mount")

	// Mount custom overlays loaded from the /run/cos/cos-layout.env file
	s.LogIfError(s.MountCustomOverlayDagStep(g), "custom overlays mount")

	s.LogIfError(s.MountCustomMountsDagStep(g), "custom mounts mount")

	// Mount custom binds loaded from the /run/cos/cos-layout.env file
	// Depends on mount binds as that usually mounts COS_PERSISTENT
	s.LogIfError(s.MountCustomBindsDagStep(g), "custom binds mount")

	//
	s.LogIfError(s.EnableSysExtensions(g, herd.WithWeakDeps(cnst.OpMountBind)), "enable sysextensions")

	// Write fstab file
	s.LogIfError(s.WriteFstabDagStep(g,
		herd.WithDeps(cnst.OpMountRoot, cnst.OpDiscoverState, cnst.OpLoadConfig),
		herd.WithWeakDeps(cnst.OpKcryptUnlock, cnst.OpMountOEM, cnst.OpCustomMounts, cnst.OpMountBind, cnst.OpOverlayMount)), "write fstab")

	// do it after fstab is created
	s.LogIfError(s.InitramfsStageDagStep(g,
		herd.WithDeps(cnst.OpMountRoot, cnst.OpDiscoverState, cnst.OpLoadConfig, cnst.OpWriteFstab),
		herd.WithWeakDeps(cnst.OpMountBaseOverlay, cnst.OpKcryptUnlock, cnst.OpMountOEM, cnst.OpMountBind, cnst.OpMountBind, cnst.OpCustomMounts, cnst.OpOverlayMount),
	), "initramfs stage")
	return err
}
