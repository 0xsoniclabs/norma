package network

import "math/big"

// systemTxGasTipCap is a high priority fee cap for system transactions
// (UpdateNetworkRules, AdvanceEpochs). It must be larger than the tip used by
// load-generator transactions so the node emitter always processes system
// transactions before user traffic.
var systemTxGasTipCap = big.NewInt(1_000_000)

// systemTxGasLimit is a fixed gas limit for system transactions.
// It is set explicitly so that go-ethereum's bind layer skips eth_estimateGas.
// Estimation fails when a previous rule update has lowered MaxEventGas (or
// similar fields) to a very small value, because the node uses that rule as the
// cap when servicing eth_estimateGas calls, returning "gas required exceeds
// allowance (0)". A fixed limit bypasses estimation entirely; the actual
// on-chain execution of DriverAuth calls consumes far less than this limit.
const systemTxGasLimit uint64 = 1_000_000
