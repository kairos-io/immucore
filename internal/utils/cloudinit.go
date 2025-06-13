package utils

import (
	"errors"
	"fmt"
	"os"

	"github.com/hashicorp/go-multierror"
	"github.com/kairos-io/immucore/internal/constants"
	"github.com/mudler/yip/pkg/console"
	"github.com/mudler/yip/pkg/executor"
	"github.com/mudler/yip/pkg/schema"
	"github.com/twpayne/go-vfs/v4"
	"gopkg.in/yaml.v3"
)

func RunStage(stage string) error {
	var allErrors, err error
	var cmdLineYipURI string

	// Set debug logger
	yip := executor.NewExecutor(executor.WithLogger(KLog))
	c := ImmucoreConsole{}

	stageBefore := fmt.Sprintf("%s.before", stage)
	stageAfter := fmt.Sprintf("%s.after", stage)
	// Deprecated? Is nowhere on the docs....
	cosSetup := ReadCMDLineArg("cos.setup")
	if len(cosSetup) > 1 {
		cmdLineYipURI = cosSetup[1]
	}

	// Run all stages for each of the default cloud config paths + extra cloud config paths
	for _, s := range []string{stageBefore, stage, stageAfter} {
		err = yip.Run(s, vfs.OSFS, c, constants.GetCloudInitPaths()...)
		if err != nil {
			allErrors = multierror.Append(allErrors, err)
		}
	}

	// Run the stages if cmdline contains the cos.setup stanza
	if cmdLineYipURI != "" {
		cmdLineArgs := []string{cmdLineYipURI}
		for _, s := range []string{stageBefore, stage, stageAfter} {
			err = yip.Run(s, vfs.OSFS, c, cmdLineArgs...)
			if err != nil {
				allErrors = multierror.Append(allErrors, err)
			}
		}
	}

	// Enable dot notation
	// This helps to parse the cmdline in dot notation (stage.name.command) from cmdline
	// IMHO this should be deprecated, plenty of other places to set the stages config
	yip.Modifier(schema.DotNotationModifier)

	// Read and parse the cmdline looking for yip config in there
	cmdLineOut, err := os.ReadFile("/proc/cmdline")
	if err == nil {
		for _, s := range []string{stageBefore, stage, stageAfter} {
			err = yip.Run(s, vfs.OSFS, console.NewStandardConsole(), string(cmdLineOut))
			if err != nil {
				allErrors = checkYAMLError(allErrors, err)
			}
		}
	}

	// Set back the modifier to nil
	yip.Modifier(nil)

	// Not doing anything with the errors yet, need to know which ones are permissible (no metadata, marshall errors, etc..)
	return nil
}

func onlyYAMLPartialErrors(er error) bool {
	var merr *multierror.Error
	if errors.As(er, &merr) {
		for _, e := range merr.Errors {
			// Skip partial unmarshalling errors
			// TypeError is throwed when it is possible to read the yaml partially
			// XXX: Seems errors.Is and errors.As are not working as expected here.
			// Even if the underlying type is yaml.TypeError.
			var d *yaml.TypeError
			if fmt.Sprintf("%T", e) != fmt.Sprintf("%T", d) {
				return false
			}
		}
	}
	return true
}

func checkYAMLError(allErrors, err error) error {
	if !onlyYAMLPartialErrors(err) {
		// here we absorb errors only if are related to YAML unmarshalling
		// As cmdline is parsed out as a yaml file
		allErrors = multierror.Append(allErrors, err)
	}
	return allErrors
}
