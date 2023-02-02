package mount

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/containerd/containerd/mount"
	"github.com/deniswernert/go-fstab"
	"github.com/kairos-io/immucore/pkg/profile"
	"github.com/kairos-io/kairos/pkg/utils"
	"github.com/spectrocloud-labs/herd"
)

type State struct {
	Rootdir      string            // e.g. /sysroot inside initrd with pivot, / with nopivot
	TargetImage  string            // e.g. /cOS/active.img
	OverlayDir   []string          // e.g. /var
	BindMounts   []string          // e.g. /etc/kubernetes
	StateDir     string            // e.g. "/usr/local/.state"
	TargetLabel  string            // e.g. COS_ACTIVE
	FStabFile    string            // e.g. /etc/fstab
	MountRoot    bool              // e.g. if true, it tries to find the image to loopback mount
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
	opCustomMounts     = "custom-mount"
	opDiscoverState    = "discover-state"
	opMountState       = "mount-state"
	opMountRoot        = "mount-root"
	opOverlayMount     = "overlay-mount"
	opWriteFstab       = "write-fstab"
	opMountBaseOverlay = "mount-base-overlay"
)

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
				op := mountOperation{
					MountOption: mountPoint,
					FstabEntry:  *fstab,
					Target:      where,
				}

				err := op.run()
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

func (s *State) Register(g *herd.Graph) error {

	// TODO: add, hooks, fstab, systemd compat

	// This is legacy - in UKI we don't need to found the img, this needs to run in a conditional
	if s.MountRoot {
		g.Add(opDiscoverState,
			herd.WithDeps(opMountState),
			herd.WithCallback(
				func(ctx context.Context) error {
					_, err := utils.SH(fmt.Sprintf("losetup --show -f /run/initramfs/cos-state%s", s.TargetImage))
					return err
				},
			))

		g.Add(opMountState,
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

		g.Add(opMountRoot,
			herd.WithDeps(opDiscoverState),
			herd.WithCallback(
				s.MountOP(
					fmt.Sprintf("/dev/disk/by-label/%s", s.TargetLabel),
					s.Rootdir,
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

	}
	// end sysroot mount

	// overlay mount start
	if rootFSType(s.Rootdir) != "overlay" {
		g.Add(opMountBaseOverlay,
			herd.WithCallback(
				func(ctx context.Context) error {
					op, err := baseOverlay(profile.Overlay{
						Base:        "/run/overlay",
						BackingBase: "tmpfs:20%",
					})
					if err != nil {
						return err
					}
					s.fstabs = append(s.fstabs, &op.FstabEntry)
					return op.run()
				},
			),
		)
	}

	overlayCondition := herd.ConditionalOption(func() bool { return rootFSType(s.Rootdir) != "overlay" }, herd.WithDeps(opMountBaseOverlay))

	// TODO: Add fsck
	// mount overlay
	for _, p := range s.OverlayDir {
		g.Add(
			genOpreferenceName(opOverlayMount, p),
			overlayCondition,
			herd.WithCallback(
				func(ctx context.Context) error {
					op, err := mountWithBaseOverlay(p, s.Rootdir, "/run/overlay")
					if err != nil {
						return err
					}
					s.fstabs = append(s.fstabs, &op.FstabEntry)
					return op.run()
				},
			),
		)
	}

	// custom mounts TODO: disk/path
	for id, mountpoint := range s.CustomMounts {
		g.Add(
			genOpreferenceName(opCustomMounts, mountpoint),
			overlayCondition,
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
			genOpreferenceName(opMountState, p),
			overlayCondition,
			herd.WithDeps(genOpreferenceFromMap(opCustomMounts, s.CustomMounts)...),
			herd.WithCallback(
				func(ctx context.Context) error {
					op, err := mountBind(p, s.Rootdir, s.StateDir)
					if err != nil {
						return err
					}
					s.fstabs = append(s.fstabs, &op.FstabEntry)
					return op.run()
				},
			),
		)
	}

	// overlay mount end
	g.Add(opMountRoot,
		herd.ConditionalOption(func() bool { return s.MountRoot }, herd.WithDeps("mount-overlay-base")),
		herd.WithCallback(
			s.MountOP(
				"/dev/disk/by-label/COS_OEM",
				s.path("/oem"),
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

	g.Add(opWriteFstab,
		overlayCondition,
		herd.ConditionalOption(func() bool { return s.MountRoot }, herd.WithDeps(opMountRoot)),
		herd.WithDeps(opMountRoot),
		herd.WithDeps(genOpreferenceFromMap(opCustomMounts, s.CustomMounts)...),
		herd.WithDeps(genOpreference(opMountState, s.BindMounts)...),
		herd.WithDeps(genOpreference(opOverlayMount, s.OverlayDir)...),
		herd.WithCallback(s.WriteFstab(s.FStabFile)))

	return nil
}
