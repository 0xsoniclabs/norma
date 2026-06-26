// Copyright 2024 Fantom Foundation
// This file is part of Norma System Testing Infrastructure for Sonic.
//
// Norma is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Norma is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with Norma. If not, see <http://www.gnu.org/licenses/>.

package netmon

import (
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	mon "github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/monitoring/utils"
	"github.com/0xsoniclabs/sonic/gossip/contract/sfc100"
	"github.com/0xsoniclabs/sonic/opera/contracts/sfc"
)

// ValidatorStake tracks validator received stakes in the current epoch.
var ValidatorStake = mon.Metric[mon.Node, mon.Series[mon.Time, string]]{
	Name:        "ValidatorStake",
	Description: "The current-epoch received stake per validator obtained from SFC.GetEpochReceivedStake().",
}

func init() {
	if err := mon.RegisterSource(ValidatorStake, NewValidatorStakeSource); err != nil {
		panic(fmt.Sprintf("failed to register metric source: %v", err))
	}
}

type validatorStakeSource struct {
	*utils.SyncedSeriesSource[mon.Node, mon.Time, string]
	stop      chan<- bool
	done      <-chan bool
	collector func() (map[int]string, error)
}

// NewValidatorStakeSource creates a new source collecting validator stakes every second.
func NewValidatorStakeSource(monitor *mon.Monitor) mon.Source[mon.Node, mon.Series[mon.Time, string]] {
	return newValidatorStakeSourceWithCollector(monitor, 1*time.Second, func() (map[int]string, error) {
		return fetchValidatorStakes(monitor.Network())
	})
}

func newValidatorStakeSourceWithCollector(
	monitor *mon.Monitor,
	period time.Duration,
	collector func() (map[int]string, error),
) mon.Source[mon.Node, mon.Series[mon.Time, string]] {
	stop := make(chan bool)
	done := make(chan bool)

	res := &validatorStakeSource{
		SyncedSeriesSource: utils.NewSyncedSeriesSource(ValidatorStake),
		stop:               stop,
		done:               done,
		collector:          collector,
	}

	go func() {
		defer close(done)
		ticker := time.NewTicker(period)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				stakes, err := res.collector()
				if err != nil {
					if !errors.Is(err, driver.ErrEmptyNetwork) {
						slog.Error("failed to fetch validator stakes", "metric", ValidatorStake.Name, "error", err)
					}
					continue
				}

				for validatorID, stake := range stakes {
					subject := mon.Node(fmt.Sprintf("validator-%d", validatorID))
					series := res.GetOrAddSubject(subject)
					if err := series.Append(mon.NewTime(now), stake); err != nil {
						slog.Error("failed to append validator stake", "metric", ValidatorStake.Name, "validator_id", validatorID, "error", err)
					}
				}
			case <-stop:
				return
			}
		}
	}()

	return res
}

func (s *validatorStakeSource) Shutdown() error {
	close(s.stop)
	<-s.done
	return s.SyncedSeriesSource.Shutdown()
}

func fetchValidatorStakes(network driver.Network) (map[int]string, error) {
	rpcClient, err := network.DialRandomRpc()
	if err != nil {
		if errors.Is(err, driver.ErrEmptyNetwork) {
			return map[int]string{}, nil
		}
		return nil, fmt.Errorf("failed to connect to RPC: %w", err)
	}
	defer rpcClient.Close()

	sfcContract, err := sfc100.NewContract(sfc.ContractAddress, rpcClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create SFC contract binding: %w", err)
	}

	lastValidatorID, err := sfcContract.LastValidatorID(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to query last validator id: %w", err)
	}
	epoch, err := sfcContract.CurrentEpoch(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to query current epoch: %w", err)
	}

	stakes := make(map[int]string)
	for validatorID := 1; validatorID <= int(lastValidatorID.Int64()); validatorID++ {
		validatorIDBig := big.NewInt(int64(validatorID))
		stake, err := sfcContract.GetEpochReceivedStake(nil, epoch, validatorIDBig)
		if err != nil {
			return nil, fmt.Errorf("failed to query current-epoch received stake for validator %d: %w", validatorID, err)
		}

		stakes[validatorID] = stake.String()
	}

	return stakes, nil
}
