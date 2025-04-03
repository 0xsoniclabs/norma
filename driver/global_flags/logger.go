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
		Usage: "Logging verbosity: 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=detail",
		Value: 3,
	}

	vmoduleFlag = cli.StringFlag{
		Name:  "vmodule",
		Usage: "Per-module verbosity: comma-separated list of <pattern>=<level> (e.g. eth/*=5,p2p=4)",
		Value: "",
	}
)

var AllLoggerFlags = []cli.Flag{
	&verbosityFlag,
	&vmoduleFlag,
}

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
