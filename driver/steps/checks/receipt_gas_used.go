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

package checks

import (
	"context"
	"errors"

	"github.com/0xsoniclabs/norma/driver/checking"
)

// ReceiptGasUsed asserts that the amended nodes correct the receipt gas leaked
// by failed bare-root bundles, which the leaked nodes still report.
type ReceiptGasUsed struct {
	Amended     []string `yaml:"amended" help:"Required. Labels of the nodes correcting the leaked receipt gas."`
	Leaked      []string `yaml:"leaked" help:"Required. Labels of the nodes reporting the leaked receipt gas."`
	ExportEpoch uint64   `yaml:"exportEpoch" help:"Required. Epoch the exporter of the g-file stopped at; the groups must differ on a block of an earlier one."`
	Failing
}

func (*ReceiptGasUsed) Name() string { return "receiptGasUsed" }

func (*ReceiptGasUsed) Help() string {
	return "Assert that the amended nodes agree on the GasUsed of all receipts, the leaked nodes too, " +
		"and that the two groups differ on at least one in a block before the export epoch."
}

func (c *ReceiptGasUsed) Validate() error {
	if len(c.Amended) == 0 || len(c.Leaked) == 0 {
		return errors.New("requires 'amended' and 'leaked' node lists")
	}
	if c.ExportEpoch == 0 {
		return errors.New("requires a positive 'exportEpoch'")
	}
	return nil
}

func (c *ReceiptGasUsed) Run(ctx context.Context, checks checking.Checks) error {
	config := newConfig(c.Failing)
	config["amended"] = c.Amended
	config["leaked"] = c.Leaked
	config["exportEpoch"] = c.ExportEpoch
	return runChecker(ctx, checks, "receiptGasUsed", config)
}
