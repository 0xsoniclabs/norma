#!/usr/bin/env bash
# Compiles a Solidity contract to EOF format using solc standard-JSON.
#
# Usage:
#   compile_eof.sh <input.sol> <output.abi> <output.bin>
#
# Requirements: solc >=0.8.29, jq
set -euo pipefail

if [[ $# -ne 3 ]]; then
    echo "Usage: $0 <input.sol> <output.abi> <output.bin>" >&2
    exit 1
fi

SOL="$1"
OUT_ABI="$2"
OUT_BIN="$3"

if [[ ! -f "$SOL" ]]; then
    echo "Error: $SOL not found" >&2
    exit 1
fi

SOLC_OUTPUT=$(jq -n \
    --arg name "$(basename "$SOL")" \
    --arg content "$(cat "$SOL")" \
    '{
        language: "Solidity",
        sources: { ($name): { content: $content } },
        settings: {
            eofVersion: 1,
            viaIR: true,
            optimizer: { enabled: true, runs: 200 },
            outputSelection: { "*": { "*": ["abi", "evm.bytecode"] } }
        }
    }' | solc --standard-json)

# Print errors and exit if any.
if jq -e '[.errors[]? | select(.severity == "error")] | length > 0' <<< "$SOLC_OUTPUT" > /dev/null; then
    jq -r '.errors[] | select(.severity == "error") | .formattedMessage' <<< "$SOLC_OUTPUT" >&2
    exit 1
fi

FIRST_CONTRACT='.contracts | to_entries[0].value | to_entries[0].value'

jq "$FIRST_CONTRACT | .abi" <<< "$SOLC_OUTPUT" > "$OUT_ABI"

BYTECODE=$(jq -r "$FIRST_CONTRACT | .evm.bytecode.object" <<< "$SOLC_OUTPUT")
if [[ "${BYTECODE:0:4}" != "ef00" ]]; then
    echo "Warning: bytecode does not start with EOF magic (got ${BYTECODE:0:6})" >&2
fi
echo -n "$BYTECODE" > "$OUT_BIN"
