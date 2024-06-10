package dag

import (
	cnst "github.com/kairos-io/immucore/internal/constants"
	"github.com/kairos-io/immucore/pkg/state"
	"github.com/spectrocloud-labs/herd"
)

// RegisterUKI registers the dag for booting from UKI.
// This needs to set the full system and mount the final rootfs.
// We dont really pivot into it, we mount everything under /sysroot then move
// it to be the new / and chroot into it.
// Then we handover /sbin/init (systemd).
func RegisterUKI(s *state.State, g *herd.Graph) error {
	// Mount basic mounts
	s.LogIfError(s.UKIMountBaseSystem(g), "mounting base mounts")

	// Move to sysroot
	s.LogIfError(s.UkiPivotToSysroot(g), "pivot to sysroot")

	// Write sentinel
	s.LogIfError(s.WriteSentinelDagStep(g, cnst.OpUkiBaseMounts), "sentinel")

	// Load needed kernel modules
	// TODO: This seems to be wrong as it leans on the udev to infer the modules, but at this point we dont have udev
	// So we dont get all the proper modules needed!
	s.LogIfError(s.UKILoadKernelModules(g), "kernel modules")

	// Udev for devices discovery
	s.LogIfError(s.UKIUdevDaemon(g), "udev")

	// Mount ESP partition under efi if it exists
	s.LogIfError(s.UKIMountESPPartition(g, herd.WithDeps(cnst.OpSentinel, cnst.OpUkiUdev)), "mount ESP partition")

	// Extract EFI public certs for sysextensions validation
	s.LogIfError(s.ExtractCerts(g, herd.WithDeps(cnst.OpSentinel, cnst.OpUkiUdev)), "extract certs")

	// Mount cdrom under /run/initramfs/livecd and /run/rootfsbase for the efiboot.img contents
	s.LogIfError(s.UKIMountLiveCd(g, herd.WithDeps(cnst.OpSentinel, cnst.OpUkiUdev)), "Mount LiveCD")

	// Unlock partitions if needed with TPM
	s.LogIfError(s.UKIUnlock(g, herd.WithDeps(cnst.OpSentinel, cnst.OpUkiUdev)), "uki unlock")

	s.LogIfError(s.MountOemDagStep(g, herd.WithDeps(cnst.OpUkiKcrypt), herd.WeakDeps), "oem mount")

	// Run rootfs stage
	s.LogIfError(s.RootfsStageDagStep(g, herd.WithDeps(cnst.OpSentinel, cnst.OpUkiUdev, cnst.OpMountOEM), herd.WithWeakDeps(cnst.OpUkiMountLivecd)), "uki rootfs")

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

	// Copy any sysextensions found under cnst.SourceSysExtDir into cnst.DestSysExtDir so its loaded by systemd automatically on start
	// always after cnst.OpMountBind stage so we have a persistent cnst.DestSysExtDir
	// Note that the loading of the extensions is done by systemd with the systemd-sysext service
	s.LogIfError(s.CopySysExtensionsDagStep(g, herd.WithDeps(cnst.OpMountBind)), "copy sysextensions")

	// run initramfs stage
	s.LogIfError(s.InitramfsStageDagStep(g, herd.WeakDeps, herd.WithDeps(cnst.OpMountBind, cnst.OpUkiCopySysExtensions)), "uki initramfs")

	s.LogIfError(s.WriteFstabDagStep(g,
		herd.WithDeps(cnst.OpLoadConfig, cnst.OpCustomMounts, cnst.OpMountBind, cnst.OpOverlayMount),
	), "fstab")

	// Handover to /sbin/init
	_ = s.UKIBootInitDagStep(g)
	return nil
}
