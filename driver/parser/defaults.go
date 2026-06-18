package parser

import (
	"time"

	"github.com/0xsoniclabs/norma/genesis"
)

// setDefaults pass default values into the configuration of a scenario.
func (s *Scenario) setDefaults() {
	if s.NetworkRules.Genesis.Epochs == nil {
		s.NetworkRules.Genesis.Epochs = &genesis.EpochsPatch{}
	}
	if s.NetworkRules.Genesis.Epochs.MaxEpochDuration == nil {
		s.NetworkRules.Genesis.Epochs.MaxEpochDuration = genesis.NewDuration(15 * time.Second)
	}
}
