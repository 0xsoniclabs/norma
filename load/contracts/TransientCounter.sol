// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @title Minimal Transient Storage Demo (requires Cancun)
contract TransientCounter {
    // nonReentrant is a modifier that prevents multiple call to the same function in the same transaction
    modifier nonReentrant() {
        bool locked;
        assembly { locked := tload(0x1234) }
        if (locked) return;
        assembly { tstore(0x1234, 1) }
        _;
        // No reset - clears itself after the transaction
    }

    // counter is an internal counter tracking the number of increment calls.
    // The counter is initialized to 1 to make all increment-counter calls
    // equally expensive. Otherwise, the first call incrementing the counter
    // from 0 to 1 would have to pay extra gas for the storage allocation.
    int private count = 1;

    function incrementCounter() external nonReentrant {
        count += 1;
    }

    function incrementCounterTwice() external {
        this.incrementCounter(); // should increment
        this.incrementCounter(); // should not increment, because of nonReentrant
    }

    function getCount() external view returns (int) {
        return count-1;
    }

}
