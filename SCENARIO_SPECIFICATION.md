# Scenario Specification

This document specifies the YAML format used to describe **scenarios** for
Norma. It is written for both humans and coding agents that need to author,
review, or generate scenario files.

A scenario is an ordered list of blocking **steps**. Each step executes to
completion before the next one starts. Between steps, the runner
transparently waits for the network to remain healthy (block production
continues) unless the step is one that is expected to leave the network
temporarily inactive.

Scenario files live under `scenarios/` and are consumed by the parser in
[driver/parser/scenario.go](driver/parser/scenario.go) and executed by
[driver/executor/run.go](driver/executor/run.go).

---

## 1. Top-Level Structure

```yaml
Name: <string>              # required
Description: <string>       # required
InitialNetworkRules:        # optional, applied at genesis
  <NetworkRulesPatch>
DisableEndChecks: <bool>    # optional, default false
Scenario:                   # required, ordered list of steps
  - <step>
  - <step>
```

### Required fields

| Field         | Type   | Notes                                                    |
| ------------- | ------ | -------------------------------------------------------- |
| `Name`        | string | Non-empty. Displayed in reports and logs.                |
| `Description` | string | Non-empty. One or two sentences describing intent.       |
| `Scenario`    | list   | Ordered list of steps. May be empty (before end-checks). |

### Optional fields

| Field                 | Type                | Default                                                              |
| --------------------- | ------------------- | -------------------------------------------------------------------- |
| `InitialNetworkRules` | `NetworkRulesPatch` | See [§4](#4-network-rules-patch). `MaxEpochDuration` defaults apply. |
| `DisableEndChecks`    | bool                | `false`                                                              |

### End-of-scenario checks

Unless `DisableEndChecks: true` is set, the parser automatically appends the
following steps to every scenario:

```yaml
- advanceEpoch
- advanceEpoch
- checks:
    - blockHashes
    - blockHeights
```

Set `DisableEndChecks: true` for scenarios that intentionally halt the network
(e.g. stopping all validators) — otherwise the automatic checks will fail.

### Strict YAML parsing

The parser rejects **unknown keys** at every level. Misspelled fields (for
example `Descripton:` or `stkae:`) will cause the scenario to fail to load.

### Node and app names

Node identifiers and app identifiers must match the regular expression
`^[A-Za-z0-9-.]+$`. Underscores, spaces, and other punctuation are not
allowed.

---

## 2. Step Syntax

A step is written as a YAML mapping in which **one key is the step function
name** and the remaining keys are that step’s parameters. Some steps that
take no parameters may be written as a bare string.

```yaml
# Mapping form
- startNode: val-1
  type: validator
  stake: 5_000_000

# Bare-string form (no parameters)
- advanceEpoch
```

Rules:

- Exactly one function key per step. Combining `startNode:` and `stopNode:` in
  the same mapping is an error.
- A parameter that is not valid for the chosen function is an error
  (e.g. `stake:` on `runApp:`).
- Unknown function names are rejected.

---

## 3. Step Functions

The table below lists every valid step function. Sections that follow give
detailed parameter semantics for the non-trivial ones.

| Function       | Purpose                                                  |
| -------------- | -------------------------------------------------------- |
| `startNode`    | Start a network node (validator, observer, or rpc).      |
| `stopNode`     | Stop a running node.                                     |
| `undelegate`   | Undelegate stake from one or more validators.            |
| `updateRules`  | Change network rules at runtime.                         |
| `advanceEpoch` | Force an epoch seal via an on-chain transaction.         |
| `waitForEpoch` | Wait until the network reaches the next epoch boundary. |
| `runApp`       | Start a load-generating application.                     |
| `stopApp`      | Stop a running load-generating application.              |
| `checks`       | Run one or more health checks.                           |
| `waitFor`      | Pause scenario execution for a fixed duration.           |

### 3.1 `startNode`

Creates a node. The value is the node’s **identifier** (its label).

```yaml
- startNode: validator-1
  type: validator          # one of: validator, observer, rpc
  imageName: sonic:local   # optional; default: DefaultClientDockerImageName
  dataVolume: my-volume    # optional; Docker volume mounted as data dir
  stake: 5_000_000         # optional; validator stake in S, default 5_000_000
  instances: 3             # optional; create N nodes named <id>-0..<id>-N-1
  failing: false           # optional; when true, the node is expected to fail
  extraArguments: "--..."  # optional; passed to sonicd command line
```

Parameter details:

- **`type`** — Default is `observer` when omitted; validators register on-chain
  before the node is created.
- **`instances`** — When `> 1`, node names become `<id>-0`, `<id>-1`, ….
  When `1` (or omitted), the name is used as-is.
- **`stake`** — Only meaningful for validators. Underscores may be used in the
  numeric literal (`10_000_000`).
- **`failing`** — When `true`, the runner skips the post-step block-production
  wait and does **not** wait for the node to sync. A passing node in this state
  is treated as an error by later checks.
- **`dataVolume`** — Named Docker volume that persists across `stopNode` /
  `startNode` of the same identifier (used for rejoin-with-state scenarios).

**Rejoin semantics:** Calling `startNode` with an identifier that was
previously started and stopped is treated as a rejoin. No new validator is
registered on-chain; the preserved validator ID is reused. A rejoining
validator that has no preserved ID is an error.

### 3.2 `stopNode`

Stops a running node by identifier. Takes no parameters.

```yaml
- stopNode: validator-1
```

If the node was started with `instances: N`, the identifier stops **all**
instances of that name (i.e. every `<id>-i`).

### 3.3 `undelegate`

Undelegates stake from one or more validator nodes. The value is either a
bare node name (shorthand) or a list of targets.

```yaml
# Shorthand: full self-stake of a single validator
- undelegate: validator-1

# List form
- undelegate:
    - node: heavy
      stake: 1_000_000     # optional; if omitted, full self-stake is used
    - node: medium         # no stake => full self-stake
```

Each target’s `node` must be a valid node name. `stake` is optional; when
omitted, the current self-stake of the validator is queried on-chain and
fully undelegated.

### 3.4 `updateRules`

Applies a network rules patch at runtime. The value is a `NetworkRulesPatch`
mapping (see [§4](#4-network-rules-patch)); at least one field must be set.

```yaml
- updateRules:
    Blocks:
      MaxBlockGas: 10_000_000_000
    Upgrades:
      Brio: true
```

Rules changes take effect at the next epoch seal. Follow an `updateRules`
step with `waitForEpoch` (typically twice) if the next step depends on the
new rules being active.

### 3.5 `advanceEpoch`

Force the current epoch to seal by submitting the appropriate on-chain
transaction. Takes no value.

```yaml
- advanceEpoch
```

The runner waits for block production before and after this call to ensure
the transition is observed.

### 3.6 `waitForEpoch`

Blocks until the network naturally advances to the next epoch. Takes no
value.

```yaml
- waitForEpoch
```

Use this to observe passive epoch transitions (e.g. after `MaxEpochDuration`
elapses). Use `advanceEpoch` when you need to force one.

### 3.7 `runApp`

Starts a load-generating application. The value is the application’s
**identifier**.

```yaml
- runApp: load
  type: counter            # required; see supported types below
  users: 50                # optional; number of concurrent user accounts
  rate:                    # required
    constant: 20           # Tx/s
```

**Supported application types** (case-insensitive): `counter`, `erc20`,
`store`, `uniswap`, `smartaccount`, `subsidies`, `transient`,
`selfdestructoldcontract`, `selfdestructnewcontract`, `ecdsa`,
`largecontract`, `allofbundle`, `oneofbundle`, `subsidizedbundle`,
`failingbundle`, `duplicatedbundle`, `bls12add`, `mix`.

**Rate shapes** — exactly one of the following must be set on `rate`:

```yaml
rate:
  constant: 20             # constant Tx/s

rate:
  slope:                   # linearly increasing rate
    start: 5               # starting Tx/s
    increment: 1           # increase per second

rate:
  wave:                    # sinusoidal rate
    min: 5                 # optional; default 0
    max: 50                # Tx/s at peak
    period: 30             # seconds per cycle

rate:
  auto:                    # auto-tune to max throughput
    increase: 1            # optional; +Tx/s per second when not overloaded
    decrease: 0.2          # optional; fractional decrease on overload
```

### 3.8 `stopApp`

Stops a running load-generating application by identifier. Takes no parameters.

```yaml
- stopApp: load
```

### 3.9 `checks`

Runs one or more health checks. The value is a **list** of check
specifications (see [§5](#5-check-functions)).

```yaml
- checks:
    - blocksProduced:
        tolerance: 10
    - blockHashes
    - blockHeights:
        tolerance: 5
```

Each item is either a bare check-function name or a mapping whose key is the
check function and whose siblings are that check’s parameters.

### 3.10 `waitFor`

Pauses scenario execution for a fixed duration. The value is a Go duration
string (`10s`, `1m`, `1h30m`, …) and must be positive.

```yaml
- waitFor: 15s
```

---

## 4. Network Rules Patch

`InitialNetworkRules` and the value of `updateRules` share the same schema:
`NetworkRulesPatch`. All fields are optional; only set fields are applied.

```yaml
Dag:
  MaxParents: <uint64>
  MaxFreeParents: <uint64>
  MaxExtraData: <uint32>

Emitter:
  Interval: <duration>
  StallThreshold: <duration>
  StalledInterval: <duration>

Epochs:
  MaxEpochGas: <uint64>
  MaxEpochDuration: <duration>

Blocks:
  MaxBlockGas: <uint64>
  MaxEmptyBlockSkipPeriod: <duration>

Economy:
  BlockMissedSlack: <uint64>
  MinGasPrice: <bigint>
  MinBaseFee: <bigint>
  Gas:
    MaxEventGas: <uint64>
    EventGas: <uint64>
    ParentGas: <uint64>
    ExtraDataGas: <uint64>
    BlockVotesBaseGas: <uint64>
    BlockVoteGas: <uint64>
    EpochVoteGas: <uint64>
    MisbehaviourProofGas: <uint64>
  ShortGasPower:
    AllocPerSec: <uint64>
    MaxAllocPeriod: <duration>
    StartupAllocPeriod: <duration>
    MinStartupGas: <uint64>
  LongGasPower:
    AllocPerSec: <uint64>
    MaxAllocPeriod: <duration>
    StartupAllocPeriod: <duration>
    MinStartupGas: <uint64>

Upgrades:
  Berlin: <bool>
  London: <bool>
  Llr: <bool>
  Sonic: <bool>
  Allegro: <bool>
  Brio: <bool>
  SingleProposerBlockFormation: <bool>
  GasSubsidies: <bool>
  TransactionBundles: <bool>
```

Type notes:

- **`<duration>`** — Go duration string (`"15s"`, `"1m30s"`) or an integer
  nanosecond count.
- **`<bigint>`** — YAML scalar decimal integer (unquoted is fine for values
  that fit in an `int64`; quote larger values).
- **`<uintN>`** — Non-negative integer literal. Underscores are allowed in
  the numeric literal (`10_000_000`).

The canonical Go type is
[`NetworkRulesPatch`](genesis/rules_patch.go).

---

## 5. Check Functions

Checks appear as items inside a `checks:` step. Each entry is either a bare
function name or a mapping.

| Function         | Purpose                                                         | Parameters                       |
| ---------------- | --------------------------------------------------------------- | -------------------------------- |
| `blockGasRate`   | Assert block gas rate ≤ ceiling.                                | `ceiling`, `failing`             |
| `blockHashes`    | Assert all nodes agree on block hashes.                         | `failing`                        |
| `blockHeights`   | Assert all nodes are within tolerance of the same height.       | `tolerance`, `failing`           |
| `blocksHalted`   | Assert block production has halted.                             | `failing`                        |
| `blocksProduced` | Assert all nodes produce blocks within tolerance over duration. | `tolerance`, `duration`, `failing` |
| `networkRules`   | Assert the active rules on all nodes match the given patch.     | `rules`, `failing`               |

### Parameter reference

| Parameter   | Type              | Meaning                                                                                          |
| ----------- | ----------------- | ------------------------------------------------------------------------------------------------ |
| `ceiling`   | float             | Maximum allowed value (used by `blockGasRate`).                                                  |
| `tolerance` | int (blocks)      | Allowed deviation between nodes.                                                                 |
| `duration`  | duration string   | Non-negative window over which to observe the network (used by `blocksProduced`).                |
| `rules`     | `NetworkRulesPatch` | Expected rule set; every set field must equal the value reported by every node.                |
| `failing`   | bool              | When `true`, the check is **expected to fail**; a passing result is treated as an error.        |

### Examples

```yaml
- checks:
    - blocksProduced                        # bare form
    - blocksProduced:                       # with parameters
        tolerance: 10
        duration: 30s
    - blockGasRate:
        ceiling: 1_000_000_000_000_000
    - blockHashes
    - blockHeights:
        tolerance: 5
    - blocksHalted
    - networkRules:
        rules:
          Epochs:
            MaxEpochDuration: 10s
          Blocks:
            MaxBlockGas: 10_000_000_000
```

---

## 6. Runner Behaviour

### 6.1 Timeout

Every scenario has a hard wall-clock deadline (currently
**10 minutes**). Long scenarios must respect this limit; abort at deadline
causes an error naming the step that was in flight.

### 6.2 Between-step block-production wait

After steps that actively modify the network and expect it to stay healthy
(e.g. `startNode` of a working node, `updateRules`, `runApp`), the runner
transparently waits for the network to produce at least one new block before
starting the next step. This is skipped for steps that legitimately leave
the network idle: `stopNode`, `waitFor`, `checks`, and any `startNode` with
`failing: true`.

### 6.3 Node sync wait

After `startNode` (unless `failing: true`), the runner waits for each new
node to reach the current network block height before proceeding. This
means the network must already have a live block source before adding
observers or RPC nodes.

### 6.4 Genesis validators

Nodes named in the genesis (via the driver’s genesis validator map) are
started **without** an on-chain registration step, and their pre-assigned
validator IDs are reused. This lets a scenario’s first `startNode` step
simply reference a genesis validator by label.

### 6.5 Error surfaces

- Parse errors reject the file at load time, with a line number.
- Semantic errors (empty name, invalid application type, invalid rules,
  etc.) are reported before execution starts, aggregated across all steps.
- Runtime errors abort the scenario at the failing step and are reported
  with the step index, function, identifier, and underlying cause.

---

## 7. Complete Example

```yaml
Name: Change Network Rules Test
Description: >-
  Verifies that network rules can be updated at runtime
  and that the changes take effect across nodes.

InitialNetworkRules:
  Epochs:
    MaxEpochDuration: 10s
    MaxEpochGas: 1_500_000_000_000
  Blocks:
    MaxBlockGas: 20_500_000_000

Scenario:
  - startNode: local
    type: validator
    imageName: sonic:local

  - startNode: v2.1.6
    type: validator
    imageName: sonic:v2.1.6

  - waitFor: 10s

  - checks:
      - blocksProduced
      - networkRules:
          rules:
            Blocks:
              MaxBlockGas: 20_500_000_000

  - updateRules:
      Blocks:
        MaxBlockGas: 10_000_000_000

  - waitForEpoch
  - waitForEpoch

  - checks:
      - blocksProduced
      - networkRules:
          rules:
            Blocks:
              MaxBlockGas: 10_000_000_000
```

More runnable examples can be found in the
[scenarios/](scenarios) directory.

---

## 8. Authoring Checklist

Use this list when writing or reviewing a scenario:

- [ ] `Name` and `Description` are set and non-empty.
- [ ] Every node/app identifier matches `^[A-Za-z0-9-.]+$`.
- [ ] Every `startNode` step names a `type` (or is intentionally an observer).
- [ ] Every `runApp` step specifies a valid `type` and a `rate`.
- [ ] `stopNode` / `stopApp` identifiers refer to previously started ones.
- [ ] `undelegate` targets refer to running validators, and `stake` is only
      set when a partial undelegation is intended.
- [ ] `updateRules` patches contain at least one field, and are followed by
      one or two `waitForEpoch` steps if the next step depends on the change.
- [ ] Scenarios that intentionally halt the network set
      `DisableEndChecks: true`.
- [ ] Every check that is expected to fail is marked `failing: true`.
- [ ] Total wall-clock time (including waits and block production) fits
      within the 10-minute runner timeout.

---

## 9. Getting Help from the Tool

The `norma` binary can print an authoritative summary of every step
function, parameter, and check, generated directly from the parser:

```sh
go run ./driver/norma scenario-help
```

Use this output as the source of truth if this document and the parser ever
disagree.
