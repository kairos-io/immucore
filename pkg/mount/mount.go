package mount

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/mount"
	"github.com/deniswernert/go-fstab"
	"github.com/kairos-io/immucore/pkg/profile"
	"github.com/moby/sys/mountinfo"
)

func MountOverlayFS() {
	mount.All([]mount.Mount{}, "foo")
}

// https://github.com/kairos-io/packages/blob/94aa3bef3d1330cb6c6905ae164f5004b6a58b8c/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-mount-layout.sh#L129
func BaseOverlay(overlay profile.Overlay) (fstab.Mount, error) {
	if err := os.MkdirAll(overlay.Base, 0700); err != nil {
		return fstab.Mount{}, err
	}

	dat := strings.Split(overlay.BackingBase, ":")

	if len(dat) != 2 {
		return fstab.Mount{}, fmt.Errorf("invalid backing base. must be a tmpfs with a size or a block device. e.g. tmpfs:30%%, block:/dev/sda1. Input: %s", overlay.BackingBase)
	}

	t := dat[0]
	switch t {
	case "tmpfs":
		tmpMount := mount.Mount{Type: "tmpfs", Source: "tmpfs", Options: []string{"defaults", fmt.Sprintf("size=%s", dat[1])}}
		err := mount.All([]mount.Mount{tmpMount}, overlay.Base)

		fstab := mountToStab(tmpMount)
		fstab.File = overlay.BackingBase

		return *fstab, err
	case "block":
		blockMount := mount.Mount{Type: "auto", Source: dat[1]}
		err := mount.All([]mount.Mount{blockMount}, overlay.Base)

		fstab := mountToStab(blockMount)
		fstab.File = overlay.BackingBase
		fstab.MntOps["default"] = ""

		return *fstab, err
	default:
		return fstab.Mount{}, fmt.Errorf("invalid overlay backing base type")
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

func MountEphemeral(path []string) {

}

func MountPeristentPaths() {

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

// https://github.com/kairos-io/packages/blob/94aa3bef3d1330cb6c6905ae164f5004b6a58b8c/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-mount-layout.sh#L183
func mountBind(mountpoint, root, stateTarget string) (fstab.Mount, error) {
	mountpoint = strings.TrimLeft(mountpoint, "/") // normalize, remove / upfront as we are going to re-use it in subdirs
	rootMount := filepath.Join(root, mountpoint)
	bindMountPath := strings.ReplaceAll(mountpoint, "/", "-")

	stateDir := filepath.Join(root, stateTarget, fmt.Sprintf("%s.bind", bindMountPath))

	if mounted, _ := mountinfo.Mounted(rootMount); !mounted {
		if err := createIfNotExists(rootMount); err != nil {
			return fstab.Mount{}, err
		}

		if err := createIfNotExists(stateDir); err != nil {
			return fstab.Mount{}, err
		}

		syncState(appendSlash(rootMount), appendSlash(stateDir))

		tmpMount := mount.Mount{
			Type:   "overlay",
			Source: stateDir,

			Options: []string{
				"defaults",
				"bind",
			},
		}
		err := mount.All([]mount.Mount{tmpMount}, rootMount)
		if err != nil {
			return fstab.Mount{}, err
		}

		fstab := mountToStab(tmpMount)
		fstab.File = fmt.Sprintf("/%s", mountpoint)
		fstab.Spec = strings.ReplaceAll(fstab.Spec, root, "")

	}
	return fstab.Mount{}, fmt.Errorf("already mounted")
}

func syncState(src, dst string) error {
	return exec.Command("rsync", "-aqAX", src, dst).Run()
}

// https://github.com/kairos-io/packages/blob/94aa3bef3d1330cb6c6905ae164f5004b6a58b8c/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-mount-layout.sh#L145
func mountWithBaseOverlay(mountpoint, root, base string) (fstab.Mount, error) {
	mountpoint = strings.TrimLeft(mountpoint, "/") // normalize, remove / upfront as we are going to re-use it in subdirs
	rootMount := filepath.Join(root, mountpoint)
	bindMountPath := strings.ReplaceAll(mountpoint, "/", "-")

	createIfNotExists(rootMount)
	if mounted, _ := mountinfo.Mounted(rootMount); !mounted {
		upperdir := filepath.Join(base, bindMountPath, ".overlay", "upper")
		workdir := filepath.Join(base, bindMountPath, ".overlay", "work")
		// Make sure workdir and/or upper exists
		os.MkdirAll(upperdir, os.ModePerm)
		os.MkdirAll(workdir, os.ModePerm)

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
		err := mount.All([]mount.Mount{tmpMount}, rootMount)

		fstab := mountToStab(tmpMount)
		fstab.File = rootMount

		// TODO: update fstab with x-systemd info
		// https://github.com/kairos-io/packages/blob/94aa3bef3d1330cb6c6905ae164f5004b6a58b8c/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-mount-layout.sh#L170

		return *fstab, err
	}

	return fstab.Mount{}, fmt.Errorf("already mounted")
}
