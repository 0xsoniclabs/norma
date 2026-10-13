package globalflags

// This file holds the colour table used by the log handler in
// operation_color_handler.go, and nothing else. It is kept separate so that
// re-colouring an operation, or colouring a newly added one, is a change to a
// lookup table rather than a change to logging logic.

// colorReset ends a coloured run.
const colorReset = "\x1b[0m"

// phaseColors gives each value of the "phase" attribute on operation lines its
// own colour, so whether a step is starting or done is told apart at a glance.
// The keys mirror the phase constants in driver/executor/run.go, which logs the
// attribute.
var phaseColors = map[string]string{
	"started":   "\x1b[38;5;37m",         // teal — the step is under way
	"completed": "\x1b[38;2;66;132;255m", // blue (#4284FF) — the step is done
}

// phaseColor returns the colour assigned to phase, or false if phase has none.
func phaseColor(phase string) (string, bool) {
	color, ok := phaseColors[phase]
	return color, ok
}

// operationColors gives every scenario operation its own colour. On a log line
// that names an operation, the message and the operation name are printed in
// that colour, so the steps stand out from the rest of the output and from each
// other.
//
// The values are 256-colour SGR sequences. Bright reds and yellows are avoided
// because go-ethereum's terminal handler uses them for warning/error levels,
// and an operation colour should not read as a failure.
//
// Every step in parser.StepNames must appear here;
// TestOperationColors_OperationColor_CoversEveryStepFunction enforces that.
var operationColors = map[string]string{
	// Node lifecycle.
	"startNode": "\x1b[38;5;42m",  // green — a node comes up
	"stopNode":  "\x1b[38;5;208m", // orange — a node goes down
	"killSonic": "\x1b[38;5;199m", // pink — a node is killed outright
	"healDb":    "\x1b[38;5;51m",  // cyan — a node is repaired

	// Load generation.
	"runApp":  "\x1b[38;5;33m", // blue
	"stopApp": "\x1b[38;5;63m", // indigo

	// Staking.
	"delegate":     "\x1b[38;5;79m",  // aquamarine
	"undelegate":   "\x1b[38;5;173m", // clay
	"verifyStakes": "\x1b[38;5;156m", // pale green

	// Network state.
	"updateRules":  "\x1b[38;5;214m", // amber
	"advanceEpoch": "\x1b[38;5;141m", // purple
	"waitForEpoch": "\x1b[38;5;109m", // steel blue

	// Passive steps.
	"checks":        "\x1b[38;5;220m", // gold
	"waitFor":       "\x1b[38;5;244m", // grey — nothing is happening
	"waitForBlocks": "\x1b[38;5;248m", // light grey — waiting on the chain

	// Client lifecycle and file management.
	"exportEvents":     "\x1b[38;5;116m", // teal
	"importEvents":     "\x1b[38;5;110m", // slate
	"checkDb":          "\x1b[38;5;187m", // sand -- a database is verified
	"exportGenesis":    "\x1b[38;5;111m", // light blue
	"importGenesis":    "\x1b[38;5;105m", // violet
	"waitForSonicExit": "\x1b[38;5;137m", // tan -- the client goes down by itself
	"stopSonic":        "\x1b[38;5;130m", // burnt orange -- the client goes down gently
}

// operationColor returns the colour assigned to op, or false if op has none.
func operationColor(op string) (string, bool) {
	color, ok := operationColors[op]
	return color, ok
}
