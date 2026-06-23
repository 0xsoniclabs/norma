# Copilot Instructions for `norma`

These instructions apply to all Go code and tests in this repository.

## Go style

- Prefer line width of 80 columns.
- Enforce a hard maximum line width of 120 columns.
- If a function call returns only an error, prefer the inline error-check pattern:
  - `if err := doThing(); err != nil { ... }`
- Prefer `range` syntax for loops when possible, including integer ranges:
  - `for i := range 10 { ... }`
- Prefer Go 1.26 `new(...)` expression style where applicable, instead of declaring a variable and taking its address.

## Test style

- Keep all helper types and helper functions that are not `Test*` at the end of the file.
- Name tests as:
  - `Test<Function>_<Expectation>_<WhenCase>`
- Use `require` from `testify` for assertions when `testify` is available in `go.mod`.
- For tests with multiple cases, use table-driven tests.
- Define test tables as `map[string]fixtureType`.

## Examples

- Single-error call:
  - `if err := client.Close(); err != nil { return err }`
- Integer range loop:
  - `for i := range 10 { total += i }`
- Table test shape:
  - `cases := map[string]fixtureType{"valid": {...}, "invalid": {...}}`
