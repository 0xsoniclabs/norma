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
`store`, `uniswap`, `smartaccount`, `subsidies`, `priority`, `transient`,
`selfdestructoldcontract`, `selfdestructnewcontract`, `ecdsa`,
`largecontract`, `allofbundle`, `oneofbundle`, `subsidizedbundle`,
`failingbundle`, `duplicatedbundle`, `bls12add`, `mix`.

A priority lane is a property of a load rather than a load of its own, so
`priority` does not generate traffic itself: the optional `load` parameter names
the type that does, and the accounts signing it are registered in the on-chain
priority registry.

```yaml
- runApp: fast
  type: priority
  load: uniswap            # optional; any type above, defaults to counter
  rate:
    constant: 20
```

Running that next to a plain `uniswap` application isolates the effect of the
feature, since the two loads differ in nothing else. `load` applies to `priority`
only; on any other type it is an error, as is nesting one lane in another.

`priority` requires the `TransactionPriorities` upgrade to be active and fails
to start otherwise; see
[scenarios/examples/priority_lanes.yml](scenarios/examples/priority_lanes.yml)
for a scenario making the effect of the lanes visible.

The loads whose users create the accounts they sign with while they run - the
bundle types and `mix` - cannot be prioritized: their accounts are not known when
the registry is written, and registering the ones that happen to exist would
prioritize a part of their traffic and quietly leave the rest behind. Asking for
them reports that rather than doing it.

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
  TransactionPriorities: <bool>
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

| Function           | Purpose                                                          | Parameters                                    |
| ------------------ | ---------------------------------------------------------------- | --------------------------------------------- |
| `blockGasRate`     | Assert block gas rate ≤ ceiling over an observation window.      | `ceiling`, `tolerance`, `duration`, `failing` |
| `blockHashes`      | Assert all nodes agree on block hashes.                          | `failing`                                     |
| `blockHeights`     | Assert all nodes are within tolerance of the same height.        | `tolerance`, `duration`, `failing`            |
| `blocksHalted`     | Assert block production has halted over an observation window.   | `tolerance`, `duration`, `failing`            |
| `blocksProduced`   | Assert the network produces blocks over an observation window.   | `tolerance`, `duration`, `failing`            |
| `eventThrottled`   | Assert the listed validators emit events far slower than others. | `throttledNodes`, `failing`                   |
| `networkRules`     | Assert the active rules on all nodes match the given patch.      | `rules`, `duration`, `failing`                |
| `validatorsActive` | Assert every running validator is in the epoch's validator set.  | `failing`                                     |

### 5.1 Windows of time

Every check fixes the span it judges **when it starts**, and never reads data
recorded before that instant. There are four shapes.

| Check            | Window kind            | Anchored at                  | Judges                                 |
| ---------------- | ---------------------- | ---------------------------- | -------------------------------------- |
| `blocksProduced` | Forward observation    | now, at entry                | Height samples taken while waiting     |
| `blocksHalted`   | Forward observation    | now, at entry                | Height samples taken while waiting     |
| `blockGasRate`   | Forward observation    | chain head, at entry         | Blocks produced while waiting          |
| `blockHeights`   | Convergence budget     | now + budget, at entry       | Live heights, re-read until they agree |
| `networkRules`   | Convergence budget     | now + budget, at entry       | Live rules, re-read until they agree   |
| `blockHashes`    | Fixed block range      | lowest head of healthy nodes | Blocks 0…that head, settled everywhere |
| `eventThrottled` | Two measured snapshots | each DAG head query          | Event delta ÷ the interval measured    |

**Forward observation.** The check notes the current instant, waits for its
window, and then looks only at what the monitor collected while it waited.
This matters because a step does not necessarily leave the network in its final
state the moment it returns — after a `stopNode`, blocks keep being produced for
a few seconds until the remaining validators time out. A check looking backwards
would see that production and fail; a forward window cannot see it at all. So
scenarios do **not** need to pad a transition with a `waitFor` long enough to
push it out of a check's field of view.

`blockGasRate` is a forward window expressed in block numbers rather than
timestamps: it records the chain head, waits, and inspects the blocks that
appeared in between.

**Convergence budget.** These checks compare nodes against each other at one
instant, which asks the network to be settled exactly then. It often is not: an
epoch seal reaches nodes a moment apart, and a rejoining validator is behind for
a while. So they re-read every 500ms until the nodes agree, and report the last
disagreement only if it outlives the budget. A network in the expected state
passes on the first read and costs nothing.

**Fixed block range.** `blockHashes` compares immutable history, so a forward
window makes no sense; instead it reads every healthy node's head once and
compares blocks 0 to the lowest of them. Those blocks exist on every node, so
the walk terminates and the verdict does not depend on which node was ahead
while it ran. Blocks above that point are left for the next check rather than
being read as a disagreement.

**Measured snapshots.** `eventThrottled` derives emission rates from two DAG
snapshots. Walking a DAG costs one RPC per event, so the snapshots can end up
further apart than the requested window; the rates are divided by the interval
actually measured, not by the requested one. A snapshot is timestamped when its
heads are queried, since the heads fix which events get counted.

### 5.2 Parameter reference and defaults

| Parameter        | Type                | Meaning                                                                         |
| ---------------- | ------------------- | ------------------------------------------------------------------------------- |
| `ceiling`        | float               | Maximum allowed gas rate.                                                       |
| `tolerance`      | int                 | Meaning depends on the check — see below.                                       |
| `duration`       | duration string     | Observation window (minimum **2s**), or convergence budget — see below.         |
| `rules`          | `NetworkRulesPatch` | Expected rule set; every field set must equal the value reported by every node. |
| `throttledNodes` | list of strings     | Node labels expected to be throttled. Required; every label must resolve.       |
| `failing`        | bool                | When `true` the check is **expected to fail**; a passing result is an error.    |

`tolerance` and `duration` are deliberately overloaded; this is what they mean
per check, with the value used when the parameter is omitted:

| Check            | `tolerance`                       | `duration`                   | Other                       |
| ---------------- | --------------------------------- | ---------------------------- | --------------------------- |
| `blocksProduced` | window, in samples — **10** (10s) | overrides the window — unset | —                           |
| `blocksHalted`   | window, in samples — **10** (10s) | overrides the window — unset | —                           |
| `blockGasRate`   | window, in samples — **10** (10s) | overrides the window — unset | `ceiling` — **unbounded**   |
| `blockHeights`   | height slack, in blocks — **5**   | convergence budget — **30s** | —                           |
| `networkRules`   | —                                 | convergence budget — **30s** | `rules` — required          |
| `blockHashes`    | —                                 | —                            | —                           |
| `eventThrottled` | —                                 | —                            | `throttledNodes` — required |

Two things to keep in mind:

- For `blocksProduced`, `blocksHalted` and `blockGasRate`, `tolerance` is a
  **window length counted in monitoring samples** (the monitor samples each node
  once per second, so 10 ≈ 10s). For `blockHeights` it is a **height deviation
  in blocks**. `duration`, when given, always wins over `tolerance`.
- `duration` is an observation window for the three observing checks, but a
  convergence budget for `blockHeights` and `networkRules`.

Both floors are enforced when the scenario is loaded, not minutes into the run:
on a check that reads the parameter as a window, a `duration` below 2s is
rejected, and so is a `tolerance` below 2. Deciding whether a height changed
takes two samples, and a shorter window could only ever report that nothing was
seen.

Neither floor applies to a `duration` used as a convergence budget. Nothing has
to be sampled twice for `blockHeights` or `networkRules` to reach a verdict, so a
short budget is legitimate, and `0` is meaningful: it asks for a single attempt
with no retry.

### 5.3 Values not configurable from a scenario

These are compiled in. They are listed because they explain the numbers above
and the cost of a check.

| Constant                    | Value | Role                                                               |
| --------------------------- | ----- | ------------------------------------------------------------------ |
| Monitor sampling interval   | 1s    | Converts a `tolerance` in samples into a duration.                 |
| Minimum observation samples | 2     | Floor behind the 2s minimum observation `duration`.                |
| Convergence poll interval   | 500ms | How often `blockHeights` and `networkRules` re-read.               |
| Minimum comparable block    | 2     | `blockHashes` refuses to judge a shorter chain.                    |
| Minimum gap ratio           | 2.0   | `eventThrottled`: unthrottled must emit ≥2× the fastest throttled. |
| DAG sample window           | 5s    | `eventThrottled` interval between snapshots.                       |
| DAG sample attempts         | 5     | Retries when a window straddles an epoch seal.                     |
| DAG retry backoff           | 2s    | Pause between discarded attempts.                                  |

### 5.4 What a check costs

| Check                                            | Wall-clock cost                                                                                                              |
| ------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------- |
| `blocksProduced`, `blocksHalted`, `blockGasRate` | Always the full window (10s by default).                                                                                     |
| `blockHeights`, `networkRules`                   | Nothing when the network is already in the expected state; up to the budget (30s) otherwise, including when `failing: true`. |
| `blockHashes`                                    | No waiting; RPC-bound, roughly one call per block per node.                                                                  |
| `eventThrottled`                                 | 5s plus DAG walking time per attempt, up to 5 attempts with 2s pauses.                                                       |

The scenario deadline is 10 minutes, so budget the observing checks accordingly.
Against the default 15s epoch a 10s window will often span an epoch seal, which
is harmless for all of them: a seal does not stop block production, and each
block is compared on its own.

### 5.5 Examples

```yaml
- checks:
    - blocksProduced                        # bare form, 10s window
    - blocksProduced:                       # with parameters
        tolerance: 10
        duration: 30s
    - blockGasRate:
        ceiling: 1_000_000_000_000_000
        duration: 20s
    - blockHashes
    - blockHeights:
        tolerance: 5                        # blocks of slack, not a window
        duration: 60s                       # time allowed to converge
    - blocksHalted:
        duration: 15s
    - eventThrottled:
        throttledNodes: [validator-2, validator-3]
    - networkRules:
        rules:
          Epochs:
            MaxEpochDuration: 10s
          Blocks:
            MaxBlockGas: 10_000_000_000
    - validatorsActive
```

> `validatorsActive` asserts the guarantee described in
> [§6.4](#64-validator-activation): a node started as `type: validator` is
> really a validator. It compares the running validator nodes against the
> validator set of the epoch being built, rather than observing event
> emission — a validator that just joined, or whose empty events the event
> throttler suppresses, is a full member yet emits rarely. See
> [`validators_are_really_validators.yml`](scenarios/examples/validators_are_really_validators.yml).
>
> Every running validator node has to be a member, so the check does not fit a
> scenario that leaves one running after taking it out of the set: a node that
> has been [`undelegate`](#4-step-functions)d but not yet stopped fails it, by
> design, because it is no longer in the set. Place the check where the running
> validators are the ones the scenario means to have, or use `failing: true` on
> nodes that are expected to have left.

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

### 6.4 Validator activation

Every node started as `type: validator` is guaranteed to be a validator in
the network by the time the step completes — a scenario needs no epoch
handling of its own for that.

Only the first `startNode` step forms the genesis validator set. A validator
introduced by any later step is created in the SFC contract instead, and SFC
validator sets are **per-epoch**: such a validator carries no stake weight
and emits no events until the epoch it was created in is sealed. Left alone
it would sit in the network as an observer while the genesis validators
carried the whole consensus load.

So `startNode` reads the validator set, seals an epoch if a validator it
started is missing from it, and waits until all of them are reported in it,
failing the step if they never arrive. The seal happens *after* the new nodes
are up and synced (§6.3), because a validator gains stake weight the moment it
joins the set — admitting it earlier would deprive the network of that share
of its online stake.

Membership is read rather than assumed, so a rejoin (a `startNode` reusing an
earlier identifier) is held to the same guarantee as a first start. It keeps
its preserved validator ID and normally is still in the set, which costs the
scenario nothing but that read; if it is not — its stake was undelegated while
it was down, say — the step fails instead of letting the node rejoin as an
observer. Genesis validators, whose pre-assigned IDs are reused, are in the
set from the first block and are not checked.

A `startNode` with `failing: true` is registered but not activated, as it is
also excluded from the waits of §6.2 and §6.3: a node that may never come up
must not be the subject of an assertion. Such a validator joins at the next
epoch the scenario seals, or at the next `startNode` that has to seal one, so
the guarantee above covers working nodes only — which is also why
[`validatorsActive`](#5-check-functions) skips them.

The [`validatorsActive`](#5-check-functions) check asserts this end to end.

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
