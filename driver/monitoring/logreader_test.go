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

package monitoring

import (
	"strings"
	"testing"
	"time"
)

func TestParseTime(t *testing.T) {
	tt, err := parseTime("[05-04|09:34:16.512]")
	if err != nil {
		t.Errorf("cannot parse: %s", err)
	}

	if got, want := tt.Year(), time.Now().Year(); got != want {
		t.Errorf("wrong year parsed, want %d, got %d", want, got)
	}
	if val := tt.Month(); val != 5 {
		t.Errorf("wrong time parsed: %d", val)
	}
	if val := tt.Day(); val != 4 {
		t.Errorf("wrong time parsed: %d", val)
	}
	if val := tt.Hour(); val != 9 {
		t.Errorf("wrong time parsed: %d", val)
	}
	if val := tt.Minute(); val != 34 {
		t.Errorf("wrong time parsed: %d", val)
	}
	if val := tt.Second(); val != 16 {
		t.Errorf("wrong time parsed: %d", val)
	}
	if val := tt.Nanosecond(); val != 512*1e6 {
		t.Errorf("wrong time parsed: %d", val)
	}
}

func TestParseBlock(t *testing.T) {
	blockReader := NewLogReader(strings.NewReader(Node1TestLog), func(string, error) {})

	var count int
	for got := range blockReader {
		want, exists := BlockHeight1TestMap[got.Height]
		if !exists {
			t.Errorf("unknow block: %d", got.Height)
		}

		if want.Txs != got.Txs {
			t.Errorf("values do not match: %v != %v", want.Txs, got.Txs)
		}
		if want.GasUsed != got.GasUsed {
			t.Errorf("values do not match: %v != %v", want.GasUsed, got.GasUsed)
		}
		if want.Time != got.Time {
			t.Errorf("values do not match: %v != %v", want.Time, got.Time)
		}
		if want.ProcessingTime != got.ProcessingTime {
			t.Errorf("values do not match: %s != %s", want.Time, got.Time)
		}

		count++
	}

	if len(BlockHeight1TestMap) != count {
		t.Errorf("not all keys were visited")
	}
}

func TestParseLogStream(t *testing.T) {
	blockReader := NewLogReader(strings.NewReader(Node1TestLog), func(string, error) {})

	var count int
	for got := range blockReader {
		want, err := BlockHeight1TestMap[got.Height]
		if !err {
			t.Errorf("unknow block: %d", got.Height)
		}

		if want.Txs != got.Txs {
			t.Errorf("values do not match: %v != %v", want.Txs, got.Txs)
		}
		if want.GasUsed != got.GasUsed {
			t.Errorf("values do not match: %v != %v", want.GasUsed, got.GasUsed)
		}
		if want.Time != got.Time {
			t.Errorf("values do not match: %v != %v", want.Time, got.Time)
		}
		if want.ProcessingTime != got.ProcessingTime {
			t.Errorf("values do not match: %s != %s", want.Time, got.Time)
		}

		count++
	}

	if len(BlockHeight1TestMap) != count {
		t.Errorf("not all keys were visited")
	}
}

func TestLogReader_SkipsLinesThatCannotBeParsed(t *testing.T) {
	log := "garbage\n" +
		"INFO [05-04|09:34:15.080] New block index=1 gas_used=11\n" +
		"INFO [05-04|09:34:15.080] " + strings.Repeat("x", 100_000) + "\n" +
		"INFO [05-04|09:34:15.537] New block      index=2 id=3:1:3d6fb6       gas_used=22 txs=20/0 base_fee=2 gas_rate=2.31 age=343.255ms t=1.579ms"

	var skipped []string
	var blocks []Block
	for block := range NewLogReader(strings.NewReader(log), func(line string, err error) {
		skipped = append(skipped, line)
	}) {
		blocks = append(blocks, block)
	}

	if len(skipped) != 1 || !strings.Contains(skipped[0], "index=1") {
		t.Errorf("unexpected skipped lines: %q", skipped)
	}
	if len(blocks) != 1 || blocks[0] != block2 {
		t.Errorf("unexpected blocks: %v, want %v", blocks, block2)
	}
}
