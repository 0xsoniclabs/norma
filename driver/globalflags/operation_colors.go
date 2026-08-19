package globalflags

import (
	"github.com/0xsoniclabs/norma/driver/parser"
)

// This file holds the colour table used by the log handler in
// operation_color_handler.go, and nothing else. It is kept separate so that
// re-colouring an operation, or colouring a newly added one, is a change to a
// lookup table rather than a change to logging logic.

// colorReset ends a coloured run.
const colorReset = "\x1b[0m"

// operationColors gives every scenario operation its own colour. On a log line
// that names an operation, the message and the operation name are printed in
// that colour, so the steps stand out from the rest of the output and from each
// other.
//
// The values are 256-colour SGR sequences. Bright reds and yellows are avoided
// because go-ethereum's terminal handler uses them for warning/error levels,
// and an operation colour should not read as a failure.
//
// Every operation in parser.AllStepFunctions must appear here;
// TestOperationColors_OperationColor_CoversEveryStepFunction enforces that.
var operationColors = map[parser.StepFunction]string{
	// Node lifecycle.
	parser.FuncStartNode: "\x1b[38;5;42m",  // green — a node comes up
	parser.FuncStopNode:  "\x1b[38;5;208m", // orange — a node goes down
	parser.FuncKillSonic: "\x1b[38;5;199m", // pink — a node is killed outright
	parser.FuncHealDb:    "\x1b[38;5;51m",  // cyan — a node is repaired

	// Load generation.
	parser.FuncRunApp:  "\x1b[38;5;33m", // blue
	parser.FuncStopApp: "\x1b[38;5;63m", // indigo

	// Staking.
	parser.FuncDelegate:     "\x1b[38;5;79m",  // aquamarine
	parser.FuncUndelegate:   "\x1b[38;5;173m", // clay
	parser.FuncVerifyStakes: "\x1b[38;5;156m", // pale green

	// Network state.
	parser.FuncUpdateRules:  "\x1b[38;5;214m", // amber
	parser.FuncAdvanceEpoch: "\x1b[38;5;141m", // purple
	parser.FuncWaitForEpoch: "\x1b[38;5;109m", // steel blue

	// Passive steps.
	parser.FuncChecks:  "\x1b[38;5;220m", // gold
	parser.FuncWaitFor: "\x1b[38;5;244m", // grey — nothing is happening
}

// operationColor returns the colour assigned to op, or false if op has none.
func operationColor(op parser.StepFunction) (string, bool) {
	color, ok := operationColors[op]
	return color, ok
}
