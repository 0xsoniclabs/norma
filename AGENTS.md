# Agent instructions for Norma

These rules apply to every coding agent working in this repository. Claude
reads them through `CLAUDE.md`; GitHub Copilot reads this file directly.

## Go style

- Prefer a line width of 80 columns; never exceed 120.
- When a call returns only an error, check it inline:
  `if err := client.Close(); err != nil { return err }`.
- Prefer `range` loops, including over integers: `for i := range 10 { ... }`.
- Prefer `new(expr)` (Go 1.26) over declaring a variable to take its address.

## Test style

- Name tests `Test<Function>_<Expectation>_<WhenCase>`, for example
  `TestDryRun_ReportsTheStep_WhenItCannotRun`.
- Assert with `require` from `testify`.
- Write cases as table-driven tests, with the table a `map[string]fixtureType`
  keyed by the case name.
- Put helper types and functions that are not `Test*` at the end of the file.

## Commands

- `gofmt -w <file>`: format a Go file.
- `go test -run <test> ./...`: run selected tests.
- `golangci-lint run ./...`: lint.
- `go run ./driver/norma check <scenario.yml>`: check a scenario without
  running it.
- `go test ./scenarios/`: parse every scenario in the repository.

## Writing scenarios

[SCENARIO_SPECIFICATION.md](SCENARIO_SPECIFICATION.md) defines the format;
`go run ./driver/norma scenario-help` lists every step and its parameters.

- Fill in `Name` and `Description`; the description says what is tested and
  what is expected to happen.
- Name the file after the behaviour under test, without durations.
- The first step must start a validator.
- Keep the network alive: when nodes stop, enough stake must stay online. To
  give a validator the stake it needs, prefer its `stake` parameter over
  adding validators.
- Set rules the scenario needs from the start in `InitialNetworkRules`, not
  with an `updateRules` step.
- Leave `imageName` out for the client under test; that is the `sonic`
  submodule. Pin released versions such as `sonic:v2.1.6` only as
  [RELEASE_TESTING.md](RELEASE_TESTING.md) describes, and never write
  `sonic:local`.
- Unless `DisableEndChecks: true` is set, every scenario ends with two
  `advanceEpoch` steps and the `blockHashes` and `blockHeights` checks. Do not
  repeat them; set `DisableEndChecks: true` when the scenario deliberately
  halts the network.
- Add `advanceEpoch` only when the test needs an epoch transition.
- Give every expectation the scenario states a check, placed where it must
  hold rather than only at the end. For example, quorum surviving a validator
  going offline is a `blocksProduced` check during the offline window.
- Do not end with `stopApp` unless stopping the application is part of the
  test.
- If a requirement would halt the network, say so and propose a safe
  alternative.
