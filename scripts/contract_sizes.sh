#!/bin/bash
# Reports init code and runtime bytecode sizes for named contracts in a Solidity file.
#
# Usage: ./scripts/contract_sizes.sh <Contract.sol> <ContractName> [ContractName...]
#
# Example:
#   ./scripts/contract_sizes.sh load/contracts/LargeContract.sol LargeContract LargeContractCounter

set -euo pipefail

SOL="${1:?Usage: $0 <Contract.sol> <ContractName> [ContractName...]}"
shift

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

solc --evm-version london -o "$TMPDIR" --overwrite --bin --bin-runtime "$SOL" 2>/dev/null

for NAME in "$@"; do
    INIT=$(wc -c < "$TMPDIR/$NAME.bin")
    RT=$(wc -c < "$TMPDIR/$NAME.bin-runtime")
    echo "$NAME: init=$(( INIT / 2 ))b  runtime=$(( RT / 2 ))b"
done
