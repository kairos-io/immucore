package mount

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/go-multierror"
	cnst "github.com/kairos-io/immucore/internal/constants"
	internalUtils "github.com/kairos-io/immucore/internal/utils"
	"github.com/kairos-io/kairos/pkg/utils"
	"github.com/kairos-io/kairos/sdk/state"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spectrocloud-labs/herd"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MountTmpfsDagStep adds the step to mount /tmp
func (s *State) MountTmpfsDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpMountTmpfs, herd.WithCallback(s.MountOP("tmpfs", "/tmp", "tmpfs", []string{"rw"}, 10*time.Second)))
}

// MountRootDagStep will add the step to mount the Rootdir for the system
// 1 - mount the state partition to find the images (active/passive/recovery)
// 2 - mount the image as a loop device
// 3 - Mount the labels as /sysroot
func (s *State) MountRootDagStep(g *herd.Graph) error {
	var err error
	runtime, err := state.NewRuntime()
	if err != nil {
		s.Logger.Debug().Err(err).Msg("runtime")
	}
	stateName := runtime.State.Name
	stateFs := runtime.State.Type
	// Recovery is a different partition
	if internalUtils.IsRecovery() {
		stateName = runtime.Recovery.Name
		stateFs = runtime.Recovery.Type
	}
	// 1 - mount the state partition to find the images (active/passive/recovery)
	err = g.Add(cnst.OpMountState,
		herd.WithCallback(
			s.MountOP(
				stateName,
				s.path("/run/initramfs/cos-state"),
				stateFs,
				[]string{
					"ro", // or rw
				}, 60*time.Second),
		),
	)
	if err != nil {
		s.Logger.Err(err).Send()
	}

	// 2 - mount the image as a loop device
	err = g.Add(cnst.OpDiscoverState,
		herd.WithDeps(cnst.OpMountState),
		herd.WithCallback(
			func(ctx context.Context) error {
				// Check if loop device is mounted by checking the existance of the target label
				if internalUtils.IsMountedByLabel(s.TargetLabel) {
					log.Logger.Debug().Str("targetImage", s.TargetImage).Str("path", s.Rootdir).Str("TargetLabel", s.TargetLabel).Msg("Not mounting loop, already mounted")
					return nil
				}
				// TODO: squashfs recovery image?
				cmd := fmt.Sprintf("losetup --show -f %s", s.path("/run/initramfs/cos-state", s.TargetImage))
				_, err := utils.SH(cmd)
				s.LogIfError(err, "losetup")
				// Trigger udevadm
				// On some systems the COS_ACTIVE/PASSIVE label is automatically shown as soon as we mount the device
				// But on other it seems like it won't trigger which causes the sysroot to not be mounted as we cant find
				// the block device by the target label. Make sure we run this after mounting so we refresh the devices.
				sh, _ := utils.SH("udevadm trigger --settle")
				s.Logger.Debug().Str("output", sh).Msg("udevadm trigger")
				return err
			},
		))
	if err != nil {
		s.Logger.Err(err).Send()
	}

	// 3 - Mount the labels as Rootdir
	err = g.Add(cnst.OpMountRoot,
		herd.WithDeps(cnst.OpDiscoverState),
		herd.WithCallback(
			s.MountOP(
				// Using /dev/disk/by-label here allows us to not have to deal with loop devices to identify where was the image mounted
				fmt.Sprintf("/dev/disk/by-label/%s", s.TargetLabel),
				s.Rootdir,
				"ext4", // are images always ext2?
				[]string{
					"ro", // or rw
					"suid",
					"dev",
					"exec",
					// "auto",
					//"nouser",
					"async",
				}, 10*time.Second),
		),
	)
	if err != nil {
		s.Logger.Err(err).Send()
	}
	return err
}

// RootfsStageDagStep will add the rootfs stage.
func (s *State) RootfsStageDagStep(g *herd.Graph, deps ...string) error {
	return g.Add(cnst.OpRootfsHook, herd.WithDeps(deps...), herd.WithCallback(s.RunStageOp("rootfs")))
}

// LoadEnvLayoutDagStep will add the stage to load from cos-layout.env and fill the proper CustomMounts, OverlayDirs and BindMounts
func (s *State) LoadEnvLayoutDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpLoadConfig,
		herd.WithDeps(cnst.OpRootfsHook),
		herd.WithCallback(func(ctx context.Context) error {
			log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
			if s.CustomMounts == nil {
				s.CustomMounts = map[string]string{}
			}

			env, err := internalUtils.ReadEnv("/run/cos/cos-layout.env")
			if err != nil {
				log.Logger.Err(err).Msg("Reading env")
				return err
			}
			// populate from env here
			s.OverlayDirs = internalUtils.CleanupSlice(strings.Split(env["RW_PATHS"], " "))
			// Append default RW_Paths if Dirs are empty
			if len(s.OverlayDirs) == 0 {
				s.OverlayDirs = cnst.DefaultRWPaths()
			}
			// Remove any duplicates
			s.OverlayDirs = internalUtils.UniqueSlice(s.OverlayDirs)

			s.BindMounts = internalUtils.CleanupSlice(strings.Split(env["PERSISTENT_STATE_PATHS"], " "))
			// Remove any duplicates
			s.BindMounts = internalUtils.UniqueSlice(s.BindMounts)

			s.StateDir = env["PERSISTENT_STATE_TARGET"]
			if s.StateDir == "" {
				s.StateDir = cnst.PersistentStateTarget
			}

			addLine := func(d string) {
				dat := strings.Split(d, ":")
				if len(dat) == 2 {
					disk := dat[0]
					path := dat[1]
					s.CustomMounts[disk] = path
				}
			}
			// Parse custom mounts also from cmdline (rd.cos.mount=)
			// Parse custom mounts also from env file (VOLUMES)
			for _, v := range append(internalUtils.ReadCMDLineArg("rd.cos.mount="), strings.Split(env["VOLUMES"], " ")...) {
				addLine(internalUtils.ParseMount(v))
			}

			return nil
		}))
}

// MountOemDagStep will add mounting COS_OEM partition under s.Rootdir + /oem
func (s *State) MountOemDagStep(g *herd.Graph, deps ...string) error {
	runtime, err := state.NewRuntime()
	if err != nil {
		s.Logger.Debug().Err(err).Msg("runtime")
	}
	return g.Add(cnst.OpMountOEM,
		herd.WithDeps(deps...),
		herd.WithCallback(
			s.MountOP(
				fmt.Sprintf("/dev/disk/by-label/%s", runtime.OEM.Label),
				s.path("/oem"),
				runtime.OEM.Type,
				[]string{
					"rw",
					"suid",
					"dev",
					"exec",
					//"noauto",
					//"nouser",
					"async",
				}, 10*time.Second),
		),
	)
}

// MountBaseOverlayDagStep will add mounting /run/overlay as an overlay dir
func (s *State) MountBaseOverlayDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpMountBaseOverlay,
		herd.WithCallback(
			func(ctx context.Context) error {
				op, err := baseOverlay(Overlay{
					Base:        "/run/overlay",
					BackingBase: "tmpfs:20%",
				})
				if err != nil {
					return err
				}
				err2 := op.run()
				// No error, add fstab
				if err2 == nil {
					s.fstabs = append(s.fstabs, &op.FstabEntry)
					return nil
				}
				// Error but its already mounted error, dont add fstab but dont return error
				if err2 != nil && errors.Is(err2, cnst.ErrAlreadyMounted) {
					return nil
				}

				return err2
			},
		),
	)
}

// MountCustomOverlayDagStep will add mounting s.OverlayDirs under /run/overlay
func (s *State) MountCustomOverlayDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpOverlayMount,
		herd.WithDeps(cnst.OpLoadConfig),
		herd.WithCallback(
			func(ctx context.Context) error {
				var multierr *multierror.Error
				s.Logger.Debug().Strs("dirs", s.OverlayDirs).Msg("Mounting overlays")
				for _, p := range s.OverlayDirs {
					op := mountWithBaseOverlay(p, s.Rootdir, "/run/overlay")
					err := op.run()
					// Append to errors only if it's not an already mounted error
					if err != nil && !errors.Is(err, cnst.ErrAlreadyMounted) {
						log.Logger.Err(err).Msg("overlay mount")
						multierr = multierror.Append(multierr, err)
						continue
					}
					s.fstabs = append(s.fstabs, &op.FstabEntry)
				}
				return multierr.ErrorOrNil()
			},
		),
	)
}

// MountCustomMountsDagStep will add mounting s.CustomMounts
func (s *State) MountCustomMountsDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpCustomMounts,
		herd.WithDeps(cnst.OpLoadConfig),
		herd.WithCallback(func(ctx context.Context) error {
			var err *multierror.Error

			for what, where := range s.CustomMounts {
				// TODO: scan for the custom mount disk to know the underlying fs and set it proper
				fstype := "ext4"
				mountOptions := []string{"ro"}
				// Persistent needs to be RW
				if strings.Contains(what, "COS_PERSISTENT") {
					mountOptions = []string{"rw"}
				}
				err = multierror.Append(err, s.MountOP(
					what,
					s.path(where),
					fstype,
					mountOptions,
					10*time.Second,
				)(ctx))

			}
			s.Logger.Err(err.ErrorOrNil()).Send()

			return err.ErrorOrNil()
		}),
	)
}

// MountCustomBindsDagStep will add mounting s.BindMounts
// mount state is defined over a custom mount (/usr/local/.state for instance, needs to be mounted over a device)
func (s *State) MountCustomBindsDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpMountBind,
		herd.WithDeps(cnst.OpCustomMounts, cnst.OpLoadConfig),
		herd.WithCallback(
			func(ctx context.Context) error {
				var err *multierror.Error
				s.Logger.Debug().Strs("mounts", s.BindMounts).Msg("Mounting binds")

				for _, p := range s.BindMounts {
					op := mountBind(p, s.Rootdir, s.StateDir)
					err2 := op.run()
					if err2 == nil {
						// Only append to fstabs if there was no error, otherwise we will try to mount it after switch_root
						s.fstabs = append(s.fstabs, &op.FstabEntry)
					}
					// Append to errors only if it's not an already mounted error
					if err2 != nil && !errors.Is(err2, cnst.ErrAlreadyMounted) {
						log.Logger.Err(err2).Send()
						err = multierror.Append(err, err2)
					}
				}
				log.Logger.Err(err.ErrorOrNil()).Send()
				return err.ErrorOrNil()
			},
		),
	)
}

// WriteFstabDagStep will add writing the final fstab file with all the mounts
// Depends on everything but weak, so it will still try to write
func (s *State) WriteFstabDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpWriteFstab,
		herd.WithDeps(cnst.OpMountRoot, cnst.OpDiscoverState, cnst.OpLoadConfig, cnst.OpMountOEM, cnst.OpCustomMounts, cnst.OpMountBind, cnst.OpOverlayMount),
		herd.WeakDeps,
		herd.WithCallback(s.WriteFstab(s.path("/etc/fstab"))))
}

// WriteSentinelDagStep sets the sentinel file to identify the boot mode.
// This is used by several things to know in which state they are, for example cloud configs
func (s *State) WriteSentinelDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpSentinel,
		herd.WithCallback(func(ctx context.Context) error {
			var sentinel string

			err := internalUtils.CreateIfNotExists("/run/cos/")
			if err != nil {
				return err
			}
			runtime, err := state.NewRuntime()
			if err != nil {
				return err
			}

			switch runtime.BootState {
			case state.Active:
				sentinel = "active_mode"
			case state.Passive:
				sentinel = "passive_mode"
			case state.Recovery:
				sentinel = "recovery_mode"
			case state.LiveCD:
				sentinel = "live_mode"
			default:
				sentinel = string(state.Unknown)
			}

			// Workaround for runtime not detecting netboot as live_mode
			// Needs changes to the kairos sdk
			// TODO: drop once the netboot detection change is on the kairos sdk
			cmdline, err := os.ReadFile("/proc/cmdline")
			cmdlineS := string(cmdline)
			if strings.Contains(cmdlineS, "netboot") {
				sentinel = "live_mode"
			}

			s.Logger.Info().Str("to", sentinel).Msg("Setting sentinel file")
			err = os.WriteFile(filepath.Join("/run/cos/", sentinel), []byte("1"), os.ModePerm)
			if err != nil {
				return err
			}
			return nil
		}))
}
