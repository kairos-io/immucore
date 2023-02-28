package mount

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/mount"
	internalUtils "github.com/kairos-io/immucore/internal/utils"
)

// https://github.com/kairos-io/packages/blob/94aa3bef3d1330cb6c6905ae164f5004b6a58b8c/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-mount-layout.sh#L129
func baseOverlay(overlay Overlay) (mountOperation, error) {
	if err := os.MkdirAll(overlay.Base, 0700); err != nil {
		return mountOperation{}, err
	}

	dat := strings.Split(overlay.BackingBase, ":")

	if len(dat) != 2 {
		return mountOperation{}, fmt.Errorf("invalid backing base. must be a tmpfs with a size or a block device. e.g. tmpfs:30%%, block:/dev/sda1. Input: %s", overlay.BackingBase)
	}

	t := dat[0]
	switch t {
	case "tmpfs":
		tmpMount := mount.Mount{Type: "tmpfs", Source: "tmpfs", Options: []string{fmt.Sprintf("size=%s", dat[1])}}
		tmpFstab := internalUtils.MountToFstab(tmpMount)
		tmpFstab.File = internalUtils.CleanSysrootForFstab(overlay.Base)
		return mountOperation{
			MountOption: tmpMount,
			FstabEntry:  *tmpFstab,
			Target:      overlay.Base,
		}, nil
	case "block":
		blockMount := mount.Mount{Type: "auto", Source: dat[1]}
		tmpFstab := internalUtils.MountToFstab(blockMount)
		// TODO: Check if this is properly written to fstab, currently have no examples
		tmpFstab.File = internalUtils.CleanSysrootForFstab(overlay.Base)
		tmpFstab.MntOps["default"] = ""

		return mountOperation{
			MountOption: blockMount,
			FstabEntry:  *tmpFstab,
			Target:      overlay.Base,
		}, nil
	default:
		return mountOperation{}, fmt.Errorf("invalid overlay backing base type")
	}
}

// https://github.com/kairos-io/packages/blob/94aa3bef3d1330cb6c6905ae164f5004b6a58b8c/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-mount-layout.sh#L183
func mountBind(mountpoint, root, stateTarget string) mountOperation {
	mountpoint = strings.TrimLeft(mountpoint, "/") // normalize, remove / upfront as we are going to re-use it in subdirs
	rootMount := filepath.Join(root, mountpoint)
	bindMountPath := strings.ReplaceAll(mountpoint, "/", "-")

	stateDir := filepath.Join(root, stateTarget, fmt.Sprintf("%s.bind", bindMountPath))

	tmpMount := mount.Mount{
		Type:   "overlay",
		Source: stateDir,
		Options: []string{
			//"defaults",
			"bind",
		},
	}

	tmpFstab := internalUtils.MountToFstab(tmpMount)
	tmpFstab.File = internalUtils.CleanSysrootForFstab(fmt.Sprintf("/%s", mountpoint))
	tmpFstab.Spec = strings.ReplaceAll(tmpFstab.Spec, root, "")
	return mountOperation{
		MountOption: tmpMount,
		FstabEntry:  *tmpFstab,
		Target:      rootMount,
		PrepareCallback: func() error {
			if err := internalUtils.CreateIfNotExists(rootMount); err != nil {
				return err
			}

			if err := internalUtils.CreateIfNotExists(stateDir); err != nil {
				return err
			}
			return internalUtils.SyncState(internalUtils.AppendSlash(rootMount), internalUtils.AppendSlash(stateDir))
		},
	}
}

// https://github.com/kairos-io/packages/blob/94aa3bef3d1330cb6c6905ae164f5004b6a58b8c/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-mount-layout.sh#L145
func mountWithBaseOverlay(mountpoint, root, base string) mountOperation {
	mountpoint = strings.TrimLeft(mountpoint, "/") // normalize, remove / upfront as we are going to re-use it in subdirs
	rootMount := filepath.Join(root, mountpoint)
	bindMountPath := strings.ReplaceAll(mountpoint, "/", "-")

	// TODO: Should we error out if we cant create the target to mount to?
	_ = internalUtils.CreateIfNotExists(rootMount)
	upperdir := filepath.Join(base, bindMountPath, ".overlay", "upper")
	workdir := filepath.Join(base, bindMountPath, ".overlay", "work")

	tmpMount := mount.Mount{
		Type:   "overlay",
		Source: "overlay",
		Options: []string{
			//"defaults",
			fmt.Sprintf("lowerdir=%s", rootMount),
			fmt.Sprintf("upperdir=%s", upperdir),
			fmt.Sprintf("workdir=%s", workdir),
		},
	}

	tmpFstab := internalUtils.MountToFstab(tmpMount)
	tmpFstab.File = internalUtils.CleanSysrootForFstab(rootMount)
	// TODO: update fstab with x-systemd info
	// https://github.com/kairos-io/packages/blob/94aa3bef3d1330cb6c6905ae164f5004b6a58b8c/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-mount-layout.sh#L170
	return mountOperation{
		MountOption: tmpMount,
		FstabEntry:  *tmpFstab,
		Target:      rootMount,
		PrepareCallback: func() error {
			// Make sure workdir and/or upper exists
			_ = os.MkdirAll(upperdir, os.ModePerm)
			_ = os.MkdirAll(workdir, os.ModePerm)
			return nil
		},
	}
}
