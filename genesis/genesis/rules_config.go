package genesis

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/0xsoniclabs/sonic/inter"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/Fantom-foundation/lachesis-base/inter/idx"
	"math/big"
	"os"
	"strconv"
	"time"
)

// NetworkRules defines a set of network rules as a key value mapping.
type NetworkRules map[string]string

// ruleUpdater is a function that updates a rule in the network rules configuration using the given value.
type ruleUpdater func(value string, rules *opera.Rules) error

// registry is a map of network rules configuration functions.
type registry map[string]ruleUpdater

// supportedNetworkRulesConfigurations is a map of currently configured network rules.
var supportedNetworkRulesConfigurations = make(registry)

// init registers all currently supported network rules.
func init() {
	// Blocks
	register("MAX_BLOCK_GAS", maxBlockGas)
	register("MAX_EMPTY_BLOCK_SKIP_PERIOD", maxEmptyBlockSkipPeriod)

	// Epochs
	register("MAX_EPOCH_GAS", maxEpochGas)
	register("MAX_EPOCH_DURATION", maxEpochDuration)

	// Emitter
	register("EMITTER_INTERVAL", emitterInterval)
	register("EMITTER_STALL_THRESHOLD", emitterStallThreshold)
	register("EMITTER_STALLED_INTERVAL", emitterStallInterval)

	// Upgrades
	register("UPGRADES_BERLIN", upgradesBerlin)
	register("UPGRADES_LONDON", upgradesLondon)
	register("UPGRADES_LLR", upgradesLlr)
	register("UPGRADES_SONIC", upgradesSonic)

	// Economy
	register("MIN_GAS_PRICE", minGasPrice)
	register("MIN_BASE_FEE", minBaseFee)
	register("BLOCK_MISSED_SLACK", blockMissedSlack)
	register("MAX_EVENT_GAS", maxEventGas)
	register("EVENT_GAS", eventGas)
	register("PARENT_GAS", parentGas)
	register("EXTRA_DATA_GAS", extraDataGas)
	register("BLOCK_VOTES_BASE_GAS", blockVotesBaseGas)
	register("BLOCK_VOTE_GAS", blockVoteGas)
	register("EPOCH_VOTE_GAS", epochVoteGas)
	register("MISBEHAVIOUR_PROOF_GAS", misbehaviourProofGas)

	register("SHORT_ALLOC_PER_SEC", shortAllocPerSec)
	register("SHORT_MAX_ALLOC_PERIOD", shortMaxAllocPeriod)
	register("SHORT_STARTUP_ALLOC_PERIOD", shortStartupAllocPeriod)
	register("SHORT_MIN_STARTUP_GAS", shortMinStartupGas)

	register("LONG_ALLOC_PER_SEC", longAllocPerSec)
	register("LONG_MAX_ALLOC_PERIOD", longMaxAllocPeriod)
	register("LONG_STARTUP_ALLOC_PERIOD", longStartupAllocPeriod)
	register("LONG_MIN_STARTUP_GAS", longMinStartupGas)

	// DAG rules
	register("MAX_PARENTS", maxParents)
	register("MAX_FREE_PARENTS", maxFreeParents)
	register("MAX_EXTRA_DATA", maxExtraData)
}

// ConfigureNetworkRulesEnv configures the network rules based on the environment variables
// applying all registered rules.
func ConfigureNetworkRulesEnv(rules *opera.Rules) error {
	var errs []error
	for k, v := range supportedNetworkRulesConfigurations {
		property := os.Getenv(k)
		// apply only non-empty values
		if property != "" {
			errs = append(errs, v(property, rules))
		}
	}

	return errors.Join(errs...)
}

// ConfigureNetworkRulesMap configures the network rules based on the given map of updates.
func ConfigureNetworkRulesMap(rules *opera.Rules, updates map[string]string) error {
	var errs []error
	for k, v := range supportedNetworkRulesConfigurations {
		property, ok := updates[k]
		if ok {
			errs = append(errs, v(property, rules))
		}
	}

	return errors.Join(errs...)
}

// GenerateJsonFakeNetNetworkRulesUpdates generates a JSON string with the differences between the original network rules
// and the updated network rules configuration, provided on the input.
// This function uses the default fake network rules as the original configuration.
func GenerateJsonFakeNetNetworkRulesUpdates(updates NetworkRules) (string, error) {
	originalRules := opera.FakeNetRules(opera.SonicFeatures)
	return GenerateJsonNetworkRulesUpdates(originalRules, updates)
}

// GenerateJsonNetworkRulesUpdates generates a JSON string with the differences between the original network rules
// and the updated network rules configuration, provided on the input.
func GenerateJsonNetworkRulesUpdates(rules opera.Rules, updates NetworkRules) (string, error) {
	original := rules.String()

	// apply the rules configuration
	if err := ConfigureNetworkRulesMap(&rules, updates); err != nil {
		return "", fmt.Errorf("failed to configure network rules: %w", err)
	}

	// Parse JSON into maps
	var objA, objB map[string]any
	if err := json.Unmarshal([]byte(original), &objA); err != nil {
		return "", err
	}
	if err := json.Unmarshal([]byte(rules.String()), &objB); err != nil {
		return "", err
	}

	diff := diffMapsSameStructure(objA, objB)
	b, err := json.Marshal(diff)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

// diffMapsSameStructure identifies and returns the differences between two maps that share the same structure.
// This simplified diff function compares values only for matching keys in both maps.
// It assumes that the two maps do not contain unique or additional keys.
// The result is a map consisting of only the key-value pairs where the values differ between the two input maps.
// Nested maps are fully supported and will be compared recursively.
func diffMapsSameStructure(map1, map2 map[string]any) map[string]any {
	result := make(map[string]any)

	// Iterate over keys in map1
	for key, value1 := range map1 {
		if value2, exists := map2[key]; exists {
			// Check if both values are maps
			nestedMap1, ok1 := value1.(map[string]any)
			nestedMap2, ok2 := value2.(map[string]any)
			if ok1 && ok2 {
				// Recurse on nested maps
				nestedDiff := diffMapsSameStructure(nestedMap1, nestedMap2)
				if len(nestedDiff) > 0 { // Add only non-empty differences
					result[key] = nestedDiff
				}
			} else if value1 != value2 {
				// Add differing values
				result[key] = value2
			}
		}
	}

	return result
}

// register registers a new network rule configuration.
func register(key string, apply ruleUpdater) {
	supportedNetworkRulesConfigurations[key] = func(value string, rules *opera.Rules) error {
		return apply(value, rules)
	}
}

var maxBlockGas = func(value string, rules *opera.Rules) (err error) {
	rules.Blocks.MaxBlockGas, err = strconv.ParseUint(value, 10, 64)
	return err
}

var maxEpochGas = func(value string, rules *opera.Rules) (err error) {
	rules.Epochs.MaxEpochGas, err = strconv.ParseUint(value, 10, 64)
	return err
}

var maxEmptyBlockSkipPeriod = func(value string, rules *opera.Rules) error {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	rules.Blocks.MaxEmptyBlockSkipPeriod = inter.Timestamp(duration)
	return err
}

var maxEpochDuration = func(value string, rules *opera.Rules) error {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	rules.Epochs.MaxEpochDuration = inter.Timestamp(duration)
	return err
}

var emitterInterval = func(value string, rules *opera.Rules) error {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	rules.Emitter.Interval = inter.Timestamp(duration)
	return err
}

var emitterStallThreshold = func(value string, rules *opera.Rules) error {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	rules.Emitter.StallThreshold = inter.Timestamp(duration)
	return err
}

var emitterStallInterval = func(value string, rules *opera.Rules) error {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	rules.Emitter.StalledInterval = inter.Timestamp(duration)
	return err
}

var upgradesBerlin = func(value string, rules *opera.Rules) error {
	rules.Upgrades.Berlin = value == "true"
	return nil
}

var upgradesLondon = func(value string, rules *opera.Rules) error {
	rules.Upgrades.London = value == "true"
	return nil
}

var upgradesLlr = func(value string, rules *opera.Rules) error {
	rules.Upgrades.Llr = value == "true"
	return nil
}

var upgradesSonic = func(value string, rules *opera.Rules) error {
	rules.Upgrades.Sonic = value == "true"
	return nil
}

var minGasPrice = func(value string, rules *opera.Rules) error {
	var ok bool
	var err error
	number := new(big.Int)
	rules.Economy.MinGasPrice, ok = number.SetString(value, 10)
	if !ok {
		err = fmt.Errorf("cannot parse %s as a number", value)
	}
	return err
}

var minBaseFee = func(value string, rules *opera.Rules) error {
	var ok bool
	var err error
	number := new(big.Int)
	rules.Economy.MinBaseFee, ok = number.SetString(value, 10)
	if !ok {
		err = fmt.Errorf("cannot parse %s as a number", value)
	}
	return err
}

var blockMissedSlack = func(value string, rules *opera.Rules) error {
	number, err := strconv.ParseUint(value, 10, 64)
	rules.Economy.BlockMissedSlack = idx.Block(number)
	return err
}

var maxEventGas = func(value string, rules *opera.Rules) (err error) {
	rules.Economy.Gas.MaxEventGas, err = strconv.ParseUint(value, 10, 64)
	return err
}

var eventGas = func(value string, rules *opera.Rules) (err error) {
	rules.Economy.Gas.EventGas, err = strconv.ParseUint(value, 10, 64)
	return err
}

var parentGas = func(value string, rules *opera.Rules) (err error) {
	rules.Economy.Gas.ParentGas, err = strconv.ParseUint(value, 10, 64)
	return err
}

var extraDataGas = func(value string, rules *opera.Rules) (err error) {
	rules.Economy.Gas.ExtraDataGas, err = strconv.ParseUint(value, 10, 64)
	return err
}

var blockVotesBaseGas = func(value string, rules *opera.Rules) (err error) {
	rules.Economy.Gas.BlockVotesBaseGas, err = strconv.ParseUint(value, 10, 64)
	return err
}

var blockVoteGas = func(value string, rules *opera.Rules) (err error) {
	rules.Economy.Gas.BlockVoteGas, err = strconv.ParseUint(value, 10, 64)
	return err
}

var epochVoteGas = func(value string, rules *opera.Rules) (err error) {
	rules.Economy.Gas.EpochVoteGas, err = strconv.ParseUint(value, 10, 64)
	return err
}

var misbehaviourProofGas = func(value string, rules *opera.Rules) (err error) {
	rules.Economy.Gas.MisbehaviourProofGas, err = strconv.ParseUint(value, 10, 64)
	return err
}

var shortAllocPerSec = func(value string, rules *opera.Rules) (err error) {
	rules.Economy.ShortGasPower.AllocPerSec, err = strconv.ParseUint(value, 10, 64)
	return err
}

var shortMaxAllocPeriod = func(value string, rules *opera.Rules) (err error) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	rules.Economy.ShortGasPower.MaxAllocPeriod = inter.Timestamp(duration)
	return nil
}

var shortStartupAllocPeriod = func(value string, rules *opera.Rules) (err error) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	rules.Economy.ShortGasPower.StartupAllocPeriod = inter.Timestamp(duration)
	return nil
}

var shortMinStartupGas = func(value string, rules *opera.Rules) (err error) {
	rules.Economy.ShortGasPower.MinStartupGas, err = strconv.ParseUint(value, 10, 64)
	return err
}

var longAllocPerSec = func(value string, rules *opera.Rules) (err error) {
	rules.Economy.LongGasPower.AllocPerSec, err = strconv.ParseUint(value, 10, 64)
	return err
}

var longMaxAllocPeriod = func(value string, rules *opera.Rules) (err error) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	rules.Economy.LongGasPower.MaxAllocPeriod = inter.Timestamp(duration)
	return nil
}

var longStartupAllocPeriod = func(value string, rules *opera.Rules) (err error) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	rules.Economy.LongGasPower.StartupAllocPeriod = inter.Timestamp(duration)
	return nil
}

var longMinStartupGas = func(value string, rules *opera.Rules) (err error) {
	rules.Economy.LongGasPower.MinStartupGas, err = strconv.ParseUint(value, 10, 64)
	return err
}

var maxParents = func(value string, rules *opera.Rules) error {
	number, err := strconv.ParseUint(value, 10, 64)
	rules.Dag.MaxParents = idx.Event(number)
	return err
}

var maxFreeParents = func(value string, rules *opera.Rules) error {
	number, err := strconv.ParseUint(value, 10, 64)
	rules.Dag.MaxFreeParents = idx.Event(number)
	return err
}

var maxExtraData = func(value string, rules *opera.Rules) error {
	number, err := strconv.ParseUint(value, 10, 32)
	rules.Dag.MaxExtraData = uint32(number)
	return err
}
