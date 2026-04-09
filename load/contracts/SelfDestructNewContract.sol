// SPDX-License-Identifier: MIT
pragma solidity ^0.8.4;

/// @notice SelfDestructNewContractFactory allows self-destruct testing by deploying and self-destructing a contract.
/// Contracts are actually destroyed even on Cancun, when created in the same transaction.
contract SelfDestructNewContractFactory {
    function deployAndDestruct() public payable {
        if (msg.value != 1 wei) {
            revert("Expected 1 wei paid");
        }
        SelfDestructNewContract newContract = new SelfDestructNewContract{value: msg.value}();
        newContract.destroy(); // expected to transfer the 1 wei back to this contract
    }

    function getCount() public view returns (uint256) {
        return address(this).balance;
    }
}

contract SelfDestructNewContract {
    constructor() payable {}

    function destroy() public {
        selfdestruct(payable(msg.sender));
    }
}
