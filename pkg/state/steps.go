package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	cnst "github.com/kairos-io/immucore/internal/constants"
	internalUtils "github.com/kairos-io/immucore/internal/utils"
	"github.com/kairos-io/immucore/pkg/op"
	"github.com/kairos-io/kairos-sdk/utils"
	kcrypt "github.com/kairos-io/kcrypt/pkg/lib"
	"github.com/spectrocloud-labs/herd"
)

// MountTmpfsDagStep adds the step to mount /tmp .
func (s *State) MountTmpfsDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpMountTmpfs, herd.WithCallback(
		func(_ context.Context) error {
			fstab, err := op.MountOPWithFstab("tmpfs", "/tmp", "tmpfs", []string{"rw"}, 10*time.Second)
			for _, f := range fstab {
				s.fstabs = append(s.fstabs, f)
			}
			return err
		},
	))
}

// MountRootDagStep will add the step to mount the Rootdir for the system
// 1 - mount the state partition to find the images (active/passive/recovery)
// 2 - mount the image as a loop device
// 3 - Mount the labels as /sysroot .
func (s *State) MountRootDagStep(g *herd.Graph) error {
	var err error

	// 1 - mount the state partition to find the images (active/passive/recovery)
	err = g.Add(cnst.OpMountState,
		herd.WithCallback(
			func(_ context.Context) error {
				c, _ := internalUtils.CommandWithPath("stat /sysroot")
				internalUtils.Log.Info().Str("path", c).Msg("Sysroot status")
				fstab, err := op.MountOPWithFstab(
					internalUtils.GetState(),
					s.path("/run/initramfs/cos-state"),
					internalUtils.DiskFSType(internalUtils.GetState()),
					[]string{
						s.RootMountMode,
					}, 60*time.Second)
				for _, f := range fstab {
					s.fstabs = append(s.fstabs, f)
				}
				c, _ = internalUtils.CommandWithPath("stat /sysroot")
				internalUtils.Log.Info().Str("path", c).Msg("Sysroot status")
				return err
			},
		),
	)
	if err != nil {
		internalUtils.Log.Err(err).Send()
	}

	// 2 - mount the image as a loop device
	err = g.Add(cnst.OpDiscoverState,
		herd.WithDeps(cnst.OpMountState),
		herd.WithCallback(
			func(_ context.Context) error {
				c, _ := internalUtils.CommandWithPath("stat /sysroot")
				internalUtils.Log.Info().Str("path", c).Msg("Sysroot status")
				// Check if loop device is mounted already
				if internalUtils.IsMounted(s.TargetDevice) {
					internalUtils.Log.Debug().Str("targetImage", s.TargetImage).Str("path", s.Rootdir).Str("TargetDevice", s.TargetDevice).Msg("Not mounting loop, already mounted")
					return nil
				}
				_ = internalUtils.Fsck(s.path("/run/initramfs/cos-state", s.TargetImage))
				cmd := fmt.Sprintf("losetup -f %s", s.path("/run/initramfs/cos-state", s.TargetImage))
				_, err := utils.SH(cmd)
				s.LogIfError(err, "losetup")
				// Trigger udevadm
				// On some systems the COS_ACTIVE/PASSIVE label is automatically shown as soon as we mount the device
				// But on other it seems like it won't trigger which causes the sysroot to not be mounted as we cant find
				// the block device by the target label. Make sure we run this after mounting so we refresh the devices.
				sh, _ := utils.SH("udevadm trigger")
				internalUtils.Log.Debug().Str("output", sh).Msg("udevadm trigger")
				internalUtils.Log.Debug().Str("targetImage", s.TargetImage).Str("path", s.Rootdir).Str("TargetDevice", s.TargetDevice).Msg("mount done")
				c, _ = internalUtils.CommandWithPath("stat /sysroot")
				internalUtils.Log.Info().Str("path", c).Msg("Sysroot status")
				return err
			},
		))
	if err != nil {
		internalUtils.Log.Err(err).Send()
	}

	// 3 - Mount the labels as Rootdir
	err = g.Add(cnst.OpMountRoot,
		herd.WithDeps(cnst.OpDiscoverState),
		herd.WithCallback(
			func(_ context.Context) error {
				c, _ := internalUtils.CommandWithPath("stat /sysroot")
				internalUtils.Log.Info().Str("path", c).Msg("Sysroot status")
				fstab, err := op.MountOPWithFstab(
					s.TargetDevice,
					s.Rootdir,
					"ext4", // TODO: Get this just in time? Currently if using DiskFSType is run immediately which is bad because its not mounted
					[]string{
						s.RootMountMode,
						"suid",
						"dev",
						"exec",
						"async",
					}, 10*time.Second)
				for _, f := range fstab {
					s.fstabs = append(s.fstabs, f)
				}
				c, _ = internalUtils.CommandWithPath("stat /sysroot")
				internalUtils.Log.Info().Str("path", c).Msg("Sysroot status")
				return err
			},
		),
	)
	if err != nil {
		internalUtils.Log.Err(err).Send()
	}
	return err
}

// WaitForSysrootDagStep waits for the s.Rootdir and s.Rootdir/system paths to be there
// Useful for livecd/netboot as we want to run steps after s.Rootdir is ready but we don't mount it ourselves.
func (s *State) WaitForSysrootDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpWaitForSysroot,
		herd.WithCallback(func(ctx context.Context) error {
			var timeout = 60 * time.Second
			timeoutArg := internalUtils.CleanupSlice(internalUtils.ReadCMDLineArg("rd.immucore.sysrootwait="))
			if len(timeoutArg) > 0 {
				atoi, err := strconv.Atoi(timeoutArg[0])
				if err == nil && atoi > 0 {
					timeout = time.Duration(atoi) * time.Second
				}
			}

			internalUtils.Log.Debug().Str("timeout", timeout.String()).Msg("Waiting for sysroot")

			cc := time.After(timeout)
			for {
				select {
				default:
					time.Sleep(2 * time.Second)
					_, err := os.Stat(s.Rootdir)
					if err != nil {
						internalUtils.Log.Debug().Str("what", s.Rootdir).Msg("Checking path existence")
						continue
					}
					_, err = os.Stat(filepath.Join(s.Rootdir, "system"))
					if err != nil {
						internalUtils.Log.Debug().Str("what", filepath.Join(s.Rootdir, "system")).Msg("Checking path existence")
						continue
					}
					return nil
				case <-ctx.Done():
					e := fmt.Errorf("context canceled")
					internalUtils.Log.Err(e).Str("what", s.Rootdir).Msg("filepath check canceled")
					return e
				case <-cc:
					e := fmt.Errorf("timeout exhausted")
					internalUtils.Log.Err(e).Str("what", s.Rootdir).Msg("filepath check timeout")
					return e
				}
			}
		}))
}

// LVMActivation will try to activate lvm volumes/groups on the system.
func (s *State) LVMActivation(g *herd.Graph) error {
	return g.Add(cnst.OpLvmActivate, herd.WithCallback(func(_ context.Context) error {
		return internalUtils.ActivateLVM()
	}))
}

// RunKcrypt will run the UnlockAll method of kcrypt to unlock the encrypted partitions
// Requires sysroot to be mounted as the kcrypt-challenger binary is not injected in the initramfs.
func (s *State) RunKcrypt(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add(cnst.OpKcryptUnlock, append(opts, herd.WithCallback(func(_ context.Context) error {
		internalUtils.Log.Debug().Msg("Unlocking with kcrypt")
		return kcrypt.UnlockAllWithLogger(false, internalUtils.Log)
	}))...)
}

// RunKcryptUpgrade will upgrade encrypted partitions created with 1.x to the new 2.x format, where
// we inspect the uuid of the partition directly to know which label to use for the key
// As those old installs have an old agent the only way to do it is during the first boot after the upgrade to the newest immucore.
func (s *State) RunKcryptUpgrade(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add(cnst.OpKcryptUpgrade, append(opts, herd.WithCallback(func(_ context.Context) error {
		c, _ := internalUtils.CommandWithPath("stat /sysroot")
		internalUtils.Log.Info().Str("path", c).Msg("Sysroot status")
		return internalUtils.UpgradeKcryptPartitions()
	}))...)
}
