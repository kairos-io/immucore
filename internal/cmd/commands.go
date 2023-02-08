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
			log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()

			// If we boot from CD, we do nothing
			cdBoot, err := utils.BootedFromCD()
			if err != nil {
				log.Logger.Err(err).Send()
				return err
			}

			if cdBoot {
				log.Info().Msg("Seems we booted from CD, doing nothing. Bye!")
				return nil
			}

			img := utils.ReadCMDLineArg("cos-img/filename=")
			log.Debug().Strs("TargetImage", img).Msg("Target image")
			g := herd.DAG()
			s := &mount.State{
				Logger:      log.Logger,
				Rootdir:     utils.GetRootDir(),
				MountRoot:   true,
				TargetLabel: utils.BootStateToLabel(),
				TargetImage: img[0],
				IsRecovery:  utils.IsRecovery(),
			}

			err = s.Register(g)
			if err != nil {
				s.Logger.Err(err)
				return err
			}

			log.Print(s.WriteDAG(g))

			if c.Bool("dry-run") {
				return err
			}

			log.Print("Calling dag")
			err = g.Run(context.Background())
			log.Print(s.WriteDAG(g))
			return err
		},
	},
}

func writeDag(d [][]herd.GraphEntry) {
	for i, layer := range d {
		log.Printf("%d.", (i + 1))
		for _, op := range layer {
			if op.Error != nil {
				log.Printf(" <%s> (error: %s) (background: %t) (weak: %t)", op.Name, op.Error.Error(), op.Background, op.WeakDeps)
			} else {
				log.Printf(" <%s> (background: %t) (weak: %t)", op.Name, op.Background, op.WeakDeps)
			}
		}
		log.Print("")
	}
}
