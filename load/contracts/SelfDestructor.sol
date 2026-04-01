// SPDX-License-Identifier: MIT
pragma solidity ^0.8.4;

/// @notice SelfDestructorFactory allows self-destruct testing by deploying and self-destructing a contract.
/// Contracts are not actually destroyed on Cancun, but self-destructing should still transfer the balance.
contract SelfDestructorFactory {
    address public constructedContract;

    function deployOrDestruct() public payable {
        if (msg.value != 1 wei) {
            revert("Expected 1 wei paid");
        }
        if (constructedContract == address(0)) {
            SelfDestructor newContract = new SelfDestructor{value: msg.value}();
            constructedContract = address(newContract);
        } else {
            SelfDestructor(constructedContract).destroy();
            constructedContract = address(0);
        }
    }

    function getCount() public view returns (uint256) {
        uint256 count = address(this).balance;
        if (constructedContract != address(0)) {
            count += 1;
        }
        return count;
    }
}

contract SelfDestructor {
    constructor() payable {}

    function destroy() public {
        selfdestruct(payable(msg.sender));
    }
}
