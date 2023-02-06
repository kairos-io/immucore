package cmd

import (
	"os"

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
			log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Caller().Logger()
			g := herd.DAG()

			s := &mount.State{Logger: log.Logger, Rootdir: "/"}

			err = s.Register(g)
			if err != nil {
				s.Logger.Err(err)
				return err
			}

			log.Print(s.WriteDAG(g))

			if c.Bool("dry-run") {
				return err
			}

			//log.Print("Calling dag")
			//return g.Run(context.Background())
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
