package parser

import (
	"time"

	"github.com/0xsoniclabs/norma/genesis"
)

// DefaultMaxEpochDuration is applied to scenarios that do not explicitly set
// MaxEpochDuration in their network rules.
const DefaultMaxEpochDuration = 15 * time.Second

// MinObservationSamples is the smallest number of monitoring samples an
// observation window must be able to hold. Deciding whether a block height
// changed takes two samples, so a shorter window could only ever report that
// nothing was observed. It mirrors the limit the checking package enforces at
// run time; the constants are duplicated because checking depends on this
// package.
const MinObservationSamples = 2

// MinObservationDuration is the shortest observation window a check may
// request, derived from MinObservationSamples at the monitor's sampling rate of
// one sample per second.
const MinObservationDuration = MinObservationSamples * time.Second

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
