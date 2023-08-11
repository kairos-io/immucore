package main

import (
	"context"
	"fmt"
	"os"

	"github.com/kairos-io/immucore/internal/utils"
	"github.com/kairos-io/immucore/internal/version"
	"github.com/kairos-io/immucore/pkg/mount"
	"github.com/spectrocloud-labs/herd"
	"github.com/urfave/cli/v2"
)

// Apply Immutability profiles.
func main() {
	app := cli.NewApp()
	app.Version = version.GetVersion()
	app.Authors = []*cli.Author{{Name: "Kairos authors"}}
	app.Copyright = "kairos authors"
	app.Action = func(c *cli.Context) (err error) {
		var targetDevice, targetImage string
		var state *mount.State

		utils.MountProc()
		err = utils.SetLogger()
		if err != nil {
			return err
		}

		v := version.Get()
		utils.Log.Logger.Info().Str("commit", v.GitCommit).Str("compiled with", v.GoVersion).Str("version", v.Version).Msg("Immucore")

		cmdline, _ := os.ReadFile(utils.GetHostProcCmdline())
		utils.Log.Logger.Debug().Str("content", string(cmdline)).Msg("cmdline")
		g := herd.DAG(herd.EnableInit)

		// Get targets and state
		targetImage, targetDevice, err = utils.GetTarget(c.Bool("dry-run"))
		if err != nil {
			return err
		}

		state = &mount.State{
			Rootdir:       utils.GetRootDir(),
			TargetDevice:  targetDevice,
			TargetImage:   targetImage,
			RootMountMode: utils.RootRW(),
			OverlayBase:   utils.GetOverlayBase(),
		}

		if utils.DisableImmucore() {
			utils.Log.Logger.Info().Msg("Stanza rd.cos.disable/rd.immucore.disable on the cmdline or booting from CDROM/Netboot/Squash recovery. Disabling immucore.")
			err = state.RegisterLiveMedia(g)
		} else if utils.IsUKI() {
			utils.Log.Logger.Info().Msg("UKI booting!")
			err = state.RegisterUKI(g)
		} else {
			utils.Log.Logger.Info().Msg("Booting on active/passive/recovery.")
			err = state.RegisterNormalBoot(g)
		}

		if err != nil {
			return err
		}

		utils.Log.Logger.Info().Msg(state.WriteDAG(g))

		// Once we print the dag we can exit already
		if c.Bool("dry-run") {
			return nil
		}

		err = g.Run(context.Background())
		utils.Log.Logger.Info().Msg(state.WriteDAG(g))
		return err
	}
	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name: "dry-run",
		},
	}
	app.Commands = []*cli.Command{
		{
			Name:  "version",
			Usage: "version",
			Action: func(c *cli.Context) error {
				err := utils.SetLogger()
				if err != nil {
					return err
				}
				v := version.Get()
				utils.Log.Logger.Info().Str("commit", v.GitCommit).Str("compiled with", v.GoVersion).Str("version", v.Version).Msg("Immucore")
				return nil
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
