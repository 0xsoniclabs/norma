# Metrics

Norma observes the network while a scenario runs and writes what it measures to `measurements.csv` in the run's output directory. The rendered report is built from this file; it can also be filtered with `grep` or loaded into any analysis tool.

## The CSV format

Every line of `measurements.csv` is one measurement, and all metrics share the
same columns:
```
| Metric | Network | Node | App | Time | Block | Workers | Value |
```
* `Metric`: the name of the metric.
* `Network`: the network name, currently always the same.
* `Node`: the node the measurement belongs to, if any.
* `App`: the application (load generator) the measurement belongs to, if any.
* `Time`: the timestamp, for metrics measured over time.
* `Block`: the block number, for metrics measured per block.
* `Workers`: the number of workers sending transactions, for metrics measured
  over it.
* `Value`: the measured value; always set.

Columns that do not apply to a metric are left empty. To look at one metric,
filter by its name:
```
grep TransactionsThroughput measurements.csv
```

## Transaction Lifecycle Metrics

The transactions of load generators are followed individually, from the moment they are submitted to a node until they are part of a block. They are attributed to the application that produced them, so the `App` column tells which load a measurement belongs to. Three moments are observed: a transaction is **submitted** when a node's RPC interface accepted it, **emitted** when a validator put it into an event of the DAG, and **included** when it became part of a block.

Counted once per second, per application:
* `TransactionsPending` - submitted transactions the network could emit right away, waiting for capacity
* `TransactionsStalled` - submitted transactions parked behind a missing nonce of their own sender, which cannot be emitted until that gap is filled
* `TransactionsEmitted` - transactions carried by an event but not yet by a block
* `TransactionsIncluded` - transactions that reached a block, accumulated over the run
* `TransactionsRejected` - transactions a node refused to accept, accumulated over the run

The first three are disjoint and add up to the transactions of an application currently in the system. Together they match what the nodes report as their pool contents (`txpool_pending`, `txpool_queued`), broken down per application.

Measured per transaction, in nanoseconds, positioned at the moment of the submission:
* `TransactionTimeToEmit` - from the submission until a validator emitted it
* `TransactionTimeEmitToInclude` - from the emission until it reached a block
* `TransactionTimeToInclude` - from the submission until it reached a block, the latency a user of the network observes

Recorded per block, per application:
* `BlockTransactionsPerApp` - the number of transactions of an application in a block
* `BlockGasPerApp` - the gas those transactions used in that block

Both are attributed to `(other)` for the transactions of no load generator, i.e. the ones Norma sends itself, so the values of a block add up to that block's own `BlockNumberOfTransactions` and `BlockGasUsed`. The report stacks them to show how the applications shared each block.

Recorded per block, for the network:
* `BlockGasLimit` - the gas limit the block was formed under, read from its header

This is the `Blocks.MaxBlockGas` rule of the network, a technical hard limit rather than the throughput a network is expected to reach: how much gas a block really carries is governed by the gas validators may spend per second (`Economy.ShortGasPower.AllocPerSec`) and by the gas one event may hold (`Economy.Gas.MaxEventGas`). A run of the priority lanes pilot fills a quarter of a percent of it. The report plots it next to the gas of the applications, on a logarithmic axis, so the headroom is visible.

There is one measurement per transaction, far too many to read one by one: the rendered report summarises them as distributions per application and over time. For long runs the retained measurements are thinned out, evenly over the run, so the number of rows of a duration metric is not the number of transactions sent - use `TransactionsIncluded` for that.

Note that the older `ReceivedTransactions` metric measures something different: it reads the state of an application's contract, i.e. the effect its transactions had, and therefore counts neither reverted nor pending ones.

## CPU Profile Data

Norma also collects a `pprof` CPU profile from every node every 10 seconds. The
profiles are stored in the run's output directory:
```
norma_data_<label>_<n>
└── cpu_profiles
    └── <node-name>
        ├── <sample_number>.prof
        └── ...
```
To analyse one:
```
go tool pprof -http=":8000" <sample_number>.prof
```
