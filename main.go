package main

import (
	"context"
	"fmt"
	"os"

	"github.com/kairos-io/immucore/internal/utils"
	"github.com/kairos-io/immucore/internal/version"
	"github.com/kairos-io/immucore/pkg/dag"
	"github.com/kairos-io/immucore/pkg/state"
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
		var st *state.State

		utils.MountBasic()
		utils.SetLogger()

		v := version.Get()
		utils.Log.Info().Str("commit", v.GitCommit).Str("compiled with", v.GoVersion).Str("version", v.Version).Msg("Immucore")

		cmdline, _ := os.ReadFile(utils.GetHostProcCmdline())
		utils.Log.Debug().Str("content", string(cmdline)).Msg("cmdline")
		g := herd.DAG(herd.EnableInit)

		// Get targets and state
		targetImage, targetDevice, err = utils.GetTarget(c.Bool("dry-run"))
		if err != nil {
			return err
		}

		st = &state.State{
			Rootdir:       utils.GetRootDir(),
			TargetDevice:  targetDevice,
			TargetImage:   targetImage,
			RootMountMode: utils.RootRW(),
			OverlayBase:   utils.GetOverlayBase(),
		}

		if utils.DisableImmucore() {
			utils.Log.Info().Msg("Stanza rd.cos.disable/rd.immucore.disable on the cmdline or booting from CDROM/Netboot/Squash recovery. Disabling immucore.")
			err = dag.RegisterLiveMedia(st, g)
		} else if utils.IsUKI() {
			utils.Log.Info().Msg("UKI booting!")
			err = dag.RegisterUKI(st, g)
		} else {
			utils.Log.Info().Msg("Booting on active/passive/recovery.")
			err = dag.RegisterNormalBoot(st, g)
		}

		if err != nil {
			return err
		}

		utils.Log.Info().Msg(st.WriteDAG(g))

		// Once we print the dag we can exit already
		if c.Bool("dry-run") {
			return nil
		}

		err = g.Run(context.Background())
		utils.Log.Info().Msg(st.WriteDAG(g))
		x, _ := utils.CommandWithPath("stat /sysroot")
		utils.Log.Info().Str("path", x).Msg("Sysroot status after dag")
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
			Action: func(_ *cli.Context) error {
				utils.SetLogger()
				v := version.Get()
				utils.Log.Info().Str("commit", v.GitCommit).Str("compiled with", v.GoVersion).Str("version", v.Version).Msg("Immucore")
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
