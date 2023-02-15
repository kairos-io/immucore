package constants

import "errors"

func DefaultRWPaths() []string {
	// Default RW_PATHS to mount if there are none defined
	return []string{"/etc", "/root", "/home", "/opt", "/srv", "/usr/local", "/var"}
}

var ErrAlreadyMounted = errors.New("already mounted")

const (
	OpCustomMounts  = "custom-mount"
	OpDiscoverState = "discover-state"
	OpMountState    = "mount-state"
	OpMountBind     = "mount-bind"

	OpMountRoot        = "mount-root"
	OpOverlayMount     = "overlay-mount"
	OpWriteFstab       = "write-fstab"
	OpMountBaseOverlay = "mount-base-overlay"
	OpMountOEM         = "mount-oem"

	OpRootfsHook = "rootfs-hook"
	OpLoadConfig = "load-config"
	OpMountTmpfs = "mount-tmpfs"

	PersistentStateTarget = "/usr/local/.state"
)
