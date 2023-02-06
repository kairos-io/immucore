package mount

import (
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/mount"
	"github.com/deniswernert/go-fstab"
	"github.com/kairos-io/immucore/pkg/profile"
	"github.com/kairos-io/kairos/pkg/utils"
	"github.com/moby/sys/mountinfo"
)

func rootFSType(s string) string {
	out, _ := utils.SH(fmt.Sprintf("findmnt -rno FSTYPE %s", s))
	return out
}
func createIfNotExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, os.ModePerm)
	}

	return nil
}

func appendSlash(path string) string {
	if !strings.HasSuffix(path, "/") {
		return fmt.Sprintf("%s/", path)
	}

	return path
}

// https://github.com/kairos-io/packages/blob/94aa3bef3d1330cb6c6905ae164f5004b6a58b8c/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-mount-layout.sh#L129
func baseOverlay(overlay profile.Overlay) (mountOperation, error) {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Caller().Logger()
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
		tmpFstab := mountToStab(tmpMount)
		tmpFstab.File = overlay.BackingBase
		return mountOperation{
			MountOption: tmpMount,
			FstabEntry:  *tmpFstab,
			Target:      overlay.Base,
		}, err
	case "block":
		blockMount := mount.Mount{Type: "auto", Source: dat[1]}
		err := mount.All([]mount.Mount{blockMount}, overlay.Base)

		tmpFstab := mountToStab(blockMount)
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

func mountToStab(m mount.Mount) *fstab.Mount {
	opts := map[string]string{}
	for _, o := range m.Options {
		if strings.Contains(o, "=") {
			dat := strings.Split(o, "=")
			key := dat[0]
			value := dat[1]
			opts[key] = value
		} else {
			opts[o] = ""
		}
	}
	return &fstab.Mount{
		Spec:    m.Source,
		VfsType: m.Type,
		MntOps:  opts,
		Freq:    0,
		PassNo:  0,
	}
}

// https://github.com/kairos-io/packages/blob/94aa3bef3d1330cb6c6905ae164f5004b6a58b8c/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-mount-layout.sh#L183
func mountBind(mountpoint, root, stateTarget string) (mountOperation, error) {
	mountpoint = strings.TrimLeft(mountpoint, "/") // normalize, remove / upfront as we are going to re-use it in subdirs
	rootMount := filepath.Join(root, mountpoint)
	bindMountPath := strings.ReplaceAll(mountpoint, "/", "-")

	stateDir := filepath.Join(root, stateTarget, fmt.Sprintf("%s.bind", bindMountPath))

	if mounted, _ := mountinfo.Mounted(rootMount); !mounted {
		tmpMount := mount.Mount{
			Type:   "overlay",
			Source: stateDir,
			Options: []string{
				"defaults",
				"bind",
			},
		}

		tmpFstab := mountToStab(tmpMount)
		tmpFstab.File = fmt.Sprintf("/%s", mountpoint)
		tmpFstab.Spec = strings.ReplaceAll(tmpFstab.Spec, root, "")
		return mountOperation{
			MountOption: tmpMount,
			FstabEntry:  *tmpFstab,
			Target:      rootMount,
			PrepareCallback: func() error {
				if err := createIfNotExists(rootMount); err != nil {
					return err
				}

				if err := createIfNotExists(stateDir); err != nil {
					return err
				}

				return syncState(appendSlash(rootMount), appendSlash(stateDir))
			},
		}, nil
	}
	return mountOperation{}, fmt.Errorf("already mounted")
}

func syncState(src, dst string) error {
	return exec.Command("rsync", "-aqAX", src, dst).Run()
}

// https://github.com/kairos-io/packages/blob/94aa3bef3d1330cb6c6905ae164f5004b6a58b8c/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-mount-layout.sh#L145
func mountWithBaseOverlay(mountpoint, root, base string) (mountOperation, error) {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Caller().Logger()
	log.Debug().Str("mountpoint", mountpoint).Str("root", root).Str("base", base).Msg("mount with base overlay")
	mountpoint = strings.TrimLeft(mountpoint, "/") // normalize, remove / upfront as we are going to re-use it in subdirs
	rootMount := filepath.Join(root, mountpoint)
	bindMountPath := strings.ReplaceAll(mountpoint, "/", "-")

	createIfNotExists(rootMount)
	if mounted, _ := mountinfo.Mounted(rootMount); !mounted {
		upperdir := filepath.Join(base, bindMountPath, ".overlay", "upper")
		workdir := filepath.Join(base, bindMountPath, ".overlay", "work")

		tmpMount := mount.Mount{
			Type:   "overlay",
			Source: "overlay",
			Options: []string{
				"defaults",
				fmt.Sprintf("lowerdir=%s", rootMount),
				fmt.Sprintf("upperdir=%s", upperdir),
				fmt.Sprintf("workdir=%s", workdir),
			},
		}

		tmpFstab := mountToStab(tmpMount)
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
