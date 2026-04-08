// SPDX-License-Identifier: MIT
pragma solidity ^0.8.29;

// @notice EcdsaCounter exercises the P256Verify precompile (EIP-7951, address 0x100).
// The P-256 public key is fixed at deployment as immutables.
// incrementCounter verifies a secp256r1 signature over the supplied hash and,
// if valid, increments the internal counter.
contract EcdsaCounter {
    // P256Verify precompile address (EIP-7951).
    address constant private P256VERIFY = address(0x0000000000000000000000000000000000000100);

    // P-256 public key set once at deployment and stored as immutables (no SLOAD on use).
    bytes32 private immutable publicKeyX;
    bytes32 private immutable publicKeyY;

    // counter is initialised to 1 so every increment costs the same gas
    // (no cold storage write from 0→1 on the first call).
    int private count = 1;

    constructor(bytes32 pubKeyX, bytes32 pubKeyY) {
        publicKeyX = pubKeyX;
        publicKeyY = pubKeyY;
    }

    // incrementCounter verifies a secp256r1 signature over hash, then increments
    // the internal counter.
    //
    // Parameters:
    //   hash - 32-byte message digest that was signed.
    //   r, s - P-256 signature components.
    function incrementCounter(bytes32 hash, bytes32 r, bytes32 s) public {
        (bool ok, bytes memory ret) = P256VERIFY.staticcall(
            abi.encodePacked(hash, r, s, publicKeyX, publicKeyY)
        );
        require(ok && ret.length == 32 && uint256(bytes32(ret)) == 1, "invalid P-256 signature");
        count += 1;
    }

    function getCount() public view returns (int) {
        return count - 1;
    }
}
