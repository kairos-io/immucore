package mount

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/containerd/mount"
	"github.com/deniswernert/go-fstab"
	"github.com/kairos-io/immucore/pkg/profile"
	"github.com/kairos-io/kairos/pkg/utils"
	"github.com/moby/sys/mountinfo"
	"github.com/spectrocloud-labs/herd"
)

type MountOperation struct {
	FstabEntry      fstab.Mount
	MountOption     mount.Mount
	Target          string
	PrepareCallback func() error
}

func (m MountOperation) Run() error {
	if m.PrepareCallback != nil {
		if err := m.PrepareCallback(); err != nil {
			return err
		}
	}
	return mount.All([]mount.Mount{m.MountOption}, m.Target)
}

// https://github.com/kairos-io/packages/blob/94aa3bef3d1330cb6c6905ae164f5004b6a58b8c/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-mount-layout.sh#L129
func BaseOverlay(overlay profile.Overlay) (MountOperation, error) {
	if err := os.MkdirAll(overlay.Base, 0700); err != nil {
		return MountOperation{}, err
	}

	dat := strings.Split(overlay.BackingBase, ":")

	if len(dat) != 2 {
		return MountOperation{}, fmt.Errorf("invalid backing base. must be a tmpfs with a size or a block device. e.g. tmpfs:30%%, block:/dev/sda1. Input: %s", overlay.BackingBase)
	}

	t := dat[0]
	switch t {
	case "tmpfs":
		tmpMount := mount.Mount{Type: "tmpfs", Source: "tmpfs", Options: []string{"defaults", fmt.Sprintf("size=%s", dat[1])}}
		err := mount.All([]mount.Mount{tmpMount}, overlay.Base)
		fstab := mountToStab(tmpMount)
		fstab.File = overlay.BackingBase
		return MountOperation{
			MountOption: tmpMount,
			FstabEntry:  *fstab,
			Target:      overlay.Base,
		}, err
	case "block":
		blockMount := mount.Mount{Type: "auto", Source: dat[1]}
		err := mount.All([]mount.Mount{blockMount}, overlay.Base)

		fstab := mountToStab(blockMount)
		fstab.File = overlay.BackingBase
		fstab.MntOps["default"] = ""

		return MountOperation{
			MountOption: blockMount,
			FstabEntry:  *fstab,
			Target:      overlay.Base,
		}, err
	default:
		return MountOperation{}, fmt.Errorf("invalid overlay backing base type")
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
func mountBind(mountpoint, root, stateTarget string) (MountOperation, error) {
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

		fstab := mountToStab(tmpMount)
		fstab.File = fmt.Sprintf("/%s", mountpoint)
		fstab.Spec = strings.ReplaceAll(fstab.Spec, root, "")
		return MountOperation{
			MountOption: tmpMount,
			FstabEntry:  *fstab,
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
	return MountOperation{}, fmt.Errorf("already mounted")
}

func syncState(src, dst string) error {
	return exec.Command("rsync", "-aqAX", src, dst).Run()
}

// https://github.com/kairos-io/packages/blob/94aa3bef3d1330cb6c6905ae164f5004b6a58b8c/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-mount-layout.sh#L145
func mountWithBaseOverlay(mountpoint, root, base string) (MountOperation, error) {
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

		fstab := mountToStab(tmpMount)
		fstab.File = rootMount

		// TODO: update fstab with x-systemd info
		// https://github.com/kairos-io/packages/blob/94aa3bef3d1330cb6c6905ae164f5004b6a58b8c/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-mount-layout.sh#L170
		return MountOperation{
			MountOption: tmpMount,
			FstabEntry:  *fstab,
			Target:      rootMount,
			PrepareCallback: func() error {
				// Make sure workdir and/or upper exists
				os.MkdirAll(upperdir, os.ModePerm)
				os.MkdirAll(workdir, os.ModePerm)
				return nil
			},
		}, nil
	}

	return MountOperation{}, fmt.Errorf("already mounted")
}

type State struct {
	Rootdir     string
	TargetImage string   // e.g. /cOS/active.img
	OverlayDir  []string // e.g. /var
	BindMounts  []string // e.g. /etc/kubernetes

	CustomMounts map[string]string // e.g. diskid : mountpoint

	fstabs []*fstab.Mount
}

func genOpreferenceName(op, s string) string {
	return fmt.Sprintf("%s-%s", op, s)
}

func genOpreferenceFromMap(op string, m map[string]string) (res []string) {
	values := []string{}
	for _, n := range m {
		values = append(values, n)
	}

	res = genOpreference(op, values)
	return
}
func genOpreference(op string, s []string) (res []string) {
	for _, n := range s {
		res = append(res, genOpreferenceName(op, n))
	}
	return
}

const (
	opCustomMounts = "custom-mount"
)

func (s *State) Register(g *herd.Graph) error {

	// TODO: add, hooks, fstab, systemd compat

	g.Add("discover-mount",
		herd.WithDeps("mount-cos-state"),
		herd.WithCallback(
			func(ctx context.Context) error {
				_, err := utils.SH(fmt.Sprintf("losetup --show -f /run/initramfs/cos-state%s", s.TargetImage))
				return err
			},
		))

	g.Add("mount-cos-state",
		herd.WithCallback(
			s.MountOP(
				"/dev/disk/by-label/COS_STATE",
				s.path("/run/initramfs/cos-state"),
				"auto",
				[]string{
					"ro", // or rw
				}, 60*time.Second),
		),
	)

	g.Add("mount-overlay-base",
		herd.WithCallback(
			func(ctx context.Context) error {
				op, err := BaseOverlay(profile.Overlay{
					Base:        "/run/overlay",
					BackingBase: "tmpfs:20%",
				})
				if err != nil {
					return err
				}
				s.fstabs = append(s.fstabs, &op.FstabEntry)
				return op.Run()
			},
		),
	)

	// TODO: Add fsck
	// mount overlay
	for _, p := range s.OverlayDir {
		g.Add("mount-overlays-base",
			herd.WithCallback(
				func(ctx context.Context) error {
					op, err := mountWithBaseOverlay(p, s.Rootdir, "/run/overlay")
					if err != nil {
						return err
					}
					s.fstabs = append(s.fstabs, &op.FstabEntry)
					return op.Run()
				},
			),
		)
	}

	// custom mounts TODO: disk/path
	for id, mountpoint := range s.CustomMounts {
		g.Add(
			genOpreferenceName(opCustomMounts, mountpoint),
			herd.WithCallback(
				s.MountOP(
					id,
					s.path(mountpoint),
					"auto",
					[]string{
						"ro", // or rw
					}, 60*time.Second),
			),
		)
	}

	// mount state
	// mount state is defined over a custom mount (/usr/local/.state for instance, needs to be mounted over a device)
	for _, p := range s.BindMounts {
		g.Add(
			genOpreferenceName("mount-state", p),
			herd.WithDeps(genOpreferenceFromMap(opCustomMounts, s.CustomMounts)...),
			herd.WithCallback(
				func(ctx context.Context) error {
					op, err := mountBind(p, s.Rootdir, "/usr/local/.state")
					if err != nil {
						return err
					}
					s.fstabs = append(s.fstabs, &op.FstabEntry)
					return op.Run()
				},
			),
		)
	}
	g.Add("mount-sysroot",
		herd.WithCallback(
			s.MountOP(
				"/dev/disk/by-label/COS_ACTIVE",
				s.path("/sysroot"),
				"auto",
				[]string{
					"ro", // or rw
					"suid",
					"dev",
					"exec",
					"auto",
					"nouser",
					"async",
				}, 60*time.Second),
		),
	)

	g.Add("mount-oem",
		herd.WithCallback(
			s.MountOP(
				"/dev/disk/by-label/COS_OEM",
				"/oem",
				"auto",
				[]string{
					"rw",
					"suid",
					"dev",
					"exec",
					"noauto",
					"nouser",
					"async",
				}, 60*time.Second),
		),
	)

	g.Add("write-fstab", herd.WithCallback(s.WriteFstab("foo")))

	return nil
}

func (s *State) path(p ...string) string {
	return filepath.Join(append([]string{s.Rootdir}, p...)...)
}

func (s *State) WriteFstab(fstabFile string) func(context.Context) error {
	return func(ctx context.Context) error {
		for _, fst := range s.fstabs {
			select {
			case <-ctx.Done():
			default:
				f, err := os.OpenFile(fstabFile,
					os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					return err
				}
				defer f.Close()
				if _, err := f.WriteString(fmt.Sprintf("%s\n", fst.String())); err != nil {
					return err
				}
			}
		}
		return nil
	}
}

func (s *State) MountOP(what, where, t string, options []string, timeout time.Duration) func(context.Context) error {
	return func(c context.Context) error {
		for {
			select {
			default:
				time.Sleep(1 * time.Second)
				mountPoint := mount.Mount{
					Type:    t,
					Source:  what,
					Options: options,
				}
				fstab := mountToStab(mountPoint)
				fstab.File = where
				op := MountOperation{
					MountOption: mountPoint,
					FstabEntry:  *fstab,
					Target:      where,
				}

				err := op.Run()
				if err != nil {
					continue
				}

				s.fstabs = append(s.fstabs, fstab)

				return nil
			case <-c.Done():
				return fmt.Errorf("context canceled")
			case <-time.After(timeout):
				return fmt.Errorf("timeout exhausted")
			}
		}
	}
}
