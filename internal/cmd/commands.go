package cmd

import (
	"context"
	"os"

	"github.com/kairos-io/immucore/internal/utils"
	"github.com/kairos-io/immucore/internal/version"
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

			v := version.Get()
			log.Logger.Info().Str("commit", v.GitCommit).Str("compiled with", v.GoVersion).Str("version", v.Version).Msg("Immucore")

			cmdline, _ := os.ReadFile("/proc/cmdline")
			log.Logger.Debug().Msg(string(cmdline))
			g := herd.DAG(herd.EnableInit)

			// Get targets and state
			targetLabel, targetDevice := utils.GetTarget(c.Bool("dry-run"))

			s := &mount.State{
				Logger:        log.Logger,
				Rootdir:       utils.GetRootDir(),
				TargetLabel:   targetDevice,
				TargetImage:   targetLabel,
				RootMountMode: utils.RootRW(),
			}

			if utils.DisableImmucore() {
				log.Logger.Info().Msg("Stanza rd.cos.disable on the cmdline or booting from CDROM/Netboot/recovery. Disabling immucore.")
				err = s.RegisterLiveMedia(g)
			} else {
				log.Logger.Info().Msg("Booting on active/passive.")
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
		},
	},
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
