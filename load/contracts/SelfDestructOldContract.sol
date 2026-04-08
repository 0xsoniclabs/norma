// SPDX-License-Identifier: MIT
pragma solidity ^0.8.4;

/// @notice SelfDestructOldContractFactory allows self-destruct testing by deploying and self-destructing a contract.
/// Already existing contracts are not actually destroyed on Cancun, but self-destructing should still transfer the balance.
contract SelfDestructOldContractFactory {
    address public constructedContract;

    constructor() payable {
        SelfDestructOldContract newContract = new SelfDestructOldContract{value: msg.value}();
        constructedContract = address(newContract);
    }

    function destructAndDeploy() public payable {
        if (msg.value != 1 wei) {
            revert("Expected 1 wei paid");
        }
        // destroy old contract (obtain its balance)
        SelfDestructOldContract(constructedContract).destroy();

        // deploy new contract
        SelfDestructOldContract newContract = new SelfDestructOldContract{value: msg.value}();
        constructedContract = address(newContract);
    }

    function getCount() public view returns (uint256) {
        return address(this).balance;
    }
}

contract SelfDestructOldContract {
    constructor() payable {}

    function destroy() public {
        selfdestruct(payable(msg.sender));
    }
}
