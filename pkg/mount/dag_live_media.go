package mount

import (
	"context"
	"fmt"
	internalUtils "github.com/kairos-io/immucore/internal/utils"
	"github.com/spectrocloud-labs/herd"
	"os"
	"time"
)

// RegisterLiveMedia registers the dag for booting from live media/netboot
// This sets the sentinel.
func (s *State) RegisterLiveMedia(g *herd.Graph) error {
	// Maybe LogIfErrorAndPanic ? If no sentinel, a lot of config files are not going to run
	// rootfs missing
	err := s.LogIfErrorAndReturn(s.WriteSentinelDagStep(g), "write sentinel")

	_ = g.Add("wait-for-rootfsbase", herd.WithCallback(func(ctx context.Context) error {
		cc := time.After(60 * time.Second)
		for {
			select {
			default:
				time.Sleep(1 * time.Second)
				_, err = os.Stat("/run/rootfsbase")
				if err != nil {
					internalUtils.Log.Err(err).Msg("Checking rootfsbase")
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

	_ = g.Add("test", herd.WithDeps("wait-for-rootfsbase"), herd.WithCallback(func(ctx context.Context) error {
		internalUtils.Log.Debug().Msg("CALLED!")
		return nil
	}))
	// wait for /run/rootfsbase

	return err
}
