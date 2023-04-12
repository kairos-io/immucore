package mount

import (
	cnst "github.com/kairos-io/immucore/internal/constants"
	"github.com/spectrocloud-labs/herd"
)

// RegisterUKI registers the dag for booting from UKI.
func (s *State) RegisterUKI(g *herd.Graph) error {
	// Mount basic mounts
	s.LogIfError(s.UKIMountBaseSystem(g), "mounting base mounts")

	// Write sentinel
	s.LogIfError(s.WriteSentinelDagStep(g, cnst.OpUkiBaseMounts), "sentinel")

	// Load needed kernel modules
	s.LogIfError(s.LoadKernelModules(g), "kernel modules")

	// Udev for devices discovery
	s.LogIfError(s.UKIUdevDaemon(g), "udev")

	// Run rootfs stage
	s.LogIfError(s.RootfsStageDagStep(g, herd.WithDeps(cnst.OpSentinel, cnst.OpUkiUdev)), "uki rootfs")

	// Remount root RO
	s.LogIfError(s.UKIRemountRootRODagStep(g), "remount root")

	// Populate state bind mounts, overlay mounts, custom-mounts from /run/cos/cos-layout.env
	// Requires stage rootfs to have run, which usually creates the cos-layout.env file
	s.LogIfError(s.LoadEnvLayoutDagStep(g, cnst.OpRootfsHook), "loading cos-layout.env")

	// Mount base overlay under /run/overlay
	s.LogIfError(s.MountBaseOverlayDagStep(g), "base overlay")

	// Mount custom overlays loaded from the /run/cos/cos-layout.env file
	s.LogIfError(s.MountCustomOverlayDagStep(g), "custom overlays mount")

	// Mount custom mounts loaded from the /run/cos/cos-layout.env file
	s.LogIfError(s.MountCustomMountsDagStep(g), "custom mounts mount")

	// Mount custom binds loaded from the /run/cos/cos-layout.env file
	// Depends on mount binds as that usually mounts COS_PERSISTENT
	s.LogIfError(s.MountCustomBindsDagStep(g), "custom binds mount")

	// run initramfs stage
	s.LogIfError(s.InitramfsStageDagStep(g, herd.WithDeps(cnst.OpMountBind)), "uki initramfs")

	s.LogIfError(g.Add(cnst.OpWriteFstab,
		herd.WithDeps(cnst.OpLoadConfig, cnst.OpCustomMounts, cnst.OpMountBind, cnst.OpOverlayMount),
		herd.WeakDeps,
		herd.WithCallback(s.WriteFstab(s.path("/etc/fstab")))), "fstab")

	// Handover to /sbin/init
	_ = s.UKIBootInitDagStep(g)
	return nil
}
