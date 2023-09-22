package mount

import (
	cnst "github.com/kairos-io/immucore/internal/constants"
	internalUtils "github.com/kairos-io/immucore/internal/utils"
	"github.com/spectrocloud-labs/herd"
)

// RegisterUKI registers the dag for booting from UKI.
func (s *State) RegisterUKI(g *herd.Graph) error {
	// Mount basic mounts
	s.LogIfError(s.UKIMountBaseSystem(g), "mounting base mounts")

	// Write sentinel
	s.LogIfError(s.WriteSentinelDagStep(g, cnst.OpUkiBaseMounts), "sentinel")

	// Load needed kernel modules
	// TODO: This seems to be wrong as it leans on the udev to infer the modules, but at this point we dont have udev
	// So we dont get all the proper modules needed!
	s.LogIfError(s.LoadKernelModules(g), "kernel modules")

	// Udev for devices discovery
	s.LogIfError(s.UKIUdevDaemon(g), "udev")

	// Mount ESP partition under efi if it exists
	s.LogIfError(s.MountESPPartition(g, herd.EnableIf(func() bool {
		return internalUtils.CheckEfiPartUUID() == nil
	}), herd.WithDeps(cnst.OpSentinel, cnst.OpUkiUdev)), "mount ESP partition")

	// Run rootfs stage
	s.LogIfError(s.RootfsStageDagStep(g, herd.WithDeps(cnst.OpSentinel, cnst.OpUkiUdev)), "uki rootfs")

	// Remount root RO
	s.LogIfError(s.UKIRemountRootRODagStep(g), "remount root")

	s.LogIfError(s.MountOemDagStep(g, herd.WithDeps(cnst.OpRemountRootRO), herd.WeakDeps), "oem mount")

	// Populate state bind mounts, overlay mounts, custom-mounts from /run/cos/cos-layout.env
	// Requires stage rootfs to have run, which usually creates the cos-layout.env file
	s.LogIfError(s.LoadEnvLayoutDagStep(g), "loading cos-layout.env")

	// Mount base overlay under /run/overlay
	s.LogIfError(s.MountBaseOverlayDagStep(g), "base overlay")

	// Mount custom overlays loaded from the /run/cos/cos-layout.env file
	s.LogIfError(s.MountCustomOverlayDagStep(g, herd.WeakDeps), "custom overlays mount")

	// Mount custom mounts loaded from the /run/cos/cos-layout.env file
	s.LogIfError(s.MountCustomMountsDagStep(g, herd.WeakDeps), "custom mounts mount")

	// Mount custom binds loaded from the /run/cos/cos-layout.env file
	// Depends on mount binds as that usually mounts COS_PERSISTENT
	s.LogIfError(s.MountCustomBindsDagStep(g, herd.WeakDeps), "custom binds mount")

	// run initramfs stage
	s.LogIfError(s.InitramfsStageDagStep(g, herd.WeakDeps, herd.WithDeps(cnst.OpMountBind)), "uki initramfs")

	s.LogIfError(g.Add(cnst.OpWriteFstab,
		herd.WithDeps(cnst.OpLoadConfig, cnst.OpCustomMounts, cnst.OpMountBind, cnst.OpOverlayMount),
		herd.WeakDeps,
		herd.WithCallback(s.WriteFstab(s.path("/etc/fstab")))), "fstab")

	// Handover to /sbin/init
	_ = s.UKIBootInitDagStep(g)
	return nil
}
