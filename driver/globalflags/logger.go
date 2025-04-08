package globalflags

import (
	"fmt"
	"io"
	"os"

	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli/v2"
)

var (
	verbosityFlag = cli.IntFlag{
		Name:  "verbosity",
		Usage: "Changes logging verbosity: 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=detail",
		Value: 3,
	}

	vmoduleFlag = cli.StringFlag{
		Name: "vmodule",
		Usage: `Changes per-module verbosity:

                  The syntax of the argument is a comma-separated list of pattern=N, where the
                  pattern is a literal file name or "glob" pattern matching and N is a V level.

                  For instance:
                  - pattern="gopher.go=3"
                  sets the V level to 3 in all Go files named "gopher.go"
                  - pattern="foo=3"
                  sets V to 3 in all files of any packages whose import path ends in "foo"
                  - pattern="foo/*=3"
                  sets V to 3 in all files of any packages whose import path contains "foo"
`,
	}
)

var AllLoggerFlags = []cli.Flag{
	&verbosityFlag,
	&vmoduleFlag,
}

// SetupLogger sets up the logger for the application using the provided context.
func SetupLogger(ctx *cli.Context) error {

	output := io.Writer(os.Stdout)
	handler := log.NewTerminalHandler(output, true)
	glogger := log.NewGlogHandler(handler)

	verbosity := log.FromLegacyLevel(ctx.Int(verbosityFlag.Name))
	glogger.Verbosity(verbosity)
	vmodule := ctx.String(vmoduleFlag.Name)
	err := glogger.Vmodule(vmodule)
	if err != nil {
		return fmt.Errorf("failed to set --%s: %w", vmoduleFlag.Name, err)
	}

	log.SetDefault(log.NewLogger(glogger))

	return nil
}
