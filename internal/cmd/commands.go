package cmd

import (
	"context"

	"github.com/kairos-io/immucore/pkg/mount"
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

			g := herd.DAG()

			s := &mount.State{Rootdir: "/"}

			s.Register(g)

			return g.Run(context.Background())
		},
	},
}
