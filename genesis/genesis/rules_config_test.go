package genesis

import (
	"encoding/json"
	"fmt"
	"github.com/0xsoniclabs/sonic/opera"
	"maps"
	"os"
	"testing"
)

func TestIsSupportedNetworkRule(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		{"MAX_BLOCK_GAS", true},
		{"MAX_EPOCH_GAS", true},
		{"INVALID_RULE", false},
	}

	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			result := IsSupportedNetworkRule(test.key)
			if result != test.expected {
				t.Errorf("IsSupportedNetworkRule(%s) = %v; want %v", test.key, result, test.expected)
			}
		})
	}
}

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

	t.Run("env", func(t *testing.T) {
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

				// Call the ConfigureNetworkRulesEnv function
				if err := ConfigureNetworkRulesEnv(&rules); err != nil {
					t.Fatalf("failed to configure network rules: %v", err)
				}

				// Verify the rules were set correctly
				if value, ok := test.match(rules); !ok {
					t.Errorf("unexpected value for %s: got: %s != wanted: %s", test.key, value, test.value)
				}
			})
		}
	})

	t.Run("env", func(t *testing.T) {
		for _, test := range tests {
			t.Run(test.key, func(t *testing.T) {
				updates := make(NetworkRules)
				updates[test.key] = test.value

				// Create a new Rules object
				rules := opera.MainNetRules()

				// Call the ConfigureNetworkRulesEnv function
				if err := ConfigureNetworkRulesMap(&rules, updates); err != nil {
					t.Fatalf("failed to configure network rules: %v", err)
				}

				// Verify the rules were set correctly
				if value, ok := test.match(rules); !ok {
					t.Errorf("unexpected value for %s: got: %s != wanted: %s", test.key, value, test.value)
				}
			})
		}
	})
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

	t.Run("env", func(t *testing.T) {
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

				// Call the ConfigureNetworkRulesEnv function
				if err := ConfigureNetworkRulesEnv(&rules); err == nil {
					t.Errorf("expected an error, got nil")
				}
			})
		}
	})

	t.Run("update map", func(t *testing.T) {
		for _, test := range tests {
			t.Run(test.key, func(t *testing.T) {
				updates := make(NetworkRules)
				updates[test.key] = "xxx"

				// Create a new Rules object
				rules := opera.MainNetRules()

				// Call the ConfigureNetworkRulesEnv function
				if err := ConfigureNetworkRulesMap(&rules, updates); err == nil {
					t.Errorf("expected an error, got nil")
				}
			})
		}

	})
}

func TestGenerateJsonNetworkRulesUpdates_Exported_Json_Correct(t *testing.T) {
	tests := []struct {
		key   string
		value string
		json  map[string]any
	}{
		{
			key:   "MAX_BLOCK_GAS",
			value: "2000000",
			json: map[string]any{
				"Blocks": map[string]any{
					"MaxBlockGas": 2000000,
				},
			},
		},
		{
			key:   "MAX_EMPTY_BLOCK_SKIP_PERIOD",
			value: "16ms",
			json: map[string]any{
				"Blocks": map[string]any{
					"MaxEmptyBlockSkipPeriod": 16e6,
				},
			},
		},
		{
			key:   "MAX_EPOCH_GAS",
			value: "30000000",
			json: map[string]any{
				"Epochs": map[string]any{
					"MaxEpochGas": 30000000,
				},
			},
		},
		{
			key:   "MAX_EPOCH_DURATION",
			value: "11s",
			json: map[string]any{
				"Epochs": map[string]any{
					"MaxEpochDuration": 11e9,
				},
			},
		},
		{
			key:   "EMITTER_INTERVAL",
			value: "5s",
			json: map[string]any{
				"Emitter": map[string]any{
					"Interval": 5e9,
				},
			},
		},
		{
			key:   "EMITTER_STALL_THRESHOLD",
			value: "2h",
			json: map[string]any{
				"Emitter": map[string]any{
					"StallThreshold": 2 * 60 * 60e9,
				},
			},
		},
		{
			key:   "EMITTER_STALLED_INTERVAL",
			value: "14s",
			json: map[string]any{
				"Emitter": map[string]any{
					"StalledInterval": 14e9,
				},
			},
		},
		{
			key:   "UPGRADES_BERLIN",
			value: "true",
			json: map[string]any{
				"Upgrades": map[string]any{
					"Berlin": true,
				},
			},
		},
		{
			key:   "UPGRADES_LONDON",
			value: "true",
			json: map[string]any{
				"Upgrades": map[string]any{
					"London": true,
				},
			},
		},
		{
			key:   "UPGRADES_LLR",
			value: "true",
			json: map[string]any{
				"Upgrades": map[string]any{
					"Llr": true,
				},
			},
		},
		{
			key:   "UPGRADES_SONIC",
			value: "true",
			json: map[string]any{
				"Upgrades": map[string]any{
					"Sonic": true,
				},
			},
		},
		{
			key:   "MIN_GAS_PRICE",
			value: "1000000001",
			json: map[string]any{
				"Economy": map[string]any{
					"MinGasPrice": 1000000001,
				},
			},
		},
		{
			key:   "MIN_BASE_FEE",
			value: "1000000002",
			json: map[string]any{
				"Economy": map[string]any{
					"MinBaseFee": 1000000002,
				},
			},
		},
		{
			key:   "BLOCK_MISSED_SLACK",
			value: "3",
			json: map[string]any{
				"Economy": map[string]any{
					"BlockMissedSlack": 3,
				},
			},
		},
		{
			key:   "MAX_EVENT_GAS",
			value: "1000015",
			json: map[string]any{
				"Economy": map[string]any{
					"Gas": map[string]any{
						"MaxEventGas": 1000015,
					},
				},
			},
		},
		{
			key:   "EVENT_GAS",
			value: "1000016",
			json: map[string]any{
				"Economy": map[string]any{
					"Gas": map[string]any{
						"EventGas": 1000016,
					},
				},
			},
		},
		{
			key:   "PARENT_GAS",
			value: "1000017",
			json: map[string]any{
				"Economy": map[string]any{
					"Gas": map[string]any{
						"ParentGas": 1000017,
					},
				},
			},
		},
		{
			key:   "EXTRA_DATA_GAS",
			value: "1000018",
			json: map[string]any{
				"Economy": map[string]any{
					"Gas": map[string]any{
						"ExtraDataGas": 1000018,
					},
				},
			},
		},
		{
			key:   "BLOCK_VOTES_BASE_GAS",
			value: "1000019",
			json: map[string]any{
				"Economy": map[string]any{
					"Gas": map[string]any{
						"BlockVotesBaseGas": 1000019,
					},
				},
			},
		},
		{
			key:   "BLOCK_VOTE_GAS",
			value: "1000020",
			json: map[string]any{
				"Economy": map[string]any{
					"Gas": map[string]any{
						"BlockVoteGas": 1000020,
					},
				},
			},
		},
		{
			key:   "EPOCH_VOTE_GAS",
			value: "1000021",
			json: map[string]any{
				"Economy": map[string]any{
					"Gas": map[string]any{
						"EpochVoteGas": 1000021,
					},
				},
			},
		},
		{
			key:   "MISBEHAVIOUR_PROOF_GAS",
			value: "1000022",
			json: map[string]any{
				"Economy": map[string]any{
					"Gas": map[string]any{
						"MisbehaviourProofGas": 1000022,
					},
				},
			},
		},
		{
			key:   "SHORT_ALLOC_PER_SEC",
			value: "1000023",
			json: map[string]any{
				"Economy": map[string]any{
					"ShortGasPower": map[string]any{
						"AllocPerSec": 1000023,
					},
				},
			},
		},
		{
			key:   "SHORT_MAX_ALLOC_PERIOD",
			value: "6s",
			json: map[string]any{
				"Economy": map[string]any{
					"ShortGasPower": map[string]any{
						"MaxAllocPeriod": 6e9,
					},
				},
			},
		},
		{
			key:   "SHORT_STARTUP_ALLOC_PERIOD",
			value: "6s",
			json: map[string]any{
				"Economy": map[string]any{
					"ShortGasPower": map[string]any{
						"StartupAllocPeriod": 6e9,
					},
				},
			},
		},
		{
			key:   "SHORT_MIN_STARTUP_GAS",
			value: "1000026",
			json: map[string]any{
				"Economy": map[string]any{
					"ShortGasPower": map[string]any{
						"MinStartupGas": 1000026,
					},
				},
			},
		},
		{
			key:   "LONG_ALLOC_PER_SEC",
			value: "1000027",
			json: map[string]any{
				"Economy": map[string]any{
					"LongGasPower": map[string]any{
						"AllocPerSec": 1000027,
					},
				},
			},
		},
		{
			key:   "LONG_MAX_ALLOC_PERIOD",
			value: "51ms",
			json: map[string]any{
				"Economy": map[string]any{
					"LongGasPower": map[string]any{
						"MaxAllocPeriod": 51e6,
					},
				},
			},
		},
		{
			key:   "LONG_STARTUP_ALLOC_PERIOD",
			value: "52ns",
			json: map[string]any{
				"Economy": map[string]any{
					"LongGasPower": map[string]any{
						"StartupAllocPeriod": 52,
					},
				},
			},
		},
		{
			key:   "LONG_MIN_STARTUP_GAS",
			value: "1000030",
			json: map[string]any{
				"Economy": map[string]any{
					"LongGasPower": map[string]any{
						"MinStartupGas": 1000030,
					},
				},
			},
		},
		{
			key:   "MAX_PARENTS",
			value: "1000031",
			json: map[string]any{
				"Dag": map[string]any{
					"MaxParents": 1000031,
				},
			},
		},
		{
			key:   "MAX_FREE_PARENTS",
			value: "1000032",
			json: map[string]any{
				"Dag": map[string]any{
					"MaxFreeParents": 1000032,
				},
			},
		},
		{
			key:   "MAX_EXTRA_DATA",
			value: "1000033",
			json: map[string]any{
				"Dag": map[string]any{
					"MaxExtraData": 1000033,
				},
			},
		},
	}

	t.Run("single", func(t *testing.T) {
		for _, test := range tests {
			t.Run(test.key, func(t *testing.T) {
				// Create a new Rules object
				rules := opera.MainNetRules()
				rules.Upgrades = opera.Upgrades{}

				updates := make(NetworkRules)
				updates[test.key] = test.value
				gotJson, err := GenerateJsonNetworkRulesUpdates(rules, updates)
				if err != nil {
					t.Fatalf("failed to generate json: %v", err)
				}

				b, err := json.Marshal(test.json)
				if err != nil {
					t.Fatalf("failed to marshal json: %v", err)
				}

				if got, want := gotJson, string(b); got != want {
					t.Errorf("unexpected json: got: %s != want: %s", got, want)
				}
			})
		}
	})

	t.Run("multiple", func(t *testing.T) {
		// collect changes from all tests and merge them all to one update set
		updates := make(NetworkRules)
		expected := make(map[string]any)
		for _, test := range tests {
			updates[test.key] = test.value
			expected = mergeMapsSimpleValuesOrNestedMap(expected, test.json)
		}

		// convert back and fort to unify the datatypes
		jsonData, err := json.Marshal(expected)
		if err != nil {
			t.Fatalf("failed to marshal json: %v", err)
		}
		expected = make(map[string]any)
		if err = json.Unmarshal(jsonData, &expected); err != nil {
			t.Fatalf("failed to unmarshal json: %v", err)
		}

		rules := opera.MainNetRules()
		gotJson, err := GenerateJsonNetworkRulesUpdates(rules, updates)
		if err != nil {
			t.Fatalf("failed to generate json: %v", err)
		}

		gotJsonAsMap := make(map[string]any)
		if err := json.Unmarshal([]byte(gotJson), &gotJsonAsMap); err != nil {
			t.Fatalf("failed to unmarshal json: %v", err)
		}

		if got, want := gotJsonAsMap, expected; !maps.EqualFunc(got, want, compareValuesSimpleOrNestedMap) {
			t.Errorf("unexpected json: got: %s != want: %s", got, want)
		}
	})
}

func compareValuesSimpleOrNestedMap(v1, v2 any) bool {
	map1, ok1 := v1.(map[string]any)
	map2, ok2 := v2.(map[string]any)

	if ok1 && ok2 {
		for k, v := range map1 {
			if !compareValuesSimpleOrNestedMap(v, map2[k]) {
				return false
			}
		}

		return true
	}

	return v1 == v2
}

func mergeMapsSimpleValuesOrNestedMap(map1, map2 map[string]any) map[string]any {
	mergedMap := make(map[string]any)

	for k, v := range map1 {
		mergedMap[k] = v
	}

	for k, v2 := range map2 {
		if v1, ok := mergedMap[k]; ok {
			// If both values are maps, merge them recursively
			if map1Nested, ok1 := v1.(map[string]any); ok1 {
				if map2Nested, ok2 := v2.(map[string]any); ok2 {
					mergedMap[k] = mergeMapsSimpleValuesOrNestedMap(map1Nested, map2Nested)
					continue
				}
			}
		}
		mergedMap[k] = v2
	}

	return mergedMap
}
