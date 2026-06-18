package genesis

import (
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/0xsoniclabs/sonic/inter"
	"github.com/0xsoniclabs/sonic/opera"
	"gopkg.in/yaml.v3"
)

// Duration is a YAML/JSON helper for inter.Timestamp fields.
// It accepts either duration strings (e.g. "15s") or integer nanoseconds.
type Duration inter.Timestamp

func NewDuration(d time.Duration) *Duration {
	v := Duration(inter.Timestamp(d))
	return &v
}

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == "!!str" {
			parsed, err := time.ParseDuration(node.Value)
			if err != nil {
				return fmt.Errorf("invalid duration %q: %w", node.Value, err)
			}
			*d = Duration(inter.Timestamp(parsed))
			return nil
		}

		var n int64
		if err := node.Decode(&n); err != nil {
			return err
		}
		*d = Duration(inter.Timestamp(n))
		return nil
	default:
		return fmt.Errorf("duration must be a scalar value")
	}
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(int64(d))
}

// BigIntValue accepts YAML scalar values for big integers.
type BigIntValue big.Int

func NewBigIntValue(i int64) BigIntValue {
	return BigIntValue(*big.NewInt(i))
}

func (b *BigIntValue) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("big integer must be a scalar value")
	}

	raw := node.Value
	value, ok := new(big.Int).SetString(raw, 10)
	if !ok {
		return fmt.Errorf("invalid big integer %q", raw)
	}

	*b = BigIntValue(*value)
	return nil
}

func (b *BigIntValue) UnmarshalJSON(data []byte) error {
	v := new(big.Int)
	if err := v.UnmarshalJSON(data); err != nil {
		return err
	}
	*b = BigIntValue(*v)
	return nil
}

func (b BigIntValue) MarshalJSON() ([]byte, error) {
	v := big.Int(b)
	return v.MarshalJSON()
}

func (b BigIntValue) MarshalYAML() (any, error) {
	v := big.Int(b)
	return v.String(), nil
}

// NetworkRulesPatch defines a set of network rules that can be applied to the network.
// Network rules contains all the fields in sonic's opera.Rules, but all fields
// are optional and only the non-nil fields will be applied to the network.
//
// This type is used to define the initial rule set in the genesis, by applying
// the diff to the default rules: opera.FakeNetRules(opera.GetSonicUpgrades())
// Additionally it can be sent serialized using json to change the network rules
// during execution.
type NetworkRulesPatch struct {
	Dag      *DagPatch      `yaml:"dag,omitempty" json:"Dag,omitempty"`
	Emitter  *EmitterPatch  `yaml:"emitter,omitempty" json:"Emitter,omitempty"`
	Epochs   *EpochsPatch   `yaml:"epochs,omitempty" json:"Epochs,omitempty"`
	Blocks   *BlocksPatch   `yaml:"blocks,omitempty" json:"Blocks,omitempty"`
	Economy  *EconomyPatch  `yaml:"economy,omitempty" json:"Economy,omitempty"`
	Upgrades *UpgradesPatch `yaml:"upgrades,omitempty" json:"Upgrades,omitempty"`
}

func NewRulesPatchFromOperaRules(rules opera.Rules) (NetworkRulesPatch, error) {
	patchJSON, err := json.Marshal(rules)
	if err != nil {
		return NetworkRulesPatch{}, fmt.Errorf("failed to marshal opera rules: %w", err)
	}

	var patch NetworkRulesPatch
	if err := json.Unmarshal(patchJSON, &patch); err != nil {
		return NetworkRulesPatch{}, fmt.Errorf("failed to unmarshal opera rules into patch: %w", err)
	}
	return patch, nil
}

func (p *NetworkRulesPatch) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("network rules patch must be a map")
	}

	type alias NetworkRulesPatch
	var decoded alias
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*p = NetworkRulesPatch(decoded)
	return nil
}

func (p *NetworkRulesPatch) PrettyPrint() string {
	b, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Sprintf("failed to marshal network rules patch: %v", err)
	}
	return string(b)
}

type DagPatch struct {
	MaxParents     *uint64 `yaml:"max_parents,omitempty" json:"MaxParents,omitempty"`
	MaxFreeParents *uint64 `yaml:"max_free_parents,omitempty" json:"MaxFreeParents,omitempty"`
	MaxExtraData   *uint32 `yaml:"max_extra_data,omitempty" json:"MaxExtraData,omitempty"`
}

type EmitterPatch struct {
	Interval        *Duration `yaml:"interval,omitempty" json:"Interval,omitempty"`
	StallThreshold  *Duration `yaml:"stall_threshold,omitempty" json:"StallThreshold,omitempty"`
	StalledInterval *Duration `yaml:"stalled_interval,omitempty" json:"StalledInterval,omitempty"`
}

type EpochsPatch struct {
	MaxEpochGas      *uint64   `yaml:"max_epoch_gas,omitempty" json:"MaxEpochGas,omitempty"`
	MaxEpochDuration *Duration `yaml:"max_epoch_duration,omitempty" json:"MaxEpochDuration,omitempty"`
}

type BlocksPatch struct {
	MaxBlockGas             *uint64   `yaml:"max_block_gas,omitempty" json:"MaxBlockGas,omitempty"`
	MaxEmptyBlockSkipPeriod *Duration `yaml:"max_empty_block_skip_period,omitempty" json:"MaxEmptyBlockSkipPeriod,omitempty"`
}

type EconomyPatch struct {
	BlockMissedSlack *uint64        `yaml:"block_missed_slack,omitempty" json:"BlockMissedSlack,omitempty"`
	Gas              *GasPatch      `yaml:"gas,omitempty" json:"Gas,omitempty"`
	MinGasPrice      *BigIntValue   `yaml:"min_gas_price,omitempty" json:"MinGasPrice,omitempty"`
	MinBaseFee       *BigIntValue   `yaml:"min_base_fee,omitempty" json:"MinBaseFee,omitempty"`
	ShortGasPower    *GasPowerPatch `yaml:"short_gas_power,omitempty" json:"ShortGasPower,omitempty"`
	LongGasPower     *GasPowerPatch `yaml:"long_gas_power,omitempty" json:"LongGasPower,omitempty"`
}

type GasPatch struct {
	MaxEventGas          *uint64 `yaml:"max_event_gas,omitempty" json:"MaxEventGas,omitempty"`
	EventGas             *uint64 `yaml:"event_gas,omitempty" json:"EventGas,omitempty"`
	ParentGas            *uint64 `yaml:"parent_gas,omitempty" json:"ParentGas,omitempty"`
	ExtraDataGas         *uint64 `yaml:"extra_data_gas,omitempty" json:"ExtraDataGas,omitempty"`
	BlockVotesBaseGas    *uint64 `yaml:"block_votes_base_gas,omitempty" json:"BlockVotesBaseGas,omitempty"`
	BlockVoteGas         *uint64 `yaml:"block_vote_gas,omitempty" json:"BlockVoteGas,omitempty"`
	EpochVoteGas         *uint64 `yaml:"epoch_vote_gas,omitempty" json:"EpochVoteGas,omitempty"`
	MisbehaviourProofGas *uint64 `yaml:"misbehaviour_proof_gas,omitempty" json:"MisbehaviourProofGas,omitempty"`
}

type GasPowerPatch struct {
	AllocPerSec        *uint64   `yaml:"alloc_per_sec,omitempty" json:"AllocPerSec,omitempty"`
	MaxAllocPeriod     *Duration `yaml:"max_alloc_period,omitempty" json:"MaxAllocPeriod,omitempty"`
	StartupAllocPeriod *Duration `yaml:"startup_alloc_period,omitempty" json:"StartupAllocPeriod,omitempty"`
	MinStartupGas      *uint64   `yaml:"min_startup_gas,omitempty" json:"MinStartupGas,omitempty"`
}

type UpgradesPatch struct {
	Berlin                       *bool `yaml:"berlin,omitempty" json:"Berlin,omitempty"`
	London                       *bool `yaml:"london,omitempty" json:"London,omitempty"`
	Llr                          *bool `yaml:"llr,omitempty" json:"Llr,omitempty"`
	Sonic                        *bool `yaml:"sonic,omitempty" json:"Sonic,omitempty"`
	Allegro                      *bool `yaml:"allegro,omitempty" json:"Allegro,omitempty"`
	Brio                         *bool `yaml:"brio,omitempty" json:"Brio,omitempty"`
	SingleProposerBlockFormation *bool `yaml:"single_proposer_block_formation,omitempty" json:"SingleProposerBlockFormation,omitempty"`
	GasSubsidies                 *bool `yaml:"gas_subsidies,omitempty" json:"GasSubsidies,omitempty"`
	TransactionBundles           *bool `yaml:"transaction_bundles,omitempty" json:"TransactionBundles,omitempty"`
}

func ApplyNetworkRulesPatch(rules *opera.Rules, patch NetworkRulesPatch) error {
	patchJSON, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal network rules patch: %w", err)
	}

	updated, err := opera.UpdateRules(*rules, patchJSON)
	if err != nil {
		return fmt.Errorf("failed to apply network rules patch: %w", err)
	}
	*rules = updated
	return nil
}

func ValidateNetworkRulesPatch(patch NetworkRulesPatch) error {
	rules := opera.FakeNetRules(opera.GetSonicUpgrades())
	return ApplyNetworkRulesPatch(&rules, patch)
}
