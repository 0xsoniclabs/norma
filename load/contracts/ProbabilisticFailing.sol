// SPDX-License-Identifier: MIT
pragma solidity ^0.8.4;

contract ProbabilisticFailing {
    // counter is an internal counter tracking the number of increment calls.
    // The counter is initialized to 1 to make all increment-counter calls
    // equally expensive. Otherwise, the first call incrementing the counter
    // from 0 to 1 would have to pay extra gas for the storage allocation.
    int private count = 1;

    function incrementCounter(uint8 failureProbability) public {
        // Reverts in dependency on the previous history, the sender and the block timestamp.
        // Whether this reverts or not should not be reliably statically predictable.
        uint256 rand = uint256(keccak256(abi.encodePacked(msg.sender, count, block.timestamp)));
        if (rand % 100 < failureProbability) {
            revert("Probabilistic revert");
        }
        count += 1;
    }

    function getCount() public view returns (int) {
        return count-1;
    }
}
