package constants

import (
	"errors"
)

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
	return []string{
		"af_packet",
		"ahci",
		"ahcpi-plaftorm",
		"ata_generic",
		"ata_piix",
		"cdrom",
		"dm_mod",
		"dm-verity",
		"e1000",
		"e1000e",
		"ehci_hcd",
		"ehci_pci",
		"ext2",
		"ext4",
		"fat",
		"fuse",
		"hid-generic",
		"iso9660",
		"isofs",
		"libahcpi-platform",
		"libata",
		"loop",
		"mmc_block", // mmc block device support
		"nls_cp437",
		"nls_iso8859_1",
		"nvme",
		"ohci_hcd",
		"ohci_pci",
		"overlay",
		"paride",
		"part_msdos",
		"pata_acpi",
		"scsi_mod",
		"sd_mod",
		"sdhci-pci", // some mmc devices seems to use this like the raxda x4
		"simpledrm",
		"squashfs",
		"sr_mod",
		"uas",
		"uhci_hcd",
		"usb_common",
		"usbcore",
		"usbhid",
		"usbms",
		"usb_storage",
		"vfat",
		"virtio",
		"virtio_blk",
		"virtio_net",
		"virtio_pci",
		"virtio_scsi",
		"xhci_hcd",
		"xhci_pci",
	}
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
	OpUkiInit              = "uki-init"
	OpSentinel             = "create-sentinel"
	OpUkiUdev              = "uki-udev"
	OpUkiBaseMounts        = "uki-base-mounts"
	OpUkiPivotToSysroot    = "uki-pivot-to-sysroot"
	OpUkiKernelModules     = "uki-kernel-modules"
	OpWaitForSysroot       = "wait-for-sysroot"
	OpLvmActivate          = "lvm-activation"
	OpKcryptUnlock         = "unlock-all"
	OpKcryptUpgrade        = "upgrade-kcrypt"
	OpUkiKcrypt            = "uki-unlock"
	OpUkiMountLivecd       = "mount-livecd"
	OpUkiExtractCerts      = "extract-certs"
	OpUkiCopySysExtensions = "copy-sysextensions"
	UkiLivecdMountPoint    = "/run/initramfs/live"
	UkiIsoBaseTree         = "/run/rootfsbase"
	UkiIsoBootImage        = "efiboot.img"
	UkiLivecdPath          = "/dev/disk/by-label/UKI_ISO_INSTALL"
	UkiDefaultcdrom        = "/dev/sr0"
	UkiDefaultcdromFsType  = "iso9660"
	UkiDefaultEfiimgFsType = "vfat"
	UkiSysrootDir          = "sysroot"
	PersistentStateTarget  = "/usr/local/.state"
	LogDir                 = "/run/immucore"
	PathAppend             = "/usr/bin:/usr/sbin:/bin:/sbin"
	PATH                   = "PATH"
	DefaultPCR             = 11
	SourceSysExtDir        = "/.extra/sysext/"
	DestSysExtDir          = "/run/extensions"
	VerityCertDir          = "/run/verity.d/"
	SysextDefaultPolicy    = "--image-policy=\"root=verity+signed+absent:usr=verity+signed+absent\""
)
