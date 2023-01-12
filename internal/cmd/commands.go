package cmd

import (
	"github.com/kairos-io/immucore/pkg/mount"
	"github.com/urfave/cli"
)

var Commands = []cli.Command{

	{
		Name:      "load",
		Usage:     "notify <event> <config dir>...",
		UsageText: "emits the given event with a generic event payload",
		Description: `
Sends a generic event payload with the configuration found in the scanned directories.
`,
		Aliases: []string{},
		Flags:   []cli.Flag{},
		Action: func(c *cli.Context) error {

			mount.MountOverlayFS()
			return nil
		},
	},
}
