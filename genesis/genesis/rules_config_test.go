package genesis

import (
	"fmt"
	"github.com/0xsoniclabs/sonic/opera"
	"os"
	"testing"
)

func TestConfigureNetworkRules_Values_Set(t *testing.T) {
	defaultRules := opera.MainNetRules()

	tests := []struct {
		key   string
		value string
		match func(rules opera.Rules) (string, bool)
	}{
		{
			key:   "MAX_BLOCK_GAS",
			value: "2000000",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Blocks.MaxBlockGas), rules.Blocks.MaxBlockGas == 2000000
			},
		},
		{
			key:   "MAX_EMPTY_BLOCK_SKIP_PERIOD",
			value: "16ms",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Blocks.MaxEmptyBlockSkipPeriod), rules.Blocks.MaxEmptyBlockSkipPeriod == 16e6
			},
		},
		{
			key:   "MAX_EPOCH_GAS",
			value: "30000000",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Epochs.MaxEpochGas), rules.Epochs.MaxEpochGas == 30000000
			},
		},
		{
			key:   "MAX_EPOCH_DURATION",
			value: "11s",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Epochs.MaxEpochDuration), rules.Epochs.MaxEpochDuration == 11e9
			},
		},
		{
			key:   "EMITTER_INTERVAL",
			value: "5s",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Emitter.Interval), rules.Emitter.Interval == 5e9
			},
		},
		{
			key:   "EMITTER_STALL_THRESHOLD",
			value: "2h",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Emitter.StallThreshold), rules.Emitter.StallThreshold == 2*60*60e9
			},
		},
		{
			key:   "EMITTER_STALLED_INTERVAL",
			value: "14s",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Emitter.StalledInterval), rules.Emitter.StalledInterval == 14e9
			},
		},
		{
			key:   "UPGRADES_BERLIN",
			value: "true",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%t", rules.Upgrades.Berlin), rules.Upgrades.Berlin == true
			},
		},
		{
			key:   "UPGRADES_LONDON",
			value: "true",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%t", rules.Upgrades.London), rules.Upgrades.London == true
			},
		},
		{
			key:   "UPGRADES_LLR",
			value: "true",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%t", rules.Upgrades.Llr), rules.Upgrades.Llr == true
			},
		},
		{
			key:   "UPGRADES_SONIC",
			value: "true",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%t", rules.Upgrades.Sonic), rules.Upgrades.Sonic == true
			},
		},
		{
			key:   "MIN_GAS_PRICE",
			value: "1000000001",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.MinGasPrice), rules.Economy.MinGasPrice.Uint64() == 1000000001
			},
		},
		{
			key:   "MIN_BASE_FEE",
			value: "1000000002",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.MinBaseFee), rules.Economy.MinBaseFee.Uint64() == 1000000002
			},
		},
		{
			key:   "BLOCK_MISSED_SLACK",
			value: "3",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.BlockMissedSlack), rules.Economy.BlockMissedSlack == 3
			},
		},
		{
			key:   "MAX_EVENT_GAS",
			value: "1000015",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.Gas.MaxEventGas), rules.Economy.Gas.MaxEventGas == 1000015
			},
		},
		{
			key:   "EVENT_GAS",
			value: "1000016",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.Gas.EventGas), rules.Economy.Gas.EventGas == 1000016
			},
		},
		{
			key:   "PARENT_GAS",
			value: "1000017",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.Gas.ParentGas), rules.Economy.Gas.ParentGas == 1000017
			},
		},
		{
			key:   "EXTRA_DATA_GAS",
			value: "1000018",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.Gas.ExtraDataGas), rules.Economy.Gas.ExtraDataGas == 1000018
			},
		},
		{
			key:   "BLOCK_VOTES_BASE_GAS",
			value: "1000019",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.Gas.BlockVotesBaseGas), rules.Economy.Gas.BlockVotesBaseGas == 1000019
			},
		},
		{
			key:   "BLOCK_VOTE_GAS",
			value: "1000020",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.Gas.BlockVoteGas), rules.Economy.Gas.BlockVoteGas == 1000020
			},
		},
		{
			key:   "EPOCH_VOTE_GAS",
			value: "1000021",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.Gas.EpochVoteGas), rules.Economy.Gas.EpochVoteGas == 1000021
			},
		},
		{
			key:   "MISBEHAVIOUR_PROOF_GAS",
			value: "1000022",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.Gas.MisbehaviourProofGas), rules.Economy.Gas.MisbehaviourProofGas == 1000022
			},
		},
		{
			key:   "SHORT_ALLOC_PER_SEC",
			value: "1000023",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.ShortGasPower.AllocPerSec), rules.Economy.ShortGasPower.AllocPerSec == 1000023
			},
		},
		{
			key:   "SHORT_MAX_ALLOC_PERIOD",
			value: "5s",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.ShortGasPower.MaxAllocPeriod), rules.Economy.ShortGasPower.MaxAllocPeriod == 5e9
			},
		},
		{
			key:   "SHORT_STARTUP_ALLOC_PERIOD",
			value: "6s",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.ShortGasPower.StartupAllocPeriod), rules.Economy.ShortGasPower.StartupAllocPeriod == 6e9
			},
		},
		{
			key:   "SHORT_MIN_STARTUP_GAS",
			value: "1000026",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.ShortGasPower.MinStartupGas), rules.Economy.ShortGasPower.MinStartupGas == 1000026
			},
		},
		{
			key:   "LONG_ALLOC_PER_SEC",
			value: "1000027",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.LongGasPower.AllocPerSec), rules.Economy.LongGasPower.AllocPerSec == 1000027
			},
		},
		{
			key:   "LONG_MAX_ALLOC_PERIOD",
			value: "51ms",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.LongGasPower.MaxAllocPeriod), rules.Economy.LongGasPower.MaxAllocPeriod == 51e6
			},
		},
		{
			key:   "LONG_STARTUP_ALLOC_PERIOD",
			value: "52ns",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.LongGasPower.StartupAllocPeriod), rules.Economy.LongGasPower.StartupAllocPeriod == 52
			},
		},
		{
			key:   "LONG_MIN_STARTUP_GAS",
			value: "1000030",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Economy.LongGasPower.MinStartupGas), rules.Economy.LongGasPower.MinStartupGas == 1000030
			},
		},
		{
			key:   "MAX_PARENTS",
			value: "1000031",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Dag.MaxParents), rules.Dag.MaxParents == 1000031
			},
		},
		{
			key:   "MAX_FREE_PARENTS",
			value: "1000032",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Dag.MaxFreeParents), rules.Dag.MaxFreeParents == 1000032
			},
		},
		{
			key:   "MAX_EXTRA_DATA",
			value: "1000033",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Dag.MaxExtraData), rules.Dag.MaxExtraData == 1000033
			},
		},
		{
			key:   "MAX_BLOCK_GAS - default",
			value: "",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Blocks.MaxBlockGas), rules.Blocks.MaxBlockGas == defaultRules.Blocks.MaxBlockGas
			},
		},
		{
			key:   "MAX_EPOCH_GAS - default",
			value: "",
			match: func(rules opera.Rules) (string, bool) {
				return fmt.Sprintf("%d", rules.Epochs.MaxEpochGas), rules.Epochs.MaxEpochGas == defaultRules.Epochs.MaxEpochGas
			},
		},
	}

	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			if err := os.Setenv(test.key, test.value); err != nil {
				t.Fatalf("failed to set %s: %v", test.key, err)
			}
			defer func() {
				if err := os.Unsetenv(test.key); err != nil {
					t.Fatalf("failed to unset %s: %v", test.key, err)
				}
			}()

			// Create a new Rules object
			rules := opera.MainNetRules()

			// Call the ConfigureNetworkRules function
			if err := ConfigureNetworkRules(&rules); err != nil {
				t.Fatalf("failed to configure network rules: %v", err)
			}

			// Verify the rules were set correctly
			if value, ok := test.match(rules); !ok {
				t.Errorf("unexpected value for %s: got: %s != wanted: %s", test.key, value, test.value)
			}
		})
	}
}

func TestConfigureNetworkRules_Values_CannotParse(t *testing.T) {
	tests := []struct {
		key string
	}{
		{
			key: "MAX_BLOCK_GAS",
		},
		{
			key: "MAX_EPOCH_GAS",
		},
		{
			key: "MAX_EMPTY_BLOCK_SKIP_PERIOD",
		},
		{
			key: "MAX_EPOCH_DURATION",
		},
		{
			key: "EMITTER_INTERVAL",
		},
		{
			key: "EMITTER_STALL_THRESHOLD",
		},
		{
			key: "EMITTER_STALLED_INTERVAL",
		},
		{
			key: "MIN_GAS_PRICE",
		},
		{
			key: "MIN_BASE_FEE",
		},
		{
			key: "BLOCK_MISSED_SLACK",
		},
		{
			key: "MAX_EVENT_GAS",
		},
		{
			key: "EVENT_GAS",
		},
		{
			key: "PARENT_GAS",
		},
		{
			key: "EXTRA_DATA_GAS",
		},
		{
			key: "BLOCK_VOTES_BASE_GAS",
		},
		{
			key: "BLOCK_VOTE_GAS",
		},
		{
			key: "EPOCH_VOTE_GAS",
		},
		{
			key: "MISBEHAVIOUR_PROOF_GAS",
		},
		{
			key: "SHORT_ALLOC_PER_SEC",
		},
		{
			key: "SHORT_MAX_ALLOC_PERIOD",
		},
		{
			key: "SHORT_STARTUP_ALLOC_PERIOD",
		},
		{
			key: "SHORT_MIN_STARTUP_GAS",
		},
		{
			key: "LONG_ALLOC_PER_SEC",
		},
		{
			key: "LONG_MAX_ALLOC_PERIOD",
		},
		{
			key: "LONG_STARTUP_ALLOC_PERIOD",
		},
		{
			key: "LONG_MIN_STARTUP_GAS",
		},
		{
			key: "MAX_PARENTS",
		},
		{
			key: "MAX_FREE_PARENTS",
		},
		{
			key: "MAX_EXTRA_DATA",
		},
	}

	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			if err := os.Setenv(test.key, "xxx"); err != nil {
				t.Fatalf("failed to set %s: %v", test.key, err)
			}
			defer func() {
				if err := os.Unsetenv(test.key); err != nil {
					t.Fatalf("failed to unset %s: %v", test.key, err)
				}
			}()

			// Create a new Rules object
			rules := opera.MainNetRules()

			// Call the ConfigureNetworkRules function
			if err := ConfigureNetworkRules(&rules); err == nil {
				t.Errorf("expected an error, got nil")
			}
		})
	}
}
