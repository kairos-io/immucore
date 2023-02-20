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
	"github.com/rs/zerolog/log"
	"github.com/spectrocloud-labs/herd"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

	// 1 - mount the state partition to find the images (active/passive/recovery)
	err = g.Add(cnst.OpMountState,
		herd.WithCallback(
			s.MountOP(
				internalUtils.GetState(),
				s.path("/run/initramfs/cos-state"),
				internalUtils.DiskFSType(internalUtils.GetState()),
				[]string{
					s.RootMountMode,
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
				// Check if loop device is mounted already
				if internalUtils.IsMounted(s.TargetDevice) {
					log.Logger.Debug().Str("targetImage", s.TargetImage).Str("path", s.Rootdir).Str("TargetDevice", s.TargetDevice).Msg("Not mounting loop, already mounted")
					return nil
				}
				_ = internalUtils.Fsck(s.path("/run/initramfs/cos-state", s.TargetImage))
				cmd := fmt.Sprintf("losetup --show -f %s", s.path("/run/initramfs/cos-state", s.TargetImage))
				_, err := utils.SH(cmd)
				s.LogIfError(err, "losetup")
				// Trigger udevadm
				// On some systems the COS_ACTIVE/PASSIVE label is automatically shown as soon as we mount the device
				// But on other it seems like it won't trigger which causes the sysroot to not be mounted as we cant find
				// the block device by the target label. Make sure we run this after mounting so we refresh the devices.
				sh, _ := utils.SH("udevadm trigger")
				s.Logger.Debug().Str("output", sh).Msg("udevadm trigger")
				log.Logger.Debug().Str("targetImage", s.TargetImage).Str("path", s.Rootdir).Str("TargetDevice", s.TargetDevice).Msg("mount done")
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
				s.TargetDevice,
				s.Rootdir,
				"ext4", // TODO: Get this just in time? Currently if using DiskFSType is run immediately which is bad becuase its not mounted
				[]string{
					s.RootMountMode,
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

// InitramfsStageDagStep will add the rootfs stage.
func (s *State) InitramfsStageDagStep(g *herd.Graph, deps ...string) error {
	return g.Add(cnst.OpInitramfsHook, herd.WithDeps(deps...), herd.WithCallback(s.RunStageOp("initramfs")))
}

// LoadEnvLayoutDagStep will add the stage to load from cos-layout.env and fill the proper CustomMounts, OverlayDirs and BindMounts
func (s *State) LoadEnvLayoutDagStep(g *herd.Graph, deps ...string) error {
	return g.Add(cnst.OpLoadConfig,
		herd.WithDeps(deps...),
		herd.WithCallback(func(ctx context.Context) error {
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
					3*time.Second,
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

			// Workaround for runtime not detecting netboot/rd.cos.disable as live_mode
			// TODO: drop once the netboot/rd.cos.disable detection change is on the kairos sdk
			cmdline, err := os.ReadFile("/proc/cmdline")
			cmdlineS := string(cmdline)
			if strings.Contains(cmdlineS, "netboot") || len(internalUtils.ReadCMDLineArg("rd.cos.disable")) > 0 {
				sentinel = "live_mode"
			}

			s.Logger.Info().Str("to", sentinel).Msg("Setting sentinel file")
			err = os.WriteFile(filepath.Join("/run/cos/", sentinel), []byte("1"), os.ModePerm)
			if err != nil {
				return err
			}

			// Lets add a uki sentinel as well!
			if strings.Contains(cmdlineS, "rd.immucore.uki") {
				err = os.WriteFile("/run/cos/uki_mode", []byte("1"), os.ModePerm)
				if err != nil {
					return err
				}
			}

			return nil
		}))
}

// UKIBootInitDagStep tries to launch /sbin/init in root and pass over the system
// booting to the real init process
// Drops to emergency if not able to. Panic if it cant even launch emergency
func (s *State) UKIBootInitDagStep(g *herd.Graph, deps ...string) error {
	return g.Add(cnst.OpUkiInit,
		herd.WithDeps(deps...),
		herd.WithCallback(func(ctx context.Context) error {
			// Print dag before exit, otherwise its never printed as we never exit the program
			log.Info().Msg(s.WriteDAG(g))
			log.Logger.Debug().Msg("Executing init callback!")
			if err := unix.Exec("/sbin/init", []string{"/sbin/init", "--system"}, os.Environ()); err != nil {
				log.Logger.Err(err).Msg("running init")
				// drop to emergency shell
				if err := unix.Exec("/bin/bash", []string{"/bin/bash"}, os.Environ()); err != nil {
					log.Logger.Fatal().Msg("Could not drop to emergency shell")
				}
			}
			return nil
		}))
}

// UKIRemountRootRODagStep remount root read only
func (s *State) UKIRemountRootRODagStep(g *herd.Graph, deps ...string) error {
	return g.Add(cnst.OpRemountRootRO,
		herd.WithDeps(deps...),
		herd.WithCallback(func(ctx context.Context) error {
			return syscall.Mount("/", "/", "rootfs", syscall.MS_REMOUNT|syscall.MS_RDONLY, "")
		}),
	)
}

// UKIUdevDaemon launches the udevd daemon and triggers+settles in order to discover devices
// Needed if we expect to find devices by label...
func (s *State) UKIUdevDaemon(g *herd.Graph) error {
	return g.Add(cnst.OpUkiUdev,
		herd.WithCallback(func(ctx context.Context) error {
			// Should probably figure out other udevd binaries....
			var udevBin string
			if _, err := os.Stat("/usr/lib/systemd/systemd-udevd"); !os.IsNotExist(err) {
				udevBin = "/usr/lib/systemd/systemd-udevd"
			}
			cmd := fmt.Sprintf("%s --daemon", udevBin)
			out, err := internalUtils.CommandWithPath(cmd)
			log.Logger.Debug().Str("out", out).Str("cmd", cmd).Msg("Udev daemon")
			if err != nil {
				log.Logger.Err(err).Msg("Udev daemon")
				return err
			}
			out, err = internalUtils.CommandWithPath("udevadm trigger")
			log.Logger.Debug().Str("out", out).Msg("Udev trigger")
			if err != nil {
				log.Logger.Err(err).Msg("Udev trigger")
				return err
			}

			out, err = internalUtils.CommandWithPath("udevadm settle")
			log.Logger.Debug().Str("out", out).Msg("Udev settle")
			if err != nil {
				log.Logger.Err(err).Msg("Udev settle")
				return err
			}
			env := os.Environ()
			env = append(env, "PATH=/usr/bin:/usr/sbin")
			unix.Exec("/bin/bash", []string{"/bin/bash"}, env)
			return nil
		}),
	)
}
