package main

import (
	"fmt"
	"os"

	"github.com/kairos-io/immucore/internal/cmd"

	"github.com/urfave/cli"
)

// Apply Immutability profiles.
func main() {
	app := &cli.App{
		Name:    "immucore",
		Version: "0.1",
		Author:  "Kairos authors",
		Usage:   "kairos agent start",
		Description: `
`,
		UsageText: ``,
		Copyright: "kairos authors",

		Commands: cmd.Commands,
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
