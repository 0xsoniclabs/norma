package network

import "math/big"

// systemTxGasTipCap is a high priority fee cap for system transactions
// (UpdateNetworkRules, AdvanceEpochs). It must be larger than the tip used by
// load-generator transactions so the node emitter always processes system
// transactions before user traffic.
//
// Load transactions are priced at twice the node's gas price suggestion
// (load/app.appContext.GetTransactOptions), which leaves them an effective tip
// of roughly one base fee. At the 1 Gwei minimum base fee that is 1e9 wei, so
// the 1e6 wei this used to be lost to user traffic by three orders of
// magnitude and system transactions could sit in the pool for minutes
// whenever a scenario changed rules while load was running.
//
// ponytail: fixed cap with ~100x headroom over the highest base fee seen in
// practice; derive it from SuggestGasPrice if a scenario ever drives the base
// fee near 1000 Gwei.
var systemTxGasTipCap = big.NewInt(1e12)

// systemTxGasLimit is a fixed gas limit for system transactions.
// It is set explicitly so that go-ethereum's bind layer skips eth_estimateGas.
// Estimation fails when a previous rule update has lowered MaxEventGas (or
// similar fields) to a very small value, because the node uses that rule as the
// cap when servicing eth_estimateGas calls, returning "gas required exceeds
// allowance (0)". A fixed limit bypasses estimation entirely; the actual
// on-chain execution of DriverAuth calls consumes far less than this limit.
const systemTxGasLimit uint64 = 1_000_000
