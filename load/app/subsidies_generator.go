// Copyright 2026 Fantom Foundation
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

package app

import (
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"sync/atomic"

	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/0xsoniclabs/sonic/gossip/blockproc/subsidies/registry"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

var subsidiesFund = new(big.Int).Mul(big.NewInt(10_000), big.NewInt(1e18))

// newSubsidiesGenerator creates a generator sending transactions whose gas is paid
// out of an on-chain subsidy rather than by their sender. The senders hold no
// balance at all, so the network has to cover every one of their transactions from
// the sponsorship fund of the called contract.
//
// The generator drives the outcomes through the sponsorship instead of the gas
// limit: a call to the sponsored contract succeeds, and a zero-gas-price call to an
// unsponsored contract has nobody to pay for it and is refused by the network.
func newSubsidiesGenerator(feederId, appId uint32) Generator {
	return &subsidiesGenerator{accountFactory: NewAccountFactory(feederId, appId)}
}

type subsidiesGenerator struct {
	accountFactory *AccountFactory
	abi            *abi.ABI
	// sponsored is the contract whose calls the subsidy covers.
	sponsored common.Address
	// unsponsored is an identical contract that nobody sponsors.
	unsponsored common.Address
	senders     []*Account
	// beggars send the calls to the unsponsored contract. They are kept apart from
	// the senders because such a call may linger in the transaction pool waiting
	// for a sponsorship that never comes, where it would stand in the way of the
	// next transaction of the same account.
	beggars []*Account
	// rejectCounter varies the gas limit of the calls that are meant to be refused
	// so that no two of them share a hash.
	rejectCounter atomic.Uint64
}

func (g *subsidiesGenerator) Deploy(ctxt AppContext, numUsers int) error {
	rules, err := ctxt.GetRules()
	if err != nil {
		return err
	}
	if !rules.Upgrades.Allegro || !rules.Upgrades.GasSubsidies {
		return ErrUnsupported
	}

	client := ctxt.GetClient()

	if g.abi, err = contract.CounterMetaData.GetAbi(); err != nil {
		return err
	}

	_, sponsoredReceipt, err := DeployContract(ctxt, contract.DeployCounter)
	if err != nil {
		return fmt.Errorf("failed to deploy the sponsored Counter contract; %w", err)
	}
	g.sponsored = sponsoredReceipt.ContractAddress

	_, unsponsoredReceipt, err := DeployContract(ctxt, contract.DeployCounter)
	if err != nil {
		return fmt.Errorf("failed to deploy the unsponsored Counter contract; %w", err)
	}
	g.unsponsored = unsponsoredReceipt.ContractAddress

	if err := g.sponsorContract(ctxt); err != nil {
		return err
	}

	// The senders are deliberately left without any balance: all their gas is
	// paid by the subsidy.
	g.senders = make([]*Account, numUsers)
	g.beggars = make([]*Account, numUsers)
	for i := range g.senders {
		sender, err := g.accountFactory.CreateAccount(client)
		if err != nil {
			return err
		}
		beggar, err := g.accountFactory.CreateAccount(client)
		if err != nil {
			return err
		}
		g.senders[i], g.beggars[i] = sender, beggar
	}
	return nil
}

// sponsorContract funds the sponsorship covering all calls to the sponsored
// contract from a dedicated account.
func (g *subsidiesGenerator) sponsorContract(ctxt AppContext) error {
	sponsor, err := g.accountFactory.CreateAccount(ctxt.GetClient())
	if err != nil {
		return err
	}
	if err := ctxt.FundAccounts([]common.Address{sponsor.address}, subsidiesFund); err != nil {
		return fmt.Errorf("failed to fund the sponsor account; %w", err)
	}

	subsidiesRegistry, err := registry.NewRegistry(registry.GetAddress(), ctxt.GetClient())
	if err != nil {
		return fmt.Errorf("failed to bind to the subsidies registry contract; %w", err)
	}
	_, fundId, err := subsidiesRegistry.ContractSponsorshipFundId(nil, g.sponsored)
	if err != nil {
		return fmt.Errorf("failed to read the fund id of the contract; %w", err)
	}

	opts, err := ctxt.GetTransactOptions(sponsor)
	if err != nil {
		return fmt.Errorf("failed to get the transact options for the sponsorship; %w", err)
	}
	// Everything the sponsor holds goes into the fund, less the gas of this call.
	opts.Value = new(big.Int).Sub(subsidiesFund, new(big.Int).Mul(opts.GasPrice, big.NewInt(150_000)))

	tx, err := subsidiesRegistry.Sponsor(opts, fundId)
	if err != nil {
		return fmt.Errorf("failed to sponsor the contract; %w", err)
	}
	receipt, err := ctxt.GetReceipt(tx.Hash())
	if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
		return errors.Join(fmt.Errorf("the sponsorship transaction failed"), err)
	}
	return nil
}

func (g *subsidiesGenerator) Call(user int) (Call, error) {
	if user < 0 || user >= len(g.senders) {
		return Call{}, fmt.Errorf("no account for user %d, the generator serves %d users", user, len(g.senders))
	}
	data, err := g.abi.Pack("incrementCounter")
	if err != nil {
		return Call{}, fmt.Errorf("failed to pack incrementCounter; %w", err)
	}

	// One in twenty calls targets the unsponsored contract, which leaves nobody to
	// pay for it. It never reaches a block, so its sender stays at nonce 0 and the
	// same call can be repeated for as long as the load runs. The gas limit is
	// varied so that no two of these calls share a hash.
	if rand.Intn(20) == 0 {
		beggar := g.beggars[user]
		tx, err := g.sign(beggar, g.unsponsored, data, 28_036+g.rejectCounter.Add(1), 0)
		if err != nil {
			return Call{}, err
		}
		return Call{Tx: tx, Outcome: Rejected}, nil
	}

	from := g.senders[user]
	tx, err := g.sign(from, g.sponsored, data, 28_036, from.getNextNonce())
	if err != nil {
		return Call{}, err
	}
	return Call{Tx: tx, Outcome: Success}, nil
}

// sign builds a transaction offering no fee at all, which is how a sender asks the
// network to cover its gas out of the subsidy of the called contract.
func (g *subsidiesGenerator) sign(from *Account, to common.Address, data []byte, gasLimit, nonce uint64) (*types.Transaction, error) {
	tx := types.NewTx(&types.DynamicFeeTx{
		Nonce:     nonce,
		GasFeeCap: big.NewInt(0),
		GasTipCap: big.NewInt(0),
		Gas:       gasLimit,
		To:        &to,
		Value:     big.NewInt(0),
		Data:      data,
	})
	return types.SignTx(tx, types.NewLondonSigner(from.chainID), from.privateKey)
}

func (g *subsidiesGenerator) Check(ctxt AppContext, call Call, receipt *types.Receipt) error {
	return CheckOutcome(ctxt, call, receipt)
}
