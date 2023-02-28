package main

import (
	"context"
	"fmt"
	"github.com/kairos-io/immucore/internal/utils"
	"github.com/kairos-io/immucore/internal/version"
	"github.com/kairos-io/immucore/pkg/mount"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spectrocloud-labs/herd"
	"github.com/urfave/cli/v2"
	"os"
)

// Apply Immutability profiles.
func main() {
	app := cli.NewApp()
	app.Version = version.GetVersion()
	app.Authors = []*cli.Author{{Name: "Kairos authors"}}
	app.Copyright = "kairos authors"
	app.Action = func(c *cli.Context) (err error) {
		debug := len(utils.ReadCMDLineArg("rd.immucore.debug")) > 0
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		debugFromEnv := os.Getenv("IMMUCORE_DEBUG") != ""
		if debug || debugFromEnv {
			log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Caller().Logger()
			zerolog.SetGlobalLevel(zerolog.DebugLevel)
		}

		v := version.Get()
		log.Logger.Info().Str("commit", v.GitCommit).Str("compiled with", v.GoVersion).Str("version", v.Version).Msg("Immucore")

		cmdline, _ := os.ReadFile("/proc/cmdline")
		log.Logger.Debug().Msg(string(cmdline))
		g := herd.DAG(herd.EnableInit)

		// Get targets and state
		targetImage, targetDevice := utils.GetTarget(c.Bool("dry-run"))

		s := &mount.State{
			Logger:        log.Logger,
			Rootdir:       utils.GetRootDir(),
			TargetDevice:  targetDevice,
			TargetImage:   targetImage,
			RootMountMode: utils.RootRW(),
		}

		if utils.DisableImmucore() {
			log.Logger.Info().Msg("Stanza rd.cos.disable on the cmdline or booting from CDROM/Netboot/Squash recovery. Disabling immucore.")
			err = s.RegisterLiveMedia(g)
		} else {
			log.Logger.Info().Msg("Booting on active/passive/recovery.")
			err = s.RegisterNormalBoot(g)
		}

		if err != nil {
			s.Logger.Err(err)
			return err
		}

		log.Info().Msg(s.WriteDAG(g))

		// Once we print the dag we can exit already
		if c.Bool("dry-run") {
			return nil
		}

		err = g.Run(context.Background())
		log.Info().Msg(s.WriteDAG(g))
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
				log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
				zerolog.SetGlobalLevel(zerolog.InfoLevel)
				v := version.Get()
				log.Logger.Info().Str("commit", v.GitCommit).Str("compiled with", v.GoVersion).Str("version", v.Version).Msg("Immucore")
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
