package constants

import "errors"

func DefaultRWPaths() []string {
	// Default RW_PATHS to mount
	// If none defined, your system wont even boot probably
	return []string{"/var", "/etc", "/srv"}
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

	OpSentinel = "create-sentinel"

	PersistentStateTarget = "/usr/local/.state"
)
