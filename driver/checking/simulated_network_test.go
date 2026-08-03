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

package checking

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0xsoniclabs/norma/driver/monitoring"
)

func TestSimulation_SamplesFollowTheConfiguredCadence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		net.addNode("A", nodeBehavior{
			height:   production{blockInterval: time.Second}.heightAt,
			interval: time.Second,
		})

		net.run(10 * time.Second)

		got := net.heightsIn("A", simulationEpoch, simulationEpoch.Add(11*time.Second))
		want := []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		if !slices.Equal(got, want) {
			t.Errorf("sampled heights %v, want %v", got, want)
		}
	})
}

func TestSimulation_HaltedProductionKeepsHeightConstant(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		haltAt := simulationEpoch.Add(5 * time.Second)
		net.addNode("A", nodeBehavior{
			height: production{
				blockInterval: time.Second,
				haltAt:        haltAt,
			}.heightAt,
			interval: time.Second,
		})

		net.run(10 * time.Second)

		after := net.heightsIn("A", haltAt, simulationEpoch.Add(11*time.Second))
		if len(after) == 0 {
			t.Fatalf("no samples after the halt")
		}
		for _, height := range after {
			if height != 5 {
				t.Errorf("height %d after the halt, want a constant 5 (all: %v)",
					height, after)
			}
		}
	})
}

func TestSimulation_ResumedProductionAdvancesAgain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		net.addNode("A", nodeBehavior{
			height: production{
				blockInterval: time.Second,
				haltAt:        simulationEpoch.Add(5 * time.Second),
				resumeAt:      simulationEpoch.Add(10 * time.Second),
			}.heightAt,
			interval: time.Second,
		})

		net.run(15 * time.Second)

		got := net.heightsIn("A", simulationEpoch, simulationEpoch.Add(16*time.Second))
		want := []uint64{0, 1, 2, 3, 4, 5, 5, 5, 5, 5, 5, 6, 7, 8, 9, 10}
		if !slices.Equal(got, want) {
			t.Errorf("sampled heights %v, want %v", got, want)
		}
	})
}

func TestSimulation_FailedReadsLeaveHolesInTheSeries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		net.addNode("A", nodeBehavior{
			height:   production{blockInterval: time.Second}.heightAt,
			interval: time.Second,
			dropRate: 1, // every read fails
		})

		net.run(10 * time.Second)

		if got := net.heightsIn("A", simulationEpoch, simulationEpoch.Add(time.Hour)); len(got) != 0 {
			t.Errorf("got %d samples for a node whose reads all fail", len(got))
		}
	})
}

func TestSimulation_UnreachableNodeStopsBeingSampled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		stoppedAt := simulationEpoch.Add(5 * time.Second)
		net.addNode("A", nodeBehavior{
			height:          production{blockInterval: time.Second}.heightAt,
			interval:        time.Second,
			unreachableFrom: stoppedAt,
		})

		net.run(10 * time.Second)

		latest := net.GetBlockStatus("A").GetLatest()
		if latest == nil {
			t.Fatalf("expected samples from before the node was stopped")
		}
		if at := latest.Position.Time(); !at.Before(stoppedAt) {
			t.Errorf("last sample taken at %v, want before %v", at, stoppedAt)
		}
	})
}

func TestSimulation_GasRatesAreRecordedPerProducedBlock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		net.addNode("A", nodeBehavior{
			height:   production{blockInterval: time.Second}.heightAt,
			interval: time.Second,
		})
		net.withGasRates(func(block uint64) float64 { return float64(block) * 10 })

		net.run(3 * time.Second)

		got := net.GetBlockGasRate().GetRange(0, 100)
		want := []monitoring.DataPoint[monitoring.BlockNumber, float64]{
			{Position: 0, Value: 0},
			{Position: 1, Value: 10},
			{Position: 2, Value: 20},
			{Position: 3, Value: 30},
		}
		if !slices.Equal(got, want) {
			t.Errorf("gas rate series %v, want %v", got, want)
		}
	})
}

func TestSimulation_RandomSamplingIsReproducibleForASeed(t *testing.T) {
	// Each simulation gets its own bubble: within one, the clock has already
	// moved on and a second simulation could not start at the epoch.
	sample := func(seed int64) []monitoring.DataPoint[monitoring.Time, monitoring.BlockStatus] {
		var points []monitoring.DataPoint[monitoring.Time, monitoring.BlockStatus]
		synctest.Test(t, func(t *testing.T) {
			net := newSimulation(t, seed)
			net.addNode("A", net.randomSampling(
				production{blockInterval: 300 * time.Millisecond}.heightAt,
			))
			net.run(20 * time.Second)
			points = net.GetBlockStatus("A").GetRange(
				0, monitoring.NewTime(simulationEpoch.Add(time.Hour)),
			)
		})
		return points
	}

	if first, second := sample(7), sample(7); !slices.Equal(first, second) {
		t.Errorf("seed 7 produced different series on two runs")
	}
	if first, second := sample(7), sample(8); slices.Equal(first, second) {
		t.Errorf("seeds 7 and 8 produced identical series")
	}
}

func TestSimulation_RandomSamplingStaysWithinItsInterval(t *testing.T) {
	eachSeed(t, func(t *testing.T, seed int64) {
		net := newSimulation(t, seed)
		net.addNode("A", net.randomSampling(
			production{blockInterval: time.Second}.heightAt,
		))
		net.run(30 * time.Second)

		points := net.GetBlockStatus("A").GetRange(
			0, monitoring.NewTime(simulationEpoch.Add(time.Hour)),
		)
		for i := 1; i < len(points); i++ {
			gap := points[i].Position.Time().Sub(points[i-1].Position.Time())
			if gap <= 0 {
				t.Fatalf("seed %d: samples %d and %d are not ordered", seed, i-1, i)
			}
		}
	})
}
