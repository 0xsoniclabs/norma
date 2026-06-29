package parser

import (
	"time"

	"github.com/0xsoniclabs/norma/genesis"
)

// DefaultMaxEpochDuration is applied to scenarios that do not explicitly set
// MaxEpochDuration in their network rules.
const DefaultMaxEpochDuration = 15 * time.Second

// ensureDefaultEpochDuration sets the default MaxEpochDuration on a patch
// if it is not already set.
func ensureDefaultEpochDuration(patch *genesis.NetworkRulesPatch) {
	if patch.Epochs == nil {
		patch.Epochs = &genesis.EpochsPatch{}
	}
	if patch.Epochs.MaxEpochDuration == nil {
		patch.Epochs.MaxEpochDuration = genesis.NewDuration(DefaultMaxEpochDuration)
	}
}

// setDefaults pass default values into the configuration of a scenario.
func (s *Scenario) setDefaults() {
	ensureDefaultEpochDuration(&s.NetworkRules.Genesis)
}
