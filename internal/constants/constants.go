package constants

import "errors"

func DefaultRWPaths() []string {
	// Default RW_PATHS to mount if not override by the cos-layout.env file
	return []string{"/etc", "/root", "/home", "/opt", "/srv", "/usr/local", "/var"}
}

func GetCloudInitPaths() []string {
	return []string{"/system/oem", "/oem/", "/usr/local/cloud-config/"}
}

var ErrAlreadyMounted = errors.New("already mounted")

const (
	OpCustomMounts        = "custom-mount"
	OpDiscoverState       = "discover-state"
	OpMountState          = "mount-state"
	OpMountBind           = "mount-bind"
	OpMountRoot           = "mount-root"
	OpOverlayMount        = "overlay-mount"
	OpWriteFstab          = "write-fstab"
	OpMountBaseOverlay    = "mount-base-overlay"
	OpMountOEM            = "mount-oem"
	OpRootfsHook          = "rootfs-hook"
	OpInitramfsHook       = "initramfs-hook"
	OpLoadConfig          = "load-config"
	OpMountTmpfs          = "mount-tmpfs"
	OpRemountRootRO       = "remount-ro"
	OpUkiInit             = "uki-init"
	OpSentinel            = "create-sentinel"
	OpUkiUdev             = "uki-udev"
	OpUkiBaseMounts       = "uki-base-mounts"
	OpUkiKernelModules    = "uki-kernel-modules"
	OpWaitForSysroot      = "wait-for-sysroot"
	OpLvmActivate         = "lvm-activation"
	OpKcryptUnlock        = "unlock-all"
	PersistentStateTarget = "/usr/local/.state"
	LogDir                = "/run/immucore"
	LinuxFs               = "ext4"
)
