package constants

import "errors"

func DefaultRWPaths() []string {
	// Default RW_PATHS to mount if not overriden by the cos-layout.env file
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

	OpSentinel = "create-sentinel"

	PersistentStateTarget = "/usr/local/.state"
)
