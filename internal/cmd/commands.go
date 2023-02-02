package cmd

import (
	"os"

	"github.com/kairos-io/immucore/pkg/mount"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spectrocloud-labs/herd"
	"github.com/urfave/cli"
)

var Commands = []cli.Command{

	{
		Name:      "start",
		Usage:     "start",
		UsageText: "starts",
		Description: `
Sends a generic event payload with the configuration found in the scanned directories.
`,
		Aliases: []string{},
		Flags:   []cli.Flag{},
		Action: func(c *cli.Context) error {
			log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
			g := herd.DAG()

			s := &mount.State{Rootdir: "/"}

			s.Register(g)
			writeDag(g.Analyze())
			return nil
			//return g.Run(context.Background())
		},
	},
}

func writeDag(d [][]herd.GraphEntry) {
	for i, layer := range d {
		log.Printf("%d.", (i + 1))
		for _, op := range layer {
			if op.Error != nil {
				log.Printf(" <%s> (error: %s) (background: %t)", op.Name, op.Error.Error(), op.Background)
			} else {
				log.Printf(" <%s> (background: %t)", op.Name, op.Background)
			}
		}
		log.Print("")
	}
}
