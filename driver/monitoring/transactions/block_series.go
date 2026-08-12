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

package txmon

import (
	"sort"

	mon "github.com/0xsoniclabs/norma/driver/monitoring"
)

// blockSeries is a read-only view on values observed per block, ordered by block
// height. Such values are not kept in a monitoring.SyncedSeries: the blocks of a
// network are announced by every node of it, and the first announcement of a
// height is not guaranteed to be the lowest one still missing.
type blockSeries[T any] struct {
	items []T
	block func(T) int
	value func(T) int
}

func (s *blockSeries[T]) GetRange(from, to mon.BlockNumber) []mon.DataPoint[mon.BlockNumber, int] {
	begin := s.search(from)
	end := s.search(to)
	points := make([]mon.DataPoint[mon.BlockNumber, int], 0, end-begin)
	for _, item := range s.items[begin:end] {
		points = append(points, mon.DataPoint[mon.BlockNumber, int]{
			Position: mon.BlockNumber(s.block(item)),
			Value:    s.value(item),
		})
	}
	return points
}

func (s *blockSeries[T]) GetLatest() *mon.DataPoint[mon.BlockNumber, int] {
	if len(s.items) == 0 {
		return nil
	}
	last := s.items[len(s.items)-1]
	return &mon.DataPoint[mon.BlockNumber, int]{
		Position: mon.BlockNumber(s.block(last)),
		Value:    s.value(last),
	}
}

// search returns the index of the first value at or above the given block.
func (s *blockSeries[T]) search(block mon.BlockNumber) int {
	return sort.Search(len(s.items), func(i int) bool {
		return mon.BlockNumber(s.block(s.items[i])) >= block
	})
}
