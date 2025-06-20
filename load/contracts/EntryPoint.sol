// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.17;

/// @notice Account-Abstraction (EIP-4337) singleton EntryPoint simplification
contract EntryPoint {

    /// @notice User Operation struct
    /// @param sender The smart account.
    /// @param callData The method call to execute on this account.
    struct PackedUserOperation {
        address sender;
        bytes callData;
    }

    function handleOps(PackedUserOperation[] calldata ops) external {
        uint256 opslen = ops.length;
        for (uint256 i = 0; i < opslen; i++) {
            bytes calldata callData = ops[i].callData;
            (bool _success,) = address(ops[i].sender).call(callData);
            require(_success, "SmartAccount call failed");
        }
    }

}
