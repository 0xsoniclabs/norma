// globalflags is a package that provides global flags for the driver.
// This package can be used to define global flags that are common across different commands.
// usage of included flags can be seen by running `go run ./driver/norma --help`.
package globalflags

import "github.com/urfave/cli/v2"

// AllGlobalFlags aggregates all global flags for the application that are
// common across different commands.
var AllGlobalFlags = append(
	[]cli.Flag{},

	// Add flags related to slog configuration
	AllLoggerFlags...,
)

// ProcessGlobalFlags processes global flags for the application.
// It processes the setup of any global flag included in the AllGlobalFlags list.
func ProcessGlobalFlags(c *cli.Context) error {
	return SetupLogger(c)
}
