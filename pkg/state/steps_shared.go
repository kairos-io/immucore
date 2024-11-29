package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	cnst "github.com/kairos-io/immucore/internal/constants"
	internalUtils "github.com/kairos-io/immucore/internal/utils"
	"github.com/kairos-io/immucore/pkg/op"
	"github.com/kairos-io/immucore/pkg/schema"
	"github.com/kairos-io/kairos-sdk/state"
	"github.com/spectrocloud-labs/herd"
)

// Shared steps for all the workflows

// WriteSentinelDagStep sets the sentinel file to identify the boot mode.
// This is used by several things to know in which state they are, for example cloud configs.
func (s *State) WriteSentinelDagStep(g *herd.Graph, deps ...string) error {
	return g.Add(cnst.OpSentinel,
		herd.WithDeps(deps...),
		herd.WithCallback(func(_ context.Context) error {
			var sentinel string

			internalUtils.Log.Debug().Msg("Will now create /run/cos is not exists")
			err := internalUtils.CreateIfNotExists("/run/cos/")
			if err != nil {
				internalUtils.Log.Err(err).Msg("failed to create /run/cos")
				return err
			}

			internalUtils.Log.Debug().Msg("Will now create the runtime object")
			runtime, err := state.NewRuntimeWithLogger(internalUtils.Log)
			if err != nil {
				return err
			}
			internalUtils.Log.Debug().Msg("Bootstate: " + string(runtime.BootState))

			switch runtime.BootState {
			case state.Active:
				sentinel = "active_mode"
			case state.Passive:
				sentinel = "passive_mode"
			case state.Recovery:
				sentinel = "recovery_mode"
			case state.AutoReset:
				sentinel = "autoreset_mode"
			case state.LiveCD:
				sentinel = "live_mode"
			default:
				sentinel = string(state.Unknown)
			}

			internalUtils.Log.Debug().Str("BootState", string(runtime.BootState)).Msg("The BootState was")

			internalUtils.Log.Info().Str("to", sentinel).Msg("Setting sentinel file")
			err = os.WriteFile(filepath.Join("/run/cos/", sentinel), []byte("1"), os.ModePerm)
			if err != nil {
				return err
			}

			// Lets add a uki sentinel as well!
			cmdline, _ := os.ReadFile(internalUtils.GetHostProcCmdline())
			if strings.Contains(string(cmdline), "rd.immucore.uki") {
				state.DetectUKIboot(string(cmdline))
				// sentinel for uki mode
				if state.EfiBootFromInstall(internalUtils.Log) {
					internalUtils.Log.Info().Str("to", "uki_boot_mode").Msg("Setting sentinel file")
					err = os.WriteFile("/run/cos/uki_boot_mode", []byte("1"), os.ModePerm)
					if err != nil {
						return err
					}
				} else {
					internalUtils.Log.Info().Str("to", "uki_install_mode").Msg("Setting sentinel file")
					err := os.WriteFile("/run/cos/uki_install_mode", []byte("1"), os.ModePerm)
					if err != nil {
						return err
					}
				}
			}

			return nil
		}))
}

// RootfsStageDagStep will add the rootfs stage.
func (s *State) RootfsStageDagStep(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add(cnst.OpRootfsHook, append(opts, herd.WithCallback(s.RunStageOp("rootfs")))...)
}

// InitramfsStageDagStep will add the rootfs stage.
func (s *State) InitramfsStageDagStep(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add(cnst.OpInitramfsHook, append(opts, herd.WithCallback(s.RunStageOp("initramfs")))...)
}

// RunStageOp runs elemental run-stage stage. If its rootfs its special as it needs som symlinks
// If its uki we don't symlink as we already have everything in the sysroot.
func (s *State) RunStageOp(stage string) func(context.Context) error {
	return func(_ context.Context) error {
		switch stage {
		case "rootfs":
			if !internalUtils.IsUKI() {
				if _, err := os.Stat("/system"); os.IsNotExist(err) {
					err = os.Symlink("/sysroot/system", "/system")
					if err != nil {
						internalUtils.Log.Err(err).Msg("creating symlink")
					}
				}
				if _, err := os.Stat("/oem"); os.IsNotExist(err) {
					err = os.Symlink("/sysroot/oem", "/oem")
					if err != nil {
						internalUtils.Log.Err(err).Msg("creating symlink")
					}
				}
			}

			internalUtils.Log.Info().Msg("Running rootfs stage")
			output, _ := internalUtils.RunStage("rootfs")
			internalUtils.Log.Debug().Msg(output.String())
			err := internalUtils.CreateIfNotExists(cnst.LogDir)
			if err != nil {
				return err
			}
			e := os.WriteFile(filepath.Join(cnst.LogDir, "rootfs_stage.log"), output.Bytes(), os.ModePerm)
			if e != nil {
				internalUtils.Log.Err(e).Msg("Writing log for rootfs stage")
			}
			return err
		case "initramfs":
			// Not sure if it will work under UKI where the s.Rootdir is the current root already
			internalUtils.Log.Info().Msg("Running initramfs stage")
			if internalUtils.IsUKI() {
				output, _ := internalUtils.RunStage("initramfs")
				internalUtils.Log.Debug().Msg(output.String())
				err := internalUtils.CreateIfNotExists(cnst.LogDir)
				if err != nil {
					return err
				}
				e := os.WriteFile(filepath.Join(cnst.LogDir, "initramfs_stage.log"), output.Bytes(), os.ModePerm)
				if e != nil {
					internalUtils.Log.Err(e).Msg("Writing log for initramfs stage")
				}
				return err
			} else {
				chroot := internalUtils.NewChroot(s.Rootdir)
				return chroot.RunCallback(func() error {
					output, _ := internalUtils.RunStage("initramfs")
					internalUtils.Log.Debug().Msg(output.String())
					err := internalUtils.CreateIfNotExists(cnst.LogDir)
					if err != nil {
						return err
					}
					e := os.WriteFile(filepath.Join(cnst.LogDir, "initramfs_stage.log"), output.Bytes(), os.ModePerm)
					if e != nil {
						internalUtils.Log.Err(e).Msg("Writing log for initramfs stage")
					}
					return err
				})
			}

		default:
			return errors.New("no stage that we know off")
		}
	}
}

// LoadEnvLayoutDagStep will add the stage to load from cos-layout.env and fill the proper CustomMounts, OverlayDirs and BindMounts.
func (s *State) LoadEnvLayoutDagStep(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add(cnst.OpLoadConfig,
		append(opts, herd.WithDeps(cnst.OpRootfsHook),
			herd.WithCallback(func(_ context.Context) error {
				c, _ := internalUtils.CommandWithPath("stat /sysroot")
				internalUtils.Log.Info().Str("path", c).Msg("Sysroot status before loading env")
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

				c, _ = internalUtils.CommandWithPath("stat /sysroot")
				internalUtils.Log.Info().Str("path", c).Msg("Sysroot status after loading env")

				return nil
			}))...)
}

// MountOemDagStep will add mounting COS_OEM partition under s.Rootdir + /oem .
func (s *State) MountOemDagStep(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add(cnst.OpMountOEM,
		append(opts,
			herd.WithCallback(func(ctx context.Context) error {
				runtime, _ := state.NewRuntimeWithLogger(internalUtils.Log)
				if runtime.BootState == state.LiveCD {
					internalUtils.Log.Debug().Msg("Livecd mode detected, won't mount OEM")
					return nil
				}
				if internalUtils.GetOemLabel() == "" {
					internalUtils.Log.Debug().Msg("OEM label from cmdline empty, won't mount OEM")
					return nil
				}
				op := func(_ context.Context) error {
					fstab, err := op.MountOPWithFstab(
						fmt.Sprintf("/dev/disk/by-label/%s", internalUtils.GetOemLabel()),
						s.path("/oem"),
						internalUtils.DiskFSType(fmt.Sprintf("/dev/disk/by-label/%s", internalUtils.GetOemLabel())),
						[]string{
							"rw",
							"suid",
							"dev",
							"exec",
							"async",
						}, time.Duration(internalUtils.GetOemTimeout())*time.Second)
					for _, f := range fstab {
						s.fstabs = append(s.fstabs, f)
					}
					return err
				}
				return op(ctx)
			}))...)
}

// MountBaseOverlayDagStep will add mounting /run/overlay as an overlay dir
// Requires the config-load step because some parameters can come from there.
func (s *State) MountBaseOverlayDagStep(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add(cnst.OpMountBaseOverlay,
		append(opts, herd.WithDeps(cnst.OpLoadConfig),
			herd.WithCallback(
				func(_ context.Context) error {
					c, _ := internalUtils.CommandWithPath("stat /sysroot")
					internalUtils.Log.Info().Str("path", c).Msg("Sysroot status before mounting base overlay")
					operation, err := op.BaseOverlay(schema.Overlay{
						Base:        "/run/overlay",
						BackingBase: s.OverlayBase,
					})
					if err != nil {
						return err
					}
					err2 := operation.Run()
					// No error, add fstab
					if err2 == nil {
						s.fstabs = append(s.fstabs, &operation.FstabEntry)
						return nil
					}
					// Error but its already mounted error, dont add fstab but dont return error
					if err2 != nil && errors.Is(err2, cnst.ErrAlreadyMounted) {
						return nil
					}
					c, _ = internalUtils.CommandWithPath("stat /sysroot")
					internalUtils.Log.Info().Str("path", c).Msg("Sysroot status after mounting base overlay")
					return err2
				},
			),
		)...)
}

// MountCustomOverlayDagStep will add mounting s.OverlayDirs under /run/overlay .
func (s *State) MountCustomOverlayDagStep(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add(cnst.OpOverlayMount,
		append(opts, herd.WithDeps(cnst.OpLoadConfig, cnst.OpMountBaseOverlay),
			herd.WithCallback(
				func(_ context.Context) error {
					var multierr *multierror.Error
					internalUtils.Log.Debug().Strs("dirs", s.OverlayDirs).Msg("Mounting overlays")
					for _, p := range s.OverlayDirs {
						internalUtils.Log.Debug().Str("what", p).Msg("Overlay mount start")
						op := op.MountWithBaseOverlay(p, s.Rootdir, "/run/overlay")
						err := op.Run()
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
		)...)
}

// MountCustomMountsDagStep will add mounting s.CustomMounts .
func (s *State) MountCustomMountsDagStep(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add(cnst.OpCustomMounts, append(opts, herd.WithDeps(cnst.OpLoadConfig),
		herd.WithCallback(func(_ context.Context) error {
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
				fstab, err2 := op.MountOPWithFstab(
					what,
					s.path(where),
					fstype,
					mountOptions,
					3*time.Second,
				)
				for _, f := range fstab {
					s.fstabs = append(s.fstabs, f)
				}

				// If its COS_OEM and it fails then we can safely ignore, as it's not mandatory to have COS_OEM
				if err2 != nil && !strings.Contains(what, "COS_OEM") {
					err = multierror.Append(err, err2)
				}
				internalUtils.Log.Debug().Str("what", what).Str("where", where).Msg("Custom mount done")
			}
			internalUtils.Log.Warn().Err(err.ErrorOrNil()).Send()

			return err.ErrorOrNil()
		}),
	)...)
}

// MountCustomBindsDagStep will add mounting s.BindMounts
// mount state is defined over a custom mount (/usr/local/.state for instance, needs to be mounted over a device).
func (s *State) MountCustomBindsDagStep(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add(cnst.OpMountBind,
		append(opts, herd.WithDeps(cnst.OpOverlayMount, cnst.OpCustomMounts, cnst.OpLoadConfig),
			herd.WithCallback(
				func(_ context.Context) error {
					c, _ := internalUtils.CommandWithPath("stat /sysroot")
					internalUtils.Log.Info().Str("path", c).Msg("Sysroot status before mounting binds")
					var err *multierror.Error
					internalUtils.Log.Debug().Strs("mounts", s.BindMounts).Msg("Mounting binds")

					for _, p := range s.SortedBindMounts() {
						internalUtils.Log.Debug().Str("what", p).Msg("Bind mount start")
						op := op.MountBind(p, s.Rootdir, s.StateDir)
						err2 := op.Run()
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
					internalUtils.Log.Warn().Err(err.ErrorOrNil()).Send()
					c, _ = internalUtils.CommandWithPath("stat /sysroot")
					internalUtils.Log.Info().Str("path", c).Msg("Sysroot status after mounting binds")
					return err.ErrorOrNil()
				},
			),
		)...)
}

// WriteFstabDagStep will add writing the final fstab file with all the mounts
// Depends on everything but weak, so it will still try to write.
func (s *State) WriteFstabDagStep(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add(cnst.OpWriteFstab, append(opts, herd.WithCallback(s.WriteFstab()))...)
}
