// SPDX-License-Identifier: MIT
pragma solidity ^0.8.4;

/// @notice SelfDestructorFactory allows self-destruct testing by deploying and self-destructing a contract.
/// Contracts are not actually destroyed on Cancun, but self-destructing should still transfer the balance.
contract SelfDestructorFactory {
    address public constructedContract;

    constructor() payable {
        SelfDestructor newContract = new SelfDestructor{value: msg.value}();
        constructedContract = address(newContract);
    }

    function destructAndDeploy() public payable {
        if (msg.value != 1 wei) {
            revert("Expected 1 wei paid");
        }
        // destroy old contract (obtain its balance)
        SelfDestructor(constructedContract).destroy();

        // deploy new contract
        SelfDestructor newContract = new SelfDestructor{value: msg.value}();
        constructedContract = address(newContract);
    }

    function getCount() public view returns (uint256) {
        return address(this).balance;
    }
}

contract SelfDestructor {
    constructor() payable {}

    function destroy() public {
        selfdestruct(payable(msg.sender));
    }
}
