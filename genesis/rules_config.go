package genesis

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"time"

	"github.com/0xsoniclabs/sonic/inter"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/Fantom-foundation/lachesis-base/inter/idx"
)

// NetworkRules defines a set of network rules as a key value mapping.
type NetworkRules map[string]string

type ruleUpdater func(value string, rules *opera.Rules) error
type registry map[string]ruleUpdater

var supportedNetworkRulesConfigurations = make(registry)

func init() {
	register("MAX_BLOCK_GAS", maxBlockGas)
	register("MAX_EMPTY_BLOCK_SKIP_PERIOD", maxEmptyBlockSkipPeriod)
	register("MAX_EPOCH_GAS", maxEpochGas)
	register("MAX_EPOCH_DURATION", maxEpochDuration)
	register("EMITTER_INTERVAL", emitterInterval)
	register("EMITTER_STALL_THRESHOLD", emitterStallThreshold)
	register("EMITTER_STALLED_INTERVAL", emitterStallInterval)
	register("UPGRADES_BERLIN", upgradesBerlin)
	register("UPGRADES_LONDON", upgradesLondon)
	register("UPGRADES_LLR", upgradesLlr)
	register("UPGRADES_SONIC", upgradesSonic)
	register("UPGRADES_ALLEGRO", upgradesAllegro)
	register("UPGRADES_BRIO", upgradesBrio)
	register("UPGRADES_SINGLE_PROPOSER", upgradesSingleProposer)
	register("UPGRADES_GAS_SUBSIDIES", upgradesGasSubsidies)
	register("UPGRADES_TRANSACTION_BUNDLES", upgradesTransactionBundles)
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
	register("MAX_PARENTS", maxParents)
	register("MAX_FREE_PARENTS", maxFreeParents)
	register("MAX_EXTRA_DATA", maxExtraData)
}

func IsSupportedNetworkRule(key string) bool {
	_, ok := supportedNetworkRulesConfigurations[key]
	return ok
}

func ConfigureNetworkRulesEnv(rules *opera.Rules) error {
	var errs []error
	for k, v := range supportedNetworkRulesConfigurations {
		property := os.Getenv(k)
		if property != "" {
			errs = append(errs, v(property, rules))
		}
	}
	return errors.Join(errs...)
}

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

func GenerateJsonNetworkRulesUpdates(rules opera.Rules, updates NetworkRules) (string, error) {
	original := rules.String()
	if err := ConfigureNetworkRulesMap(&rules, updates); err != nil {
		return "", fmt.Errorf("failed to configure network rules: %w", err)
	}

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

func diffMapsSameStructure(map1, map2 map[string]any) map[string]any {
	result := make(map[string]any)
	for key, value1 := range map1 {
		if value2, exists := map2[key]; exists {
			nestedMap1, ok1 := value1.(map[string]any)
			nestedMap2, ok2 := value2.(map[string]any)
			if ok1 && ok2 {
				nestedDiff := diffMapsSameStructure(nestedMap1, nestedMap2)
				if len(nestedDiff) > 0 {
					result[key] = nestedDiff
				}
			} else if value1 != value2 {
				result[key] = value2
			}
		}
	}
	return result
}

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
	return nil
}

var maxEpochDuration = func(value string, rules *opera.Rules) error {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	rules.Epochs.MaxEpochDuration = inter.Timestamp(duration)
	return nil
}

var emitterInterval = func(value string, rules *opera.Rules) error {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	rules.Emitter.Interval = inter.Timestamp(duration)
	return nil
}

var emitterStallThreshold = func(value string, rules *opera.Rules) error {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	rules.Emitter.StallThreshold = inter.Timestamp(duration)
	return nil
}

var emitterStallInterval = func(value string, rules *opera.Rules) error {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	rules.Emitter.StalledInterval = inter.Timestamp(duration)
	return nil
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

var upgradesAllegro = func(value string, rules *opera.Rules) error {
	rules.Upgrades.Allegro = value == "true"
	return nil
}

var upgradesBrio = func(value string, rules *opera.Rules) error {
	rules.Upgrades.Brio = value == "true"
	return nil
}

var upgradesSingleProposer = func(value string, rules *opera.Rules) error {
	rules.Upgrades.SingleProposerBlockFormation = value == "true"
	return nil
}

var upgradesGasSubsidies = func(value string, rules *opera.Rules) error {
	rules.Upgrades.GasSubsidies = value == "true"
	return nil
}

var upgradesTransactionBundles = func(value string, rules *opera.Rules) error {
	rules.Upgrades.TransactionBundles = value == "true"
	return nil
}

var minGasPrice = func(value string, rules *opera.Rules) error {
	var ok bool
	number := new(big.Int)
	rules.Economy.MinGasPrice, ok = number.SetString(value, 10)
	if !ok {
		return fmt.Errorf("cannot parse %s as a number", value)
	}
	return nil
}

var minBaseFee = func(value string, rules *opera.Rules) error {
	var ok bool
	number := new(big.Int)
	rules.Economy.MinBaseFee, ok = number.SetString(value, 10)
	if !ok {
		return fmt.Errorf("cannot parse %s as a number", value)
	}
	return nil
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
	number, err := strconv.ParseUint(value, 10, 64)
	rules.Dag.MaxExtraData = uint32(number)
	return err
}
