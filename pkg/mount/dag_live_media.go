package mount

import (
	"context"
	"fmt"
	cnst "github.com/kairos-io/immucore/internal/constants"
	internalUtils "github.com/kairos-io/immucore/internal/utils"
	"github.com/spectrocloud-labs/herd"
	"os"
	"time"
)

// RegisterLiveMedia registers the dag for booting from live media/netboot
// This sets the sentinel.
func (s *State) RegisterLiveMedia(g *herd.Graph) error {
	// Maybe LogIfErrorAndPanic ? If no sentinel, a lot of config files are not going to run
	err := s.LogIfErrorAndReturn(s.WriteSentinelDagStep(g), "write sentinel")

	// Waits for sysroot to be there, just in case
	_ = g.Add(cnst.OpWaitForSysroot,
		herd.WithCallback(func(ctx context.Context) error {
			cc := time.After(60 * time.Second)
			for {
				select {
				default:
					time.Sleep(1 * time.Second)
					_, err = os.Stat("/sysroot")
					if err != nil {
						internalUtils.Log.Err(err).Msg("Checking /sysroot")
						continue
					}
					_, err = os.Stat("/sysroot/system")
					if err != nil {
						internalUtils.Log.Err(err).Msg("Checking /sysroot/system")
						continue
					}
					return nil
				case <-ctx.Done():
					e := fmt.Errorf("context canceled")
					internalUtils.Log.Err(e).Msg("mount canceled")
					return e
				case <-cc:
					e := fmt.Errorf("timeout exhausted")
					internalUtils.Log.Err(e).Msg("Mount timeout")
					return e
				}
			}
		}))

	s.LogIfError(s.RootfsStageDagStep(g, cnst.OpWaitForSysroot), "rootfs stage")
	s.LogIfError(s.InitramfsStageDagStep(g, cnst.OpWaitForSysroot, cnst.OpRootfsHook), "initramfs stage")
	return err
}
