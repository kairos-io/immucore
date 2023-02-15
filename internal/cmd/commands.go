package cmd

import (
	"context"
	"os"

	"github.com/kairos-io/immucore/internal/utils"
	"github.com/kairos-io/immucore/pkg/mount"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spectrocloud-labs/herd"
	"github.com/urfave/cli/v2"
)

var Commands = []*cli.Command{

	{
		Name:      "start",
		Usage:     "start",
		UsageText: "starts",
		Description: `
Sends a generic event payload with the configuration found in the scanned directories.
`,
		Aliases: []string{},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:     "dry-run",
				EnvVars:  []string{"IMMUCORE_DRY_RUN"},
				Required: false,
			},
		},
		Action: func(c *cli.Context) (err error) {
			debug := len(utils.ReadCMDLineArg("rd.immucore.debug")) > 0
			log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
			debugFromEnv := os.Getenv("IMMUCORE_DEBUG") != ""
			if debug || debugFromEnv {
				log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Caller().Logger()
				zerolog.SetGlobalLevel(zerolog.DebugLevel)
			}

			g := herd.DAG(herd.EnableInit)

			// You can pass rd.cos.disable in the cmdline to disable the whole immutable stuff
			cosDisable := len(utils.ReadCMDLineArg("rd.cos.disable")) > 0

			img := utils.ReadCMDLineArg("cos-img/filename=")
			if len(img) == 0 {
				// If we boot from LIVE media or are using dry-run, we use a fake img as we still want to do things
				if c.Bool("dry-run") || cosDisable {
					img = []string{"fake"}
				} else {
					log.Logger.Fatal().Msg("Could not get the image name from cmdline (i.e. cos-img/filename=/cOS/active.img)")
				}
			}
			log.Debug().Strs("TargetImage", img).Msg("Target image")

			s := &mount.State{
				Logger:      log.Logger,
				Rootdir:     utils.GetRootDir(),
				MountRoot:   true,
				TargetLabel: utils.BootStateToLabel(),
				TargetImage: img[0],
			}

			if cosDisable {
				log.Logger.Info().Msg("Stanza rd.cos.disable on the cmdline.")
				err = s.RegisterLiveMedia(g)
			} else {
				err = s.RegisterNormalBoot(g)
			}

			if err != nil {
				s.Logger.Err(err)
				return err
			}

			log.Info().Msg(s.WriteDAG(g))

			if c.Bool("dry-run") {
				return nil
			}

			// First set the sentinel file before running the dag
			if !c.Bool("dry-run") {
				err = utils.SetSentinelFile()
				if err != nil {
					log.Logger.Err(err).Send()
					return err
				}
			}

			err = g.Run(context.Background())
			log.Info().Msg(s.WriteDAG(g))
			return err
		},
	},
}
