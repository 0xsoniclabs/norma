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

package steps

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"gopkg.in/yaml.v3"
)

// WaitForBlocks waits until the network has produced a number of new blocks.
type WaitForBlocks struct {
	Blocks uint64
}

func (*WaitForBlocks) Name() string { return "waitForBlocks" }

func (*WaitForBlocks) Help() string {
	return "Wait until the network has produced the given number of new blocks."
}

func (s *WaitForBlocks) DecodeValue(value *yaml.Node) error {
	// The tag check rejects floats, which Decode would truncate.
	if value.Kind != yaml.ScalarNode || value.Tag != "!!int" {
		return fmt.Errorf("waitForBlocks requires a block count, got %q", value.Value)
	}
	if err := value.Decode(&s.Blocks); err != nil {
		return fmt.Errorf("invalid block count %q: %w", value.Value, err)
	}
	return nil
}

func (s *WaitForBlocks) Validate() error {
	if s.Blocks == 0 {
		return errors.New("waitForBlocks requires a positive block count")
	}
	return nil
}

func (s *WaitForBlocks) Run(ctx context.Context, env *Env) error {
	client, err := env.Net.DialRandomRpc()
	if err != nil {
		return fmt.Errorf("failed to connect to network: %w", err)
	}
	defer client.Close()
	slog.Info("waiting for blocks", "count", s.Blocks)
	return waitForBlocks(ctx, client, s.Blocks)
}
