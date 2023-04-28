package mount

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hashicorp/go-multierror"
	cnst "github.com/kairos-io/immucore/internal/constants"
	internalUtils "github.com/kairos-io/immucore/internal/utils"
	"github.com/kairos-io/kairos-sdk/state"
	"github.com/kairos-io/kairos-sdk/utils"
	"github.com/mudler/go-kdetect"
	"github.com/spectrocloud-labs/herd"
	"golang.org/x/sys/unix"
)

// MountTmpfsDagStep adds the step to mount /tmp .
func (s *State) MountTmpfsDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpMountTmpfs, herd.WithCallback(s.MountOP("tmpfs", "/tmp", "tmpfs", []string{"rw"}, 10*time.Second)))
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
		internalUtils.Log.Err(err).Send()
	}

	// 2 - mount the image as a loop device
	err = g.Add(cnst.OpDiscoverState,
		herd.WithDeps(cnst.OpMountState),
		herd.WithCallback(
			func(ctx context.Context) error {
				// Check if loop device is mounted already
				if internalUtils.IsMounted(s.TargetDevice) {
					internalUtils.Log.Debug().Str("targetImage", s.TargetImage).Str("path", s.Rootdir).Str("TargetDevice", s.TargetDevice).Msg("Not mounting loop, already mounted")
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
				internalUtils.Log.Debug().Str("output", sh).Msg("udevadm trigger")
				internalUtils.Log.Debug().Str("targetImage", s.TargetImage).Str("path", s.Rootdir).Str("TargetDevice", s.TargetDevice).Msg("mount done")
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
			s.MountOP(
				s.TargetDevice,
				s.Rootdir,
				"ext4", // TODO: Get this just in time? Currently if using DiskFSType is run immediately which is bad because its not mounted
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
		internalUtils.Log.Err(err).Send()
	}
	return err
}

// RootfsStageDagStep will add the rootfs stage.
func (s *State) RootfsStageDagStep(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add(cnst.OpRootfsHook, append(opts, herd.WithCallback(s.RunStageOp("rootfs")))...)
}

// InitramfsStageDagStep will add the rootfs stage.
func (s *State) InitramfsStageDagStep(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add(cnst.OpInitramfsHook, append(opts, herd.WithCallback(s.RunStageOp("initramfs")))...)
}

// LoadEnvLayoutDagStep will add the stage to load from cos-layout.env and fill the proper CustomMounts, OverlayDirs and BindMounts.
func (s *State) LoadEnvLayoutDagStep(g *herd.Graph, deps ...string) error {
	return g.Add(cnst.OpLoadConfig,
		herd.WithDeps(deps...),
		herd.WithCallback(func(ctx context.Context) error {
			if s.CustomMounts == nil {
				s.CustomMounts = map[string]string{}
			}

			env, err := internalUtils.ReadEnv("/run/cos/cos-layout.env")
			if err != nil {
				internalUtils.Log.Err(err).Msg("Reading env")
				return err
			}
			// populate from env here
			s.OverlayDirs = internalUtils.CleanupSlice(strings.Split(env["RW_PATHS"], " "))
			// Append default RW_Paths if list is empty, otherwise we won't boot properly
			if len(s.OverlayDirs) == 0 {
				s.OverlayDirs = cnst.DefaultRWPaths()
			}

			// Remove any duplicates
			s.OverlayDirs = internalUtils.UniqueSlice(internalUtils.CleanupSlice(s.OverlayDirs))

			s.BindMounts = strings.Split(env["PERSISTENT_STATE_PATHS"], " ")
			// Add custom bind mounts
			s.BindMounts = append(s.BindMounts, strings.Split(env["CUSTOM_BIND_MOUNTS"], " ")...)
			// Remove any duplicates
			s.BindMounts = internalUtils.UniqueSlice(internalUtils.CleanupSlice(s.BindMounts))

			// Load Overlay config
			overlayConfig := env["OVERLAY"]
			if overlayConfig != "" {
				s.OverlayBase = overlayConfig
			}

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
			// Parse custom mounts also from cmdline (rd.immucore.mount=)
			// Parse custom mounts also from env file (VOLUMES)

			for _, v := range append(append(internalUtils.ReadCMDLineArg("rd.cos.mount="), internalUtils.ReadCMDLineArg("rd.immucore.mount=")...), strings.Split(env["VOLUMES"], " ")...) {
				addLine(internalUtils.ParseMount(v))
			}

			return nil
		}))
}

// MountOemDagStep will add mounting COS_OEM partition under s.Rootdir + /oem .
func (s *State) MountOemDagStep(g *herd.Graph, deps ...string) error {
	return g.Add(cnst.OpMountOEM,
		herd.WithDeps(deps...),
		herd.EnableIf(func() bool {
			runtime, _ := state.NewRuntime()
			switch runtime.BootState {
			// Don't run this on LiveCD/Netboot
			case state.LiveCD:
				return false
			default:
				return internalUtils.GetOemLabel() != ""
			}
		}),
		herd.WithCallback(
			s.MountOP(
				fmt.Sprintf("/dev/disk/by-label/%s", internalUtils.GetOemLabel()),
				s.path("/oem"),
				internalUtils.DiskFSType(fmt.Sprintf("/dev/disk/by-label/%s", internalUtils.GetOemLabel())),
				[]string{
					"rw",
					"suid",
					"dev",
					"exec",
					"async",
				}, time.Duration(internalUtils.GetOemTimeout())*time.Second),
		),
	)
}

// MountBaseOverlayDagStep will add mounting /run/overlay as an overlay dir
// Requires the config-load step because some parameters can come from there.
func (s *State) MountBaseOverlayDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpMountBaseOverlay,
		herd.WithDeps(cnst.OpLoadConfig),
		herd.WithCallback(
			func(ctx context.Context) error {
				op, err := baseOverlay(Overlay{
					Base:        "/run/overlay",
					BackingBase: s.OverlayBase,
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

// MountCustomOverlayDagStep will add mounting s.OverlayDirs under /run/overlay .
func (s *State) MountCustomOverlayDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpOverlayMount,
		herd.WithDeps(cnst.OpLoadConfig, cnst.OpMountBaseOverlay),
		herd.WithCallback(
			func(ctx context.Context) error {
				var multierr *multierror.Error
				internalUtils.Log.Debug().Strs("dirs", s.OverlayDirs).Msg("Mounting overlays")
				for _, p := range s.OverlayDirs {
					internalUtils.Log.Debug().Str("what", p).Msg("Overlay mount start")
					op := mountWithBaseOverlay(p, s.Rootdir, "/run/overlay")
					err := op.run()
					// Append to errors only if it's not an already mounted error
					if err != nil && !errors.Is(err, cnst.ErrAlreadyMounted) {
						internalUtils.Log.Err(err).Msg("overlay mount")
						multierr = multierror.Append(multierr, err)
						continue
					}
					s.fstabs = append(s.fstabs, &op.FstabEntry)
					internalUtils.Log.Debug().Str("what", p).Msg("Overlay mount done")
				}
				return multierr.ErrorOrNil()
			},
		),
	)
}

// MountCustomMountsDagStep will add mounting s.CustomMounts .
func (s *State) MountCustomMountsDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpCustomMounts,
		herd.WithDeps(cnst.OpLoadConfig),
		herd.WithCallback(func(ctx context.Context) error {
			var err *multierror.Error
			internalUtils.Log.Debug().Interface("mounts", s.CustomMounts).Msg("Mounting custom mounts")

			for what, where := range s.CustomMounts {
				internalUtils.Log.Debug().Str("what", what).Str("where", where).Msg("Custom mount start")
				// TODO: scan for the custom mount disk to know the underlying fs and set it proper
				fstype := "ext4"
				mountOptions := []string{"ro"}
				// TODO: Are custom mounts always rw?ro?depends? Clarify.
				// Persistent needs to be RW
				if strings.Contains(what, "COS_PERSISTENT") {
					mountOptions = []string{"rw"}
				}
				err2 := s.MountOP(
					what,
					s.path(where),
					fstype,
					mountOptions,
					3*time.Second,
				)(ctx)

				// If its COS_OEM and it fails then we can safely ignore, as it's not mandatory to have COS_OEM
				if err2 != nil && !strings.Contains(what, "COS_OEM") {
					err = multierror.Append(err, err2)
				}
				internalUtils.Log.Debug().Str("what", what).Str("where", where).Msg("Custom mount done")
			}
			internalUtils.Log.Err(err.ErrorOrNil()).Send()

			return err.ErrorOrNil()
		}),
	)
}

// MountCustomBindsDagStep will add mounting s.BindMounts
// mount state is defined over a custom mount (/usr/local/.state for instance, needs to be mounted over a device).
func (s *State) MountCustomBindsDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpMountBind,
		herd.WithDeps(cnst.OpOverlayMount, cnst.OpCustomMounts, cnst.OpLoadConfig),
		herd.WithCallback(
			func(ctx context.Context) error {
				var err *multierror.Error
				internalUtils.Log.Debug().Strs("mounts", s.BindMounts).Msg("Mounting binds")

				for _, p := range s.BindMounts {
					internalUtils.Log.Debug().Str("what", p).Msg("Bind mount start")
					op := mountBind(p, s.Rootdir, s.StateDir)
					err2 := op.run()
					if err2 == nil {
						// Only append to fstabs if there was no error, otherwise we will try to mount it after switch_root
						s.fstabs = append(s.fstabs, &op.FstabEntry)
					}
					// Append to errors only if it's not an already mounted error
					if err2 != nil && !errors.Is(err2, cnst.ErrAlreadyMounted) {
						internalUtils.Log.Err(err2).Send()
						err = multierror.Append(err, err2)
					}
					internalUtils.Log.Debug().Str("what", p).Msg("Bind mount end")
				}
				internalUtils.Log.Err(err.ErrorOrNil()).Send()
				return err.ErrorOrNil()
			},
		),
	)
}

// WriteFstabDagStep will add writing the final fstab file with all the mounts
// Depends on everything but weak, so it will still try to write.
func (s *State) WriteFstabDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpWriteFstab,
		herd.WithDeps(cnst.OpMountRoot, cnst.OpDiscoverState, cnst.OpLoadConfig),
		herd.WithWeakDeps(cnst.OpMountOEM, cnst.OpCustomMounts, cnst.OpMountBind, cnst.OpOverlayMount),
		herd.WithCallback(s.WriteFstab(s.path("/etc/fstab"))))
}

// WriteSentinelDagStep sets the sentinel file to identify the boot mode.
// This is used by several things to know in which state they are, for example cloud configs.
func (s *State) WriteSentinelDagStep(g *herd.Graph, deps ...string) error {
	return g.Add(cnst.OpSentinel,
		herd.WithDeps(deps...),
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

			internalUtils.Log.Info().Str("to", sentinel).Msg("Setting sentinel file")
			err = os.WriteFile(filepath.Join("/run/cos/", sentinel), []byte("1"), os.ModePerm)
			if err != nil {
				return err
			}

			// Lets add a uki sentinel as well!
			cmdline, _ := os.ReadFile(internalUtils.GetHostProcCmdline())
			if strings.Contains(string(cmdline), "rd.immucore.uki") {
				err = os.WriteFile("/run/cos/uki_mode", []byte("1"), os.ModePerm)
				if err != nil {
					return err
				}
			}

			return nil
		}))
}

func (s *State) UKIMountBaseSystem(g *herd.Graph) error {
	type mount struct {
		where string
		what  string
		fs    string
		flags uintptr
		data  string
	}

	return g.Add(
		cnst.OpUkiBaseMounts,
		herd.WithCallback(
			func(ctx context.Context) error {
				var err error
				mounts := []mount{
					{
						"/run",
						"tmpfs",
						"tmpfs",
						syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC | syscall.MS_RELATIME,
						"mode=755",
					},
					{
						"/sys",
						"sysfs",
						"sysfs",
						syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC | syscall.MS_RELATIME,
						"",
					},
					{
						"/dev",
						"devtmpfs",
						"devtmpfs",
						syscall.MS_NOSUID,
						"mode=755",
					},
					{
						"/tmp",
						"tmpfs",
						"tmpfs",
						syscall.MS_NOSUID | syscall.MS_NODEV,
						"",
					},
				}
				for _, m := range mounts {
					e := os.MkdirAll(m.where, 0755)
					if e != nil {
						err = multierror.Append(err, e)
						internalUtils.Log.Err(e).Msg("Creating dir")
					}
					e = syscall.Mount(m.what, m.where, m.fs, m.flags, m.data)
					if e != nil {
						err = multierror.Append(err, e)
						internalUtils.Log.Err(e).Str("what", m.what).Str("where", m.where).Str("type", m.fs).Msg("Mounting")
					}
				}
				return err
			},
		),
	)
}

// UKIBootInitDagStep tries to launch /sbin/init in root and pass over the system
// booting to the real init process
// Drops to emergency if not able to. Panic if it cant even launch emergency.
func (s *State) UKIBootInitDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpUkiInit,
		herd.WithDeps(),
		herd.WithWeakDeps(cnst.OpRemountRootRO, cnst.OpRootfsHook, cnst.OpInitramfsHook, cnst.OpWriteFstab),
		herd.WithCallback(func(ctx context.Context) error {
			// Print dag before exit, otherwise its never printed as we never exit the program
			internalUtils.Log.Info().Msg(s.WriteDAG(g))
			internalUtils.Log.Debug().Msg("Executing init callback!")
			internalUtils.CloseLogFiles()
			if err := unix.Exec("/sbin/init", []string{"/sbin/init", "--system"}, os.Environ()); err != nil {
				internalUtils.Log.Err(err).Msg("running init")
				// drop to emergency shell
				if err := unix.Exec("/bin/bash", []string{"/bin/bash"}, os.Environ()); err != nil {
					internalUtils.Log.Fatal().Msg("Could not drop to emergency shell")
				}
			}
			return nil
		}))
}

// UKIRemountRootRODagStep remount root read only.
func (s *State) UKIRemountRootRODagStep(g *herd.Graph) error {
	return g.Add(cnst.OpRemountRootRO,
		herd.WithDeps(cnst.OpRootfsHook),
		herd.WithCallback(func(ctx context.Context) error {
			var err error
			for i := 1; i < 5; i++ {
				time.Sleep(1 * time.Second)
				// Should we try to stop udev here?
				err = syscall.Mount("", "/", "", syscall.MS_REMOUNT|syscall.MS_RDONLY, "")
				if err != nil {
					continue
				}
			}
			return err
		}),
	)
}

// UKIUdevDaemon launches the udevd daemon and triggers+settles in order to discover devices
// Needed if we expect to find devices by label...
func (s *State) UKIUdevDaemon(g *herd.Graph) error {
	return g.Add(cnst.OpUkiUdev,
		herd.WithDeps(cnst.OpUkiBaseMounts, cnst.OpUkiKernelModules),
		herd.WithCallback(func(ctx context.Context) error {
			// Should probably figure out other udevd binaries....
			var udevBin string
			if _, err := os.Stat("/usr/lib/systemd/systemd-udevd"); !os.IsNotExist(err) {
				udevBin = "/usr/lib/systemd/systemd-udevd"
			}
			cmd := fmt.Sprintf("%s --daemon", udevBin)
			out, err := internalUtils.CommandWithPath(cmd)
			internalUtils.Log.Debug().Str("out", out).Str("cmd", cmd).Msg("Udev daemon")
			if err != nil {
				internalUtils.Log.Err(err).Msg("Udev daemon")
				return err
			}
			out, err = internalUtils.CommandWithPath("udevadm trigger")
			internalUtils.Log.Debug().Str("out", out).Msg("Udev trigger")
			if err != nil {
				internalUtils.Log.Err(err).Msg("Udev trigger")
				return err
			}

			out, err = internalUtils.CommandWithPath("udevadm settle")
			internalUtils.Log.Debug().Str("out", out).Msg("Udev settle")
			if err != nil {
				internalUtils.Log.Err(err).Msg("Udev settle")
				return err
			}
			return nil
		}),
	)
}

// LoadKernelModules loads kernel modules needed during uki boot to load the disks for.
// Mainly block devices and net devices
// probably others down the line.
func (s *State) LoadKernelModules(g *herd.Graph) error {
	return g.Add(cnst.OpUkiKernelModules,
		herd.WithDeps(cnst.OpUkiBaseMounts),
		herd.WithCallback(func(ctx context.Context) error {
			drivers, err := kdetect.ProbeKernelModules("")
			if err != nil {
				internalUtils.Log.Err(err).Msg("Detecting needed modules")
			}
			internalUtils.Log.Debug().Strs("drivers", drivers).Msg("Detecting needed modules")
			for _, driver := range drivers {
				cmd := fmt.Sprintf("modprobe %s", driver)
				out, err := internalUtils.CommandWithPath(cmd)
				if err != nil {
					internalUtils.Log.Err(err).Str("out", out).Msg("modprobe")
				}
			}
			return nil
		}),
	)
}

// WaitForSysrootDagStep waits for the s.Rootdir and s.Rootdir/system paths to be there
// Useful for livecd/netboot as we want to run steps after s.Rootdir is ready but we don't mount it ourselves.
func (s *State) WaitForSysrootDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpWaitForSysroot,
		herd.WithCallback(func(ctx context.Context) error {
			cc := time.After(60 * time.Second)
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

// LvmActivation will try to activate lvm volumes/groups on the system.
func (s *State) LVMActivation(g *herd.Graph) error {
	return g.Add(cnst.OpLvmActivate, herd.WithCallback(func(ctx context.Context) error {
		return internalUtils.ActivateLVM()
	}))
}
