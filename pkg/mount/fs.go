package mount

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/mount"
	internalUtils "github.com/kairos-io/immucore/internal/utils"
	"github.com/moby/sys/mountinfo"
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
		err := mount.All([]mount.Mount{tmpMount}, overlay.Base)
		tmpFstab := internalUtils.MountToFstab(tmpMount)
		tmpFstab.File = overlay.BackingBase
		return mountOperation{
			MountOption: tmpMount,
			FstabEntry:  *tmpFstab,
			Target:      overlay.Base,
		}, err
	case "block":
		blockMount := mount.Mount{Type: "auto", Source: dat[1]}
		err := mount.All([]mount.Mount{blockMount}, overlay.Base)

		tmpFstab := internalUtils.MountToFstab(blockMount)
		tmpFstab.File = overlay.BackingBase
		tmpFstab.MntOps["default"] = ""

		return mountOperation{
			MountOption: blockMount,
			FstabEntry:  *tmpFstab,
			Target:      overlay.Base,
		}, err
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
	tmpFstab.File = fmt.Sprintf("/%s", mountpoint)
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
func mountWithBaseOverlay(mountpoint, root, base string) (mountOperation, error) {
	mountpoint = strings.TrimLeft(mountpoint, "/") // normalize, remove / upfront as we are going to re-use it in subdirs
	rootMount := filepath.Join(root, mountpoint)
	bindMountPath := strings.ReplaceAll(mountpoint, "/", "-")

	// TODO: Should we error out if we cant create the target to mount to?
	_ = internalUtils.CreateIfNotExists(rootMount)
	if mounted, _ := mountinfo.Mounted(rootMount); !mounted {
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
		tmpFstab.File = rootMount

		// TODO: update fstab with x-systemd info
		// https://github.com/kairos-io/packages/blob/94aa3bef3d1330cb6c6905ae164f5004b6a58b8c/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-mount-layout.sh#L170
		return mountOperation{
			MountOption: tmpMount,
			FstabEntry:  *tmpFstab,
			Target:      rootMount,
			PrepareCallback: func() error {
				// Make sure workdir and/or upper exists
				os.MkdirAll(upperdir, os.ModePerm)
				os.MkdirAll(workdir, os.ModePerm)
				return nil
			},
		}, nil
	}

	return mountOperation{}, fmt.Errorf("already mounted")
}
