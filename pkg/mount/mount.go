package mount

import (
	"context"
	"errors"
	"fmt"
	"github.com/kairos-io/immucore/internal/constants"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/containerd/mount"
	"github.com/deniswernert/go-fstab"
	"github.com/hashicorp/go-multierror"
	internalUtils "github.com/kairos-io/immucore/internal/utils"
	"github.com/kairos-io/kairos/pkg/utils"
	"github.com/kairos-io/kairos/sdk/state"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spectrocloud-labs/herd"
)

type State struct {
	Logger      zerolog.Logger
	Rootdir     string // e.g. /sysroot inside initrd with pivot, / with nopivot
	TargetImage string // e.g. /cOS/active.img
	TargetLabel string // e.g. COS_ACTIVE

	// /run/cos-layout.env (different!)
	OverlayDirs  []string          // e.g. /var
	BindMounts   []string          // e.g. /etc/kubernetes
	CustomMounts map[string]string // e.g. diskid : mountpoint

	StateDir  string // e.g. "/usr/local/.state"
	MountRoot bool   // e.g. if true, it tries to find the image to loopback mount

	fstabs []*fstab.Mount
}

const (
	opCustomMounts  = "custom-mount"
	opDiscoverState = "discover-state"
	opMountState    = "mount-state"
	opMountBind     = "mount-bind"

	opMountRoot        = "mount-root"
	opOverlayMount     = "overlay-mount"
	opWriteFstab       = "write-fstab"
	opMountBaseOverlay = "mount-base-overlay"
	opMountOEM         = "mount-oem"

	opRootfsHook = "rootfs-hook"
	opLoadConfig = "load-config"
)

func (s *State) path(p ...string) string {
	return filepath.Join(append([]string{s.Rootdir}, p...)...)
}

func (s *State) WriteFstab(fstabFile string) func(context.Context) error {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
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
				if _, err := f.WriteString(fmt.Sprintf("%s\n", fst.String())); err != nil {
					_ = f.Close()
					return err
				}
				_ = f.Close()
			}
		}
		return nil
	}
}

func (s *State) RunStageOp(stage string) func(context.Context) error {
	return func(ctx context.Context) error {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
		if stage == "rootfs" {
			if _, err := os.Stat("/system"); os.IsNotExist(err) {
				err = os.Symlink("/sysroot/system", "/system")
				if err != nil {
					s.Logger.Err(err).Msg("creating symlink")
					return err
				}
			}
		}

		cmd := fmt.Sprintf("elemental run-stage %s", stage)
		// If we set the level to debug, also call elemental with debug
		if log.Logger.GetLevel() == zerolog.DebugLevel {
			cmd = fmt.Sprintf("%s --debug", cmd)
		}
		output, err := utils.SH(cmd)
		log.Debug().Msg(output)
		return err
	}
}

func (s *State) MountOP(what, where, t string, options []string, timeout time.Duration) func(context.Context) error {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()

	return func(c context.Context) error {
		cc := time.After(timeout)
		for {
			select {
			default:
				err := internalUtils.CreateIfNotExists(where)
				if err != nil {
					log.Logger.Debug().Str("what", what).Str("where", where).Str("type", t).Strs("options", options).Err(err).Msg("Creating dir")
					continue
				}
				time.Sleep(1 * time.Second)
				mountPoint := mount.Mount{
					Type:    t,
					Source:  what,
					Options: options,
				}
				tmpFstab := internalUtils.MountToFstab(mountPoint)
				tmpFstab.File = internalUtils.CleanSysrootForFstab(where)
				op := mountOperation{
					MountOption: mountPoint,
					FstabEntry:  *tmpFstab,
					Target:      where,
				}

				err = op.run()

				if err == nil {
					s.fstabs = append(s.fstabs, tmpFstab)
				}

				// only continue the loop if it's an error and not an already mounted error
				if err != nil && !errors.Is(err, constants.ErrAlreadyMounted) {
					continue
				}

				return nil
			case <-c.Done():
				e := fmt.Errorf("context canceled")
				log.Logger.Debug().Str("what", what).Str("where", where).Str("type", t).Strs("options", options).Err(e).Msg("mount canceled")
				return e
			case <-cc:
				e := fmt.Errorf("timeout exhausted")
				log.Logger.Debug().Str("what", what).Str("where", where).Str("type", t).Strs("options", options).Err(e).Msg("Mount timeout")
				return e
			}
		}
	}
}

func (s *State) WriteDAG(g *herd.Graph) (out string) {
	for i, layer := range g.Analyze() {
		out += fmt.Sprintf("%d.\n", i+1)
		for _, op := range layer {
			if op.Error != nil {
				out += fmt.Sprintf(" <%s> (error: %s) (background: %t) (weak: %t)\n", op.Name, op.Error.Error(), op.Background, op.WeakDeps)
			} else {
				out += fmt.Sprintf(" <%s> (background: %t) (weak: %t)\n", op.Name, op.Background, op.WeakDeps)
			}
		}
	}
	return
}

func (s *State) Register(g *herd.Graph) error {
	var err error

	runtime, err := state.NewRuntime()
	if err != nil {
		s.Logger.Debug().Err(err).Msg("")
		return err
	}

	// TODO: add hooks, fstab (might have missed some), systemd compat
	// TODO: We should also set tmpfs here (not -related)

	// All of this below need to run after rootfs stage runs (so the layout file is created)
	// This is legacy - in UKI we don't need to found the img, this needs to run in a conditional
	if s.MountRoot {
		// setup loopback mount for the image target for booting
		err = g.Add(opDiscoverState,
			herd.WithDeps(opMountState),
			herd.WithCallback(
				func(ctx context.Context) error {
					log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
					// Check if loop device is mounted by checking the existance of the target label
					if internalUtils.IsMountedByLabel(s.TargetLabel) {
						log.Logger.Debug().Str("targetImage", s.TargetImage).Str("path", s.Rootdir).Str("TargetLabel", s.TargetLabel).Msg("Not mounting loop, already mounted")
						return nil
					}
					cmd := fmt.Sprintf("losetup --show -f %s", s.path("/run/initramfs/cos-state", s.TargetImage))
					_, err := utils.SH(cmd)
					if err != nil {
						log.Logger.Debug().Err(err).Msg("")
					}
					return err
				},
			))
		if err != nil {
			s.Logger.Err(err).Send()
		}

		// mount the state partition so to find the loopback device
		stateName := runtime.State.Name
		stateFs := runtime.State.Type
		// Recovery is a different partition
		if internalUtils.IsRecovery() {
			stateName = runtime.Recovery.Name
			stateFs = runtime.Recovery.Type
		}
		err = g.Add(opMountState,
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

		// mount the loopback device as root of the fs
		err = g.Add(opMountRoot,
			herd.WithDeps(opDiscoverState),
			herd.WithCallback(
				s.MountOP(
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

	}

	// depending on /run/cos-layout.env
	// This is building the mountRoot dependendency if it was enabled
	mountRootCondition := herd.ConditionalOption(func() bool { return s.MountRoot }, herd.WithDeps(opMountRoot))

	// this needs to be run after sysroot, so we can link to /sysroot/system/oem and after /oem mounted
	err = g.Add(opRootfsHook, mountRootCondition, herd.WithDeps(opMountRoot, opMountOEM), herd.WithCallback(s.RunStageOp("rootfs")))
	if err != nil {
		s.Logger.Err(err).Msg("running rootfs stage")
	}

	// /run/cos/cos-layout.env
	// populate state bindmounts, overlaymounts, custommounts
	err = g.Add(opLoadConfig,
		herd.WithDeps(opRootfsHook),
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
			s.OverlayDirs = strings.Split(env["RW_PATHS"], " ")
			// If empty, then set defaults
			if len(s.OverlayDirs) == 0 {
				s.OverlayDirs = constants.DefaultRWPaths()
			}
			// Remove any duplicates
			s.OverlayDirs = internalUtils.UniqueSlice(s.OverlayDirs)

			s.BindMounts = strings.Split(env["PERSISTENT_STATE_PATHS"], " ")
			// Remove any duplicates
			s.BindMounts = internalUtils.UniqueSlice(s.BindMounts)

			s.StateDir = env["PERSISTENT_STATE_TARGET"]
			if s.StateDir == "" {
				s.StateDir = constants.PersistentStateTarget
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
	if err != nil {
		s.Logger.Err(err).Send()
	}
	// end sysroot mount

	// overlay mount start
	if internalUtils.DiskFSType(s.Rootdir) != "overlay" {
		err = g.Add(opMountBaseOverlay,
			herd.WithCallback(
				func(ctx context.Context) error {
					op, err := baseOverlay(Overlay{
						Base:        "/run/overlay",
						BackingBase: "tmpfs:20%",
					})
					if err != nil {
						return err
					}
					s.fstabs = append(s.fstabs, &op.FstabEntry)
					err2 := op.run()
					// Don't return error if it's an already mounted error
					log.Logger.Err(err2).Send()
					if err2 != nil && errors.Is(err2, constants.ErrAlreadyMounted) {
						return nil
					}
					return err2
				},
			),
		)
		if err != nil {
			s.Logger.Err(err).Send()
		}
	}

	overlayCondition := herd.ConditionalOption(func() bool { return internalUtils.DiskFSType(s.Rootdir) != "overlay" }, herd.WithDeps(opMountBaseOverlay))
	// TODO: Add fsck
	// mount overlay
	err = g.Add(
		opOverlayMount,
		overlayCondition,
		herd.WithDeps(opLoadConfig),
		mountRootCondition,
		herd.WithCallback(
			func(ctx context.Context) error {
				var multierr *multierror.Error
				for _, p := range s.OverlayDirs {
					op, err1 := mountWithBaseOverlay(p, s.Rootdir, "/run/overlay")
					if err1 != nil {
						log.Logger.Err(err1).Msg("mountWithBaseOverlay")
						return err1
					}
					s.fstabs = append(s.fstabs, &op.FstabEntry)
					err2 := op.run()
					// Append to errors only if it's not an already mounted error
					if err2 != nil && !errors.Is(err2, constants.ErrAlreadyMounted) {
						log.Logger.Err(err2).Msg("overlay mount")
						multierr = multierror.Append(multierr, err2)
					}
				}
				return multierr.ErrorOrNil()
			},
		),
	)
	if err != nil {
		s.Logger.Err(err).Send()
	}
	err = g.Add(
		opCustomMounts,
		mountRootCondition,
		overlayCondition,
		herd.WithDeps(opLoadConfig),
		herd.WithCallback(func(ctx context.Context) error {
			var err *multierror.Error

			for what, where := range s.CustomMounts {
				// TODO: scan for the custom mount disk to know the underlying fs and set it proper
				fstype := "ext4"
				mountOptions := []string{"ro"}
				// Translate label to disk for COS_PERSISTENT
				// Persistent needs to be RW
				if strings.Contains(what, "COS_PERSISTENT") {
					fstype = runtime.Persistent.Type
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
	if err != nil {
		s.Logger.Err(err).Send()
	}

	// mount state is defined over a custom mount (/usr/local/.state for instance, needs to be mounted over a device)
	err = g.Add(
		opMountBind,
		overlayCondition,
		mountRootCondition,
		herd.WithDeps(opCustomMounts, opLoadConfig),
		herd.WithCallback(
			func(ctx context.Context) error {
				var err *multierror.Error
				for _, p := range s.BindMounts {
					// Ignore empty values that can get there by having extra spaces in the cos-layout file
					if p == "" {
						continue
					}
					op := mountBind(p, s.Rootdir, s.StateDir)
					err2 := op.run()
					if err2 == nil {
						// Only append to fstabs if there was no error, otherwise we will try to mount it after switch_root
						s.fstabs = append(s.fstabs, &op.FstabEntry)
					}
					// Append to errors only if it's not an already mounted error
					if err2 != nil && !errors.Is(err2, constants.ErrAlreadyMounted) {
						log.Logger.Err(err2).Send()
						err = multierror.Append(err, err2)
					}
				}
				log.Logger.Err(err.ErrorOrNil()).Send()
				return err.ErrorOrNil()
			},
		),
	)
	if err != nil {
		s.Logger.Err(err).Send()
	}

	// overlay mount end
	err = g.Add(opMountOEM,
		overlayCondition,
		mountRootCondition,
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
	if err != nil {
		s.Logger.Err(err).Send()
	}
	err = g.Add(opWriteFstab,
		overlayCondition,
		mountRootCondition,
		herd.WithDeps(opMountOEM, opCustomMounts, opMountBind, opOverlayMount),
		herd.WeakDeps,
		herd.WithCallback(s.WriteFstab(s.path("/etc/fstab"))))
	if err != nil {
		s.Logger.Err(err).Send()
	}
	return err
}
