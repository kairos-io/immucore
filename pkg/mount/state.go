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
	"github.com/kairos-io/kairos/pkg/utils"
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

	OverlayBase string // Overlay config, defaults to tmpfs:20%
	OemTimout   int    // Time to wait for the oem to time out if not found, defaults to 5s

	StateDir string // e.g. "/usr/local/.state"
	fstabs   []*fstab.Mount
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
		cmd := fmt.Sprintf("/usr/bin/elemental run-stage %s", stage)
		// If we set the level to debug, also call elemental with debug
		if internalUtils.Log.GetLevel() == zerolog.DebugLevel {
			cmd = fmt.Sprintf("%s --debug", cmd)
		}

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
			output, err := utils.SH(cmd)
			internalUtils.Log.Debug().Msg(output)
			return err
		case "initramfs":
			chroot := internalUtils.NewChroot(s.Rootdir)
			defer func(chroot *internalUtils.Chroot) {
				err := chroot.Close()
				if err != nil {
					internalUtils.Log.Err(err).Msg("closing chroot")
				}
			}(chroot)
			output, err := chroot.Run(cmd)
			internalUtils.Log.Debug().Msg(output)
			return err
		default:
			return errors.New("no stage that we know off")
		}
	}
}

// MountOP creates and executes a mount operation.
func (s *State) MountOP(what, where, t string, options []string, timeout time.Duration) func(context.Context) error {
	internalUtils.Log.With().Str("what", what).Str("where", where).Str("type", t).Strs("options", options).Logger()

	return func(c context.Context) error {
		cc := time.After(timeout)
		for {
			select {
			default:
				err := internalUtils.CreateIfNotExists(where)
				if err != nil {
					internalUtils.Log.Err(err).Msg("Creating dir")
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

				if err == nil {
					s.fstabs = append(s.fstabs, tmpFstab)
				}

				// only continue the loop if it's an error and not an already mounted error
				if err != nil && !errors.Is(err, constants.ErrAlreadyMounted) {
					internalUtils.Log.Err(err).Send()
					continue
				}
				internalUtils.Log.Debug().Msg("mount done")
				return nil
			case <-c.Done():
				e := fmt.Errorf("context canceled")
				internalUtils.Log.Err(e).Msg("mount canceled")
				return e
			case <-cc:
				e := fmt.Errorf("timeout exhausted")
				internalUtils.Log.Err(e).Msg("Mount timeout")
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
				out += fmt.Sprintf(" <%s> (error: %s) (background: %t) (weak: %t)\n", op.Name, op.Error.Error(), op.Background, op.WeakDeps)
			} else {
				out += fmt.Sprintf(" <%s> (background: %t) (weak: %t)\n", op.Name, op.Background, op.WeakDeps)
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
