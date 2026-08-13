package genesis

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strings"
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
	Dag      *DagPatch      `yaml:"Dag,omitempty" json:"Dag,omitempty"`
	Emitter  *EmitterPatch  `yaml:"Emitter,omitempty" json:"Emitter,omitempty"`
	Epochs   *EpochsPatch   `yaml:"Epochs,omitempty" json:"Epochs,omitempty"`
	Blocks   *BlocksPatch   `yaml:"Blocks,omitempty" json:"Blocks,omitempty"`
	Economy  *EconomyPatch  `yaml:"Economy,omitempty" json:"Economy,omitempty"`
	Upgrades *UpgradesPatch `yaml:"Upgrades,omitempty" json:"Upgrades,omitempty"`
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
	// A misspelled rule key would be dropped without a word and the scenario
	// would silently run on stock rules while claiming to patch them. Check
	// the keys against the schema before decoding.
	if err := checkKnownRuleKeys(node, reflect.TypeOf(NetworkRulesPatch{}), ""); err != nil {
		return err
	}

	type alias NetworkRulesPatch
	var decoded alias
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*p = NetworkRulesPatch(decoded)
	return nil
}

// checkKnownRuleKeys reports every key in a rules patch mapping that does not
// name a field of the patch schema, walking nested mappings. The offending key
// is named with its full path. Note that a dotted key is one key to yaml and
// names no field, so it is reported like any other unknown key.
func checkKnownRuleKeys(node *yaml.Node, structType reflect.Type, path string) error {
	if node != nil && node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	// Anything but a mapping is a type error, which the decoder reports.
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}

	errs := []error{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode, valueNode := node.Content[i], node.Content[i+1]

		// Merge keys (<<) carry a mapping of the same shape.
		if keyNode.Tag == "!!merge" {
			errs = append(errs, checkKnownRuleKeys(valueNode, structType, path))
			continue
		}

		fieldType, ok := ruleFieldType(structType, keyNode.Value)
		if !ok {
			errs = append(errs, fmt.Errorf(
				"line %d: unknown network rule %q, no such field in the rules"+
					" patch schema",
				keyNode.Line, joinRulePath(path, keyNode.Value),
			))
			continue
		}

		if fieldType != nil {
			errs = append(errs, checkKnownRuleKeys(
				valueNode, fieldType, joinRulePath(path, keyNode.Value),
			))
		}
	}

	return errors.Join(errs...)
}

// yamlUnmarshaler is the interface of the leaf types that decode themselves,
// such as durations and big integers.
var yamlUnmarshaler = reflect.TypeOf((*yaml.Unmarshaler)(nil)).Elem()

// ruleFieldType returns the type a key names in the given patch struct. Nested
// patch structs are returned with their pointers stripped, so that the walk can
// descend into them; every other field, including the leaf types that decode
// themselves, yields a nil type, ending the walk there.
func ruleFieldType(structType reflect.Type, key string) (reflect.Type, bool) {
	if structType == nil || structType.Kind() != reflect.Struct {
		return nil, false
	}
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		if ruleKeyOf(field) != key {
			continue
		}
		fieldType := field.Type
		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		// A type with its own unmarshaller defines what is valid inside it.
		if fieldType.Kind() != reflect.Struct ||
			reflect.PointerTo(fieldType).Implements(yamlUnmarshaler) {
			return nil, true
		}
		return fieldType, true
	}
	return nil, false
}

// ruleKeyOf returns the key a field is written as in YAML, matching what
// yaml.v3 accepts: the name in the yaml tag, or the lower-cased field name.
func ruleKeyOf(field reflect.StructField) string {
	tag := field.Tag.Get("yaml")
	if name, _, _ := strings.Cut(tag, ","); name != "" {
		return name
	}
	return strings.ToLower(field.Name)
}

func joinRulePath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func (p *NetworkRulesPatch) PrettyPrint() string {
	b, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Sprintf("failed to marshal network rules patch: %v", err)
	}
	return string(b)
}

type DagPatch struct {
	MaxParents     *uint64 `yaml:"MaxParents,omitempty" json:"MaxParents,omitempty"`
	MaxFreeParents *uint64 `yaml:"MaxFreeParents,omitempty" json:"MaxFreeParents,omitempty"`
	MaxExtraData   *uint32 `yaml:"MaxExtraData,omitempty" json:"MaxExtraData,omitempty"`
}

type EmitterPatch struct {
	Interval        *Duration `yaml:"Interval,omitempty" json:"Interval,omitempty"`
	StallThreshold  *Duration `yaml:"StallThreshold,omitempty" json:"StallThreshold,omitempty"`
	StalledInterval *Duration `yaml:"StalledInterval,omitempty" json:"StalledInterval,omitempty"`
}

type EpochsPatch struct {
	MaxEpochGas      *uint64   `yaml:"MaxEpochGas,omitempty" json:"MaxEpochGas,omitempty"`
	MaxEpochDuration *Duration `yaml:"MaxEpochDuration,omitempty" json:"MaxEpochDuration,omitempty"`
}

type BlocksPatch struct {
	MaxBlockGas             *uint64   `yaml:"MaxBlockGas,omitempty" json:"MaxBlockGas,omitempty"`
	MaxEmptyBlockSkipPeriod *Duration `yaml:"MaxEmptyBlockSkipPeriod,omitempty" json:"MaxEmptyBlockSkipPeriod,omitempty"`
}

type EconomyPatch struct {
	BlockMissedSlack *uint64        `yaml:"BlockMissedSlack,omitempty" json:"BlockMissedSlack,omitempty"`
	Gas              *GasPatch      `yaml:"Gas,omitempty" json:"Gas,omitempty"`
	MinGasPrice      *BigIntValue   `yaml:"MinGasPrice,omitempty" json:"MinGasPrice,omitempty"`
	MinBaseFee       *BigIntValue   `yaml:"MinBaseFee,omitempty" json:"MinBaseFee,omitempty"`
	ShortGasPower    *GasPowerPatch `yaml:"ShortGasPower,omitempty" json:"ShortGasPower,omitempty"`
	LongGasPower     *GasPowerPatch `yaml:"LongGasPower,omitempty" json:"LongGasPower,omitempty"`
}

type GasPatch struct {
	MaxEventGas          *uint64 `yaml:"MaxEventGas,omitempty" json:"MaxEventGas,omitempty"`
	EventGas             *uint64 `yaml:"EventGas,omitempty" json:"EventGas,omitempty"`
	ParentGas            *uint64 `yaml:"ParentGas,omitempty" json:"ParentGas,omitempty"`
	ExtraDataGas         *uint64 `yaml:"ExtraDataGas,omitempty" json:"ExtraDataGas,omitempty"`
	BlockVotesBaseGas    *uint64 `yaml:"BlockVotesBaseGas,omitempty" json:"BlockVotesBaseGas,omitempty"`
	BlockVoteGas         *uint64 `yaml:"BlockVoteGas,omitempty" json:"BlockVoteGas,omitempty"`
	EpochVoteGas         *uint64 `yaml:"EpochVoteGas,omitempty" json:"EpochVoteGas,omitempty"`
	MisbehaviourProofGas *uint64 `yaml:"MisbehaviourProofGas,omitempty" json:"MisbehaviourProofGas,omitempty"`
}

type GasPowerPatch struct {
	AllocPerSec        *uint64   `yaml:"AllocPerSec,omitempty" json:"AllocPerSec,omitempty"`
	MaxAllocPeriod     *Duration `yaml:"MaxAllocPeriod,omitempty" json:"MaxAllocPeriod,omitempty"`
	StartupAllocPeriod *Duration `yaml:"StartupAllocPeriod,omitempty" json:"StartupAllocPeriod,omitempty"`
	MinStartupGas      *uint64   `yaml:"MinStartupGas,omitempty" json:"MinStartupGas,omitempty"`
}

type UpgradesPatch struct {
	Berlin                       *bool `yaml:"Berlin,omitempty" json:"Berlin,omitempty"`
	London                       *bool `yaml:"London,omitempty" json:"London,omitempty"`
	Llr                          *bool `yaml:"Llr,omitempty" json:"Llr,omitempty"`
	Sonic                        *bool `yaml:"Sonic,omitempty" json:"Sonic,omitempty"`
	Allegro                      *bool `yaml:"Allegro,omitempty" json:"Allegro,omitempty"`
	Brio                         *bool `yaml:"Brio,omitempty" json:"Brio,omitempty"`
	SingleProposerBlockFormation *bool `yaml:"SingleProposerBlockFormation,omitempty" json:"SingleProposerBlockFormation,omitempty"`
	GasSubsidies                 *bool `yaml:"GasSubsidies,omitempty" json:"GasSubsidies,omitempty"`
	TransactionBundles           *bool `yaml:"TransactionBundles,omitempty" json:"TransactionBundles,omitempty"`
	TransactionPriorities        *bool `yaml:"TransactionPriorities,omitempty" json:"TransactionPriorities,omitempty"`
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
