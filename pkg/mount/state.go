package mount

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/containerd/containerd/mount"
	"github.com/deniswernert/go-fstab"
	"github.com/kairos-io/immucore/internal/constants"
	internalUtils "github.com/kairos-io/immucore/internal/utils"
	"github.com/rs/zerolog"
	"github.com/spectrocloud-labs/herd"
)

type State struct {
	Rootdir       string // where to mount the root partition e.g. /sysroot inside initrd with pivot, / with nopivot
	TargetImage   string // image from the state partition to mount as loop device e.g. /cOS/active.img
	TargetDevice  string // e.g. /dev/disk/by-label/COS_ACTIVE
	RootMountMode string // How to mount the root partition e.g. ro or rw

	// /run/cos-layout.env (different!)
	OverlayDirs  []string          // e.g. /var
	BindMounts   []string          // e.g. /etc/kubernetes
	CustomMounts map[string]string // e.g. diskid : mountpoint
	OverlayBase  string            // Overlay config, defaults to tmpfs:20%
	StateDir     string            // e.g. "/usr/local/.state"
	fstabs       []*fstab.Mount
}

func (s *State) path(p ...string) string {
	return filepath.Join(append([]string{s.Rootdir}, p...)...)
}

func (s *State) WriteFstab(fstabFile string) func(context.Context) error {
	return func(ctx context.Context) error {
		// Create the file first, override if something is there, we don't care, we are on initramfs
		f, err := os.Create(fstabFile)
		if err != nil {
			return err
		}
		f.Close()
		for _, fst := range s.fstabs {
			internalUtils.Log.Debug().Str("what", fst.String()).Msg("Adding line to fstab")
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

// RunStageOp runs elemental run-stage stage. If its rootfs its special as it needs som symlinks
// If its uki we don't symlink as we already have everything in the sysroot.
func (s *State) RunStageOp(stage string) func(context.Context) error {
	return func(ctx context.Context) error {
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
			err := internalUtils.CreateIfNotExists(constants.LogDir)
			if err != nil {
				return err
			}
			e := os.WriteFile(filepath.Join(constants.LogDir, "rootfs_stage.log"), output.Bytes(), os.ModePerm)
			if e != nil {
				internalUtils.Log.Err(e).Msg("Writing log for rootfs stage")
			}
			return err
		case "initramfs":
			// Not sure if it will work under UKI where the s.Rootdir is the current root already
			internalUtils.Log.Info().Msg("Running initramfs stage")
			chroot := internalUtils.NewChroot(s.Rootdir)
			return chroot.RunCallback(func() error {
				output, _ := internalUtils.RunStage("initramfs")
				internalUtils.Log.Debug().Msg(output.String())
				err := internalUtils.CreateIfNotExists(constants.LogDir)
				if err != nil {
					return err
				}
				e := os.WriteFile(filepath.Join(constants.LogDir, "initramfs_stage.log"), output.Bytes(), os.ModePerm)
				if e != nil {
					internalUtils.Log.Err(e).Msg("Writing log for initramfs stage")
				}
				return err
			})
		default:
			return errors.New("no stage that we know off")
		}
	}
}

// MountOP creates and executes a mount operation.
func (s *State) MountOP(what, where, t string, options []string, timeout time.Duration) func(context.Context) error {

	l := internalUtils.Log.With().Str("what", what).Str("where", where).Str("type", t).Strs("options", options).Logger()
	// Not sure why this defaults to debuglevel when creating a sublogger, so make sure we set it properly
	debug := len(internalUtils.ReadCMDLineArg("rd.immucore.debug")) > 0
	if debug {
		l = l.Level(zerolog.DebugLevel)
	}

	return func(c context.Context) error {
		cc := time.After(timeout)
		for {
			select {
			default:
				// check fs type just-in-time before running the OP
				fsType := internalUtils.DiskFSType(what)
				// If not empty and it does not match
				if fsType != "" && t != fsType {
					t = fsType
				}
				err := internalUtils.CreateIfNotExists(where)
				if err != nil {
					l.Err(err).Msg("Creating dir")
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
					PrepareCallback: func() error {
						_ = internalUtils.Fsck(what)
						return nil
					},
				}

				err = op.run()

				// If no error on mounting or error is already mounted, as that affects the sysroot
				// for some reason it reports that its already mounted (systemd is mounting it behind our back!).
				if err == nil || err != nil && errors.Is(err, constants.ErrAlreadyMounted) {
					s.AddToFstab(tmpFstab)
				} else {
					l.Debug().Err(err).Msg("Mount not added to fstab")
				}

				// only continue the loop if it's an error and not an already mounted error
				if err != nil && !errors.Is(err, constants.ErrAlreadyMounted) {
					l.Err(err).Send()
					continue
				}
				l.Info().Msg("mount done")
				return nil
			case <-c.Done():
				e := fmt.Errorf("context canceled")
				l.Err(e).Msg("mount canceled")
				return e
			case <-cc:
				e := fmt.Errorf("timeout exhausted")
				l.Err(e).Msg("Mount timeout")
				return e
			}
		}
	}
}

// WriteDAG writes the dag.
func (s *State) WriteDAG(g *herd.Graph) (out string) {
	for i, layer := range g.Analyze() {
		out += fmt.Sprintf("%d.\n", i+1)
		for _, op := range layer {
			if op.Error != nil {
				out += fmt.Sprintf(" <%s> (error: %s) (background: %t) (weak: %t) (run: %t)\n", op.Name, op.Error.Error(), op.Background, op.WeakDeps, op.Executed)
			} else {
				out += fmt.Sprintf(" <%s> (background: %t) (weak: %t) (run: %t)\n", op.Name, op.Background, op.WeakDeps, op.Executed)
			}
		}
	}
	return
}

// LogIfError will log if there is an error with the given context as message
// Context can be empty.
func (s *State) LogIfError(e error, msgContext string) {
	if e != nil {
		internalUtils.Log.Err(e).Msg(msgContext)
	}
}

// LogIfErrorAndReturn will log if there is an error with the given context as message
// Context can be empty
// Will also return the error.
func (s *State) LogIfErrorAndReturn(e error, msgContext string) error {
	if e != nil {
		internalUtils.Log.Err(e).Msg(msgContext)
	}
	return e
}

// LogIfErrorAndPanic will log if there is an error with the given context as message
// Context can be empty
// Will also panic.
func (s *State) LogIfErrorAndPanic(e error, msgContext string) {
	if e != nil {
		internalUtils.Log.Err(e).Msg(msgContext)
		internalUtils.Log.Fatal().Msg(e.Error())
	}
}

// AddToFstab will try to add an entry to the fstab list
// Will check if the entry exists before adding it to avoid duplicates.
func (s *State) AddToFstab(tmpFstab *fstab.Mount) {
	found := false
	for _, f := range s.fstabs {
		if f.Spec == tmpFstab.Spec {
			internalUtils.Log.Debug().Interface("existing", f).Interface("duplicated", tmpFstab).Msg("Duplicated fstab entry found, not adding")
			found = true
		}
	}
	if !found {
		s.fstabs = append(s.fstabs, tmpFstab)
	}
}
