package constants

import "errors"

func DefaultRWPaths() []string {
	// Default RW_PATHS to mount if not override by the cos-layout.env file
	return []string{"/etc", "/root", "/home", "/opt", "/srv", "/usr/local", "/var"}
}

func GetCloudInitPaths() []string {
	return []string{"/system/oem", "/oem/", "/usr/local/cloud-config/"}
}

// GenericKernelDrivers returns a list of generic kernel drivers to insmod during uki mode
// as they could be useful for a lot of situations.
func GenericKernelDrivers() []string {
	return []string{"virtio", "ata_piix", "cdrom", "ext4", "iso9660", "usb_storage", "ahci", "fat", "vfat",
		"virtio_blk", "virtio_scsi", "virtio_net", "nvme", "overlay", "libata", "sr_mod", "simpledrm", "loop"}
}

var ErrAlreadyMounted = errors.New("already mounted")

const (
	OpCustomMounts         = "custom-mount"
	OpDiscoverState        = "discover-state"
	OpMountState           = "mount-state"
	OpMountBind            = "mount-bind"
	OpMountRoot            = "mount-root"
	OpOverlayMount         = "overlay-mount"
	OpWriteFstab           = "write-fstab"
	OpMountBaseOverlay     = "mount-base-overlay"
	OpMountOEM             = "mount-oem"
	OpRootfsHook           = "rootfs-hook"
	OpInitramfsHook        = "initramfs-hook"
	OpLoadConfig           = "load-config"
	OpMountTmpfs           = "mount-tmpfs"
	OpRemountRootRO        = "remount-ro"
	OpUkiInit              = "uki-init"
	OpSentinel             = "create-sentinel"
	OpUkiUdev              = "uki-udev"
	OpUkiBaseMounts        = "uki-base-mounts"
	OpUkiKernelModules     = "uki-kernel-modules"
	OpWaitForSysroot       = "wait-for-sysroot"
	OpLvmActivate          = "lvm-activation"
	OpKcryptUnlock         = "unlock-all"
	OpKcryptUpgrade        = "upgrade-kcrypt"
	OpUkiKcrypt            = "uki-unlock"
	OpUkiMountLivecd       = "mount-livecd"
	UkiLivecdMountPoint    = "/run/initramfs/live"
	UkiIsoBaseTree         = "/run/rootfsbase"
	UkiIsoBootImage        = "efiboot.img"
	UkiLivecdPath          = "/dev/disk/by-label/UKI_ISO_INSTALL"
	UkiDefaultcdrom        = "/dev/sr0"
	UkiDefaultcdromFsType  = "iso9660"
	UkiDefaultEfiimgFsType = "vfat"
	PersistentStateTarget  = "/usr/local/.state"
	LogDir                 = "/run/immucore"
)
