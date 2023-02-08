package mount

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/containerd/mount"
	"github.com/deniswernert/go-fstab"
	"github.com/hashicorp/go-multierror"
	"github.com/joho/godotenv"
	internalUtils "github.com/kairos-io/immucore/internal/utils"
	"github.com/kairos-io/kairos/pkg/utils"
	"github.com/kairos-io/kairos/sdk/state"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sanity-io/litter"
	"github.com/spectrocloud-labs/herd"
)

type State struct {
	Logger      zerolog.Logger
	Rootdir     string // e.g. /sysroot inside initrd with pivot, / with nopivot
	TargetImage string // e.g. /cOS/active.img
	TargetLabel string // e.g. COS_ACTIVE

	// /run/cos-layout.env (different!)
	OverlayDir   []string          // e.g. /var
	BindMounts   []string          // e.g. /etc/kubernetes
	CustomMounts map[string]string // e.g. diskid : mountpoint

	StateDir  string // e.g. "/usr/local/.state"
	MountRoot bool   // e.g. if true, it tries to find the image to loopback mount

	fstabs     []*fstab.Mount
	IsRecovery bool // if its recovery it needs different stuff
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
				log.Logger.Debug().Str("fstabline", litter.Sdump(fst)).Str("fstabfile", fstabFile).Msg("Writing fstab line")
				f, err := os.OpenFile(fstabFile,
					os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					return err
				}
				defer f.Close()
				// As we mount on /sysroot during initramfs but the fstab file is for the real init process, we need to remove
				// Any mentions to /sysroot from the fstab lines, otherwise they wont work
				fstCleaned := strings.ReplaceAll(fst.String(), "/sysroot", "")
				toWrite := fmt.Sprintf("%s\n", fstCleaned)
				if _, err := f.WriteString(toWrite); err != nil {
					return err
				}
				log.Logger.Debug().Str("fstabline", fstCleaned).Str("fstabfile", fstabFile).Msg("Done fstab line")
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
				s.Logger.Debug().Str("from", "/sysroot/system").Str("to", "/system").Msg("Creating symlink")
				err = os.Symlink("/sysroot/system", "/system")
				if err != nil {
					s.Logger.Err(err).Msg("creating symlink")
					return err
				}
			}
		}

		cmd := fmt.Sprintf("elemental run-stage %s", stage)
		log.Logger.Debug().Str("cmd", cmd).Msg("")
		output, err := utils.SH(cmd)
		log.Info().Msg(output)
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
				if _, err := os.Stat(where); os.IsNotExist(err) {
					log.Logger.Debug().Str("what", what).Str("where", where).Str("type", t).Strs("options", options).Msg("Mount point does not exist, creating")
					err = os.MkdirAll(where, os.ModeDir|os.ModePerm)
					if err != nil {
						log.Logger.Debug().Str("what", what).Str("where", where).Str("type", t).Strs("options", options).Err(err).Msg("Creating dir")
						continue
					}
				}
				time.Sleep(1 * time.Second)
				mountPoint := mount.Mount{
					Type:    t,
					Source:  what,
					Options: options,
				}
				tmpFstab := mountToStab(mountPoint)
				tmpFstab.File = where
				op := mountOperation{
					MountOption: mountPoint,
					FstabEntry:  *tmpFstab,
					Target:      where,
				}

				err := op.run()
				if err != nil {
					log.Logger.Debug().Str("what", what).Str("where", where).Str("type", t).Strs("options", options).Err(err).Msg("mounting")
					continue
				}

				s.fstabs = append(s.fstabs, tmpFstab)
				log.Logger.Debug().Str("what", what).Str("where", where).Str("type", t).Strs("options", options).Msg("Mounted")
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
		out += fmt.Sprintf("%d.\n", (i + 1))
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

func readEnv(file string) (map[string]string, error) {
	var envMap map[string]string
	var err error

	f, err := os.Open(file)
	if err != nil {
		return envMap, err
	}
	defer f.Close()

	envMap, err = godotenv.Parse(f)
	if err != nil {
		return envMap, err
	}

	return envMap, err
}

func (s *State) Register(g *herd.Graph) error {
	var err error

	runtime, err := state.NewRuntime()
	if err != nil {
		s.Logger.Debug().Err(err).Msg("")
		return err
	}
	s.Logger.Debug().Str("state", litter.Sdump(runtime)).Msg("Register")

	// TODO: add hooks, fstab (might have missed some), systemd compat
	// TODO: We should also set tmpfs here (not -related)

	// All of this below need to run after rootfs stage runs (so the layout file is created)
	// This is legacy - in UKI we don't need to found the img, this needs to run in a conditional
	if s.MountRoot {
		// setup loopback mount for the image target for booting
		s.Logger.Debug().Str("what", opDiscoverState).Msg("Add operation")
		err = g.Add(opDiscoverState,
			herd.WithDeps(opMountState),
			herd.WithCallback(
				func(ctx context.Context) error {
					log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
					cmd := fmt.Sprintf("losetup --show -f %s", s.path("/run/initramfs/cos-state", s.TargetImage))
					log.Logger.Debug().Str("targetImage", s.TargetImage).Str("path", s.Rootdir).Str("fullcmd", cmd).Msg("Mounting image")
					_, err := utils.SH(cmd)
					if err != nil {
						log.Logger.Debug().Err(err).Msg("")
					}
					return err
				},
			))
		if err != nil {
			s.Logger.Err(err)
		}

		// mount the state partition so to find the loopback device
		// Itxaka: what if its recovery?
		s.Logger.Debug().Str("what", opMountState).Msg("Add operation")

		stateName := runtime.State.Name
		stateFs := runtime.State.Type
		// Recovery is a different partition
		if s.IsRecovery {
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
			s.Logger.Err(err)
		}

		// mount the loopback device as root of the fs
		s.Logger.Debug().Str("what", opMountRoot).Msg("Add operation")
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
			s.Logger.Err(err)
		}

	}

	// depending on /run/cos-layout.env
	// This is building the mountRoot dependendency if it was enabled
	mountRootCondition := herd.ConditionalOption(func() bool { return s.MountRoot }, herd.WithDeps(opMountRoot))
	s.Logger.Debug().Bool("mountRootCondition", s.MountRoot).Msg("condition")

	// TODO: this needs to be run after sysroot so we can link to /sysroot/system/oem and after /oem mounted
	s.Logger.Debug().Str("what", opRootfsHook).Msg("Add operation")
	err = g.Add(opRootfsHook, mountRootCondition, herd.WithDeps(opMountRoot, opMountOEM), herd.WithCallback(s.RunStageOp("rootfs")))
	if err != nil {
		s.Logger.Err(err).Msg("running rootfs stage")
	}

	// /run/cos/cos-layout.env
	// populate state bindmounts, overlaymounts, custommounts
	s.Logger.Debug().Str("what", opLoadConfig).Msg("Add operation")
	err = g.Add(opLoadConfig,
		herd.WithDeps(opRootfsHook),
		herd.WithCallback(func(ctx context.Context) error {
			log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
			if s.IsRecovery {
				log.Info().Msg("We are on recovery, not reading cos-layout.env")
				return nil
			}
			if s.CustomMounts == nil {
				s.CustomMounts = map[string]string{}
			}

			env, err := readEnv("/run/cos/cos-layout.env")
			if err != nil {
				log.Logger.Err(err).Msg("Reading env")
				return err
			}
			log.Logger.Debug().Str("envfile", litter.Sdump(env)).Msg("loading cos layout")
			// populate from env here
			s.OverlayDir = strings.Split(env["RW_PATHS"], " ")

			// TODO: PERSISTENT_STATE_TARGET /usr/local/.state
			s.BindMounts = strings.Split(env["PERSISTENT_STATE_PATHS"], " ")
			log.Logger.Debug().Strs("paths", s.BindMounts).Msg("persistent paths")
			log.Logger.Debug().Str("pathsraw", env["PERSISTENT_STATE_PATHS"]).Msg("persistent paths")

			s.StateDir = env["PERSISTENT_STATE_TARGET"]
			if s.StateDir == "" {
				s.StateDir = "/usr/local/.state"
			}

			// s.CustomMounts is special:
			// It gets parsed by the cmdline (TODO)
			// and from the env var
			// https://github.com/kairos-io/packages/blob/7c3581a8ba6371e5ce10c3a98bae54fde6a505af/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-generator.sh#L71
			// https://github.com/kairos-io/packages/blob/7c3581a8ba6371e5ce10c3a98bae54fde6a505af/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-mount-layout.sh#L80

			addLine := func(d string) {
				dat := strings.Split(d, ":")
				if len(dat) == 2 {
					disk := dat[0]
					path := dat[1]
					s.CustomMounts[disk] = path
				}
			}
			for _, v := range append(internalUtils.ReadCMDLineArg("rd.cos.mount="), strings.Split(env["VOLUMES"], " ")...) {
				addLine(internalUtils.ParseMount(v))
			}

			return nil
		}))
	if err != nil {
		s.Logger.Err(err)
	}
	// end sysroot mount

	// overlay mount start
	if rootFSType(s.Rootdir) != "overlay" {
		s.Logger.Debug().Str("what", opMountBaseOverlay).Msg("Add operation")
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
					return op.run()
				},
			),
		)
		if err != nil {
			s.Logger.Err(err)
		}
	}

	overlayCondition := herd.ConditionalOption(func() bool { return rootFSType(s.Rootdir) != "overlay" }, herd.WithDeps(opMountBaseOverlay))
	s.Logger.Debug().Bool("overlaycondition", rootFSType(s.Rootdir) != "overlay").Msg("condition")
	// TODO: Add fsck
	// mount overlay
	s.Logger.Debug().Str("what", opOverlayMount).Msg("Add operation")
	err = g.Add(
		opOverlayMount,
		overlayCondition,
		herd.WithDeps(opLoadConfig),
		mountRootCondition,
		herd.WithCallback(
			func(ctx context.Context) error {
				var err error
				for _, p := range s.OverlayDir {
					op, err := mountWithBaseOverlay(p, s.Rootdir, "/run/overlay")
					if err != nil {
						return err
					}
					s.fstabs = append(s.fstabs, &op.FstabEntry)
					err = multierror.Append(err, op.run())
				}

				return err
			},
		),
	)
	if err != nil {
		s.Logger.Err(err)
	}
	s.Logger.Debug().Str("what", opCustomMounts).Msg("Add operation")
	err = g.Add(
		opCustomMounts,
		mountRootCondition,
		overlayCondition,
		herd.WithDeps(opLoadConfig),
		herd.WithCallback(func(ctx context.Context) error {
			s.Logger.Debug().Msg("Start" + opCustomMounts)
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
				s.Logger.Debug().Str("what", what).Str("where", s.path(where)).Str("type", fstype).Msg("mounting custom mounts")
				err = multierror.Append(err, s.MountOP(
					what,
					s.path(where),
					fstype,
					mountOptions,
					10*time.Second,
				)(ctx))

			}
			s.Logger.Debug().Msg("End" + opCustomMounts)
			s.Logger.Err(err.ErrorOrNil()).Send()

			return err.ErrorOrNil()
		}),
	)
	if err != nil {
		s.Logger.Err(err)
	}

	// mount state is defined over a custom mount (/usr/local/.state for instance, needs to be mounted over a device)
	s.Logger.Debug().Str("what", opMountBind).Msg("Add operation")
	err = g.Add(
		opMountBind,
		overlayCondition,
		mountRootCondition,
		herd.WithDeps(opCustomMounts, opLoadConfig),
		herd.WithCallback(
			func(ctx context.Context) error {
				var err error
				log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
				log.Logger.Debug().Strs("binds", s.BindMounts).Msg("Mounting bind")
				for _, p := range s.BindMounts {
					// TODO: Check why p can be empty, Example:
					/*
						3:12PM DBG Mounting bind binds=[
						"/etc/systemd","/etc/modprobe.d",
						"/etc/rancher","/etc/sysconfig",
						"/etc/runlevels","/etc/ssh",
						"/etc/ssl/certs","/etc/iscsi",
						"",  <----- HERE
						"/etc/cni","/etc/kubernetes",
						"/home","/opt","/root","/snap",
						"/var/snap","/usr/libexec",
						"/var/log","/var/lib/rancher",
						"/var/lib/kubelet","/var/lib/snapd"
						,"/var/lib/wicked","/var/lib/longhorn"
						,"/var/lib/cni","/usr/share/pki/trust"
						,"/usr/share/pki/trust/anchors",
						"/var/lib/ca-certificates"]
					*/
					if p == "" {
						continue
					}
					log.Logger.Debug().Str("what", p).Str("where", s.StateDir).Msg("Mounting bind")
					op := mountBind(p, s.Rootdir, s.StateDir)
					err2 := op.run()
					if err2 != nil {
						log.Logger.Err(err2).Send()
						err = multierror.Append(err, err2)
					} else {
						// Only append to fstabs if there was no error, otherwise we will try to mount it after switch_root
						s.fstabs = append(s.fstabs, &op.FstabEntry)
					}
				}
				log.Logger.Err(err).Send()
				return err
			},
		),
	)
	if err != nil {
		s.Logger.Err(err)
	}

	// overlay mount end
	s.Logger.Debug().Str("what", opMountOEM).Msg("Add operation")
	err = g.Add(opMountOEM,
		overlayCondition,
		mountRootCondition,
		herd.WithCallback(
			s.MountOP(
				runtime.OEM.Name,
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
		s.Logger.Err(err)
	}
	s.Logger.Debug().Str("what", opWriteFstab).Msg("Add operation")
	err = g.Add(opWriteFstab,
		overlayCondition,
		mountRootCondition,
		herd.WithDeps(opMountOEM, opCustomMounts, opMountBind, opOverlayMount),
		herd.WeakDeps,
		herd.WithCallback(s.WriteFstab(s.path("/etc/fstab"))))
	if err != nil {
		s.Logger.Err(err)
	}
	return err
}
