// SPDX-License-Identifier: MIT

// @notice ClzCounter exercises the CLZ opcode (EIP-7939, opcode 0x1e).
// CLZ counts the number of leading zero bits in a 256-bit word (result 0–256).
// incrementCounter asserts that clz(value) == expectedClz, then increments the
// internal counter.
//
// Pure Yul is required because solc does not yet expose CLZ as a named
// Yul builtin; the opcode is emitted via verbatim_1i_1o.
//
// ABI (selectors computed via keccak256):
//   incrementCounter(uint256 value, uint256 expectedClz)  0xf41a27b9
//   getCount() returns (int256)                           0xa87d942c

object "ClzCounter" {
    code {
        // Initialise count slot to 1 so every incrementCounter call pays the
        // same gas (no cold 0→1 SSTORE on the first call).
        sstore(0, 1)
        datacopy(0, dataoffset("runtime"), datasize("runtime"))
        return(0, datasize("runtime"))
    }

    object "runtime" {
        code {
            // Require at least 4 bytes for the selector.
            if lt(calldatasize(), 4) { revert(0, 0) }

            switch shr(224, calldataload(0))

            // incrementCounter(uint256 value, uint256 expectedClz) → 0xf41a27b9
            case 0xf41a27b9 {
                if lt(calldatasize(), 68) { revert(0, 0) }
                let value       := calldataload(4)
                let expectedClz := calldataload(36)

                // CLZ opcode (EIP-7939, opcode 0x1e): count leading zero bits.
                let result := verbatim_1i_1o(hex"1e", value)

                if iszero(eq(result, expectedClz)) {
                    // revert with Error("CLZ result mismatch")
                    // ABI encoding: selector(4) + offset(32) + length(32) + data(32) = 100 bytes
                    mstore(0x00, 0x08c379a000000000000000000000000000000000000000000000000000000000)
                    mstore(0x04, 0x0000000000000000000000000000000000000000000000000000000000000020)
                    mstore(0x24, 0x0000000000000000000000000000000000000000000000000000000000000013)
                    mstore(0x44, 0x434c5a20726573756c74206d69736d6174636800000000000000000000000000)
                    revert(0x00, 0x64)
                }
                sstore(0, add(sload(0), 1))
                stop()
            }

            // getCount() → int256  0xa87d942c
            case 0xa87d942c {
                // Returns count - 1 (internal counter is 1-indexed).
                mstore(0, sub(sload(0), 1))
                return(0, 32)
            }

            default { revert(0, 0) }
        }
    }
}
