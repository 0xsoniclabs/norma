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
	"fmt"
	"math/big"

	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
)

const smartAccountAuthorizations = 3

// newSmartAccountGenerator creates a generator sending EIP-7702 set-code
// transactions: every transaction delegates a set of accounts to a smart account
// implementation and has an entry point drive one of them into a counter call.
func newSmartAccountGenerator(feederId, appId uint32) Generator {
	g := &smartAccountGenerator{txGenerator: newTxGenerator(feederId, appId)}
	g.supports = func(rules opera.Rules) bool { return rules.Upgrades.Allegro }
	g.onDeploy = g.deploy
	g.onAccounts = g.createDelegatePools
	g.onSuccess = g.handleOps
	g.onSign = g.signSetCodeTx
	return g
}

type smartAccountGenerator struct {
	txGenerator
	smartAccountAbi *abi.ABI
	entryPointAbi   *abi.ABI
	counterAbi      *abi.ABI
	implAddress     common.Address
	entryPoint      common.Address
	counter         common.Address
	// delegates holds the accounts each user delegates, one pool per user.
	delegates []*AccountsCircularPool
}

func (g *smartAccountGenerator) deploy(ctxt AppContext) error {
	rpcClient := ctxt.GetClient()

	txOpts, err := ctxt.GetTransactOptions(ctxt.GetTreasure())
	if err != nil {
		return fmt.Errorf("failed to create the transact options for the deployment; %w", err)
	}
	nextNonce := func() {
		txOpts.Nonce = new(big.Int).Add(txOpts.Nonce, big.NewInt(1))
	}

	implAddress, implTx, _, err := contract.DeploySmartAccount(txOpts, rpcClient)
	if err != nil {
		return fmt.Errorf("failed to deploy the SmartAccount implementation; %w", err)
	}

	nextNonce()
	entryPoint, entryPointTx, _, err := contract.DeployEntryPoint(txOpts, rpcClient)
	if err != nil {
		return fmt.Errorf("failed to deploy the EntryPoint; %w", err)
	}

	nextNonce()
	counter, counterTx, _, err := contract.DeployCounter(txOpts, rpcClient)
	if err != nil {
		return fmt.Errorf("failed to deploy the Counter contract; %w", err)
	}

	if err := awaitReceipts(ctxt, []*types.Transaction{implTx, entryPointTx, counterTx}); err != nil {
		return fmt.Errorf("failed to deploy the SmartAccount contracts; %w", err)
	}
	g.implAddress, g.entryPoint, g.counter = implAddress, entryPoint, counter

	if g.smartAccountAbi, err = contract.SmartAccountMetaData.GetAbi(); err != nil {
		return err
	}
	if g.entryPointAbi, err = contract.EntryPointMetaData.GetAbi(); err != nil {
		return err
	}
	g.counterAbi, err = contract.CounterMetaData.GetAbi()
	return err
}

// createDelegatePools gives every user its own pool of accounts to delegate. A
// pool is large enough that an account is not delegated again before the previous
// delegation has been processed.
func (g *smartAccountGenerator) createDelegatePools(ctxt AppContext, accounts []common.Address) error {
	const poolSize = 1000
	g.delegates = make([]*AccountsCircularPool, len(accounts))
	for i := range g.delegates {
		pool, err := NewAccountsCircularPool(g.accountFactory, ctxt.GetClient(), poolSize)
		if err != nil {
			return fmt.Errorf("failed to create the delegate pool of user %d; %w", i, err)
		}
		g.delegates[i] = pool
	}
	return nil
}

// handleOps builds the call the entry point receives. The payload is turned into a
// set-code transaction by signSetCodeTx.
func (g *smartAccountGenerator) handleOps(user int, _ *Account) (txPayload, error) {
	delegates, err := g.delegates[user].GetAccounts(smartAccountAuthorizations)
	if err != nil {
		return txPayload{}, err
	}

	incrementData, err := g.counterAbi.Pack("incrementCounter")
	if err != nil {
		return txPayload{}, fmt.Errorf("failed to pack incrementCounter; %w", err)
	}

	accountData, err := g.smartAccountAbi.Pack("execute", []contract.SmartAccountCall{{
		To:    g.counter,
		Value: new(big.Int),
		Data:  incrementData,
	}})
	if err != nil {
		return txPayload{}, fmt.Errorf("failed to pack the SmartAccount call data; %w", err)
	}

	entryPointData, err := g.entryPointAbi.Pack("handleOps", []contract.EntryPointPackedUserOperation{{
		Sender:   delegates[0].address,
		CallData: accountData,
	}})
	if err != nil {
		return txPayload{}, fmt.Errorf("failed to pack the EntryPoint call data; %w", err)
	}

	return txPayload{
		to:         &g.entryPoint,
		data:       entryPointData,
		gasLimit:   200_000,
		delegators: delegates,
	}, nil
}

// signSetCodeTx signs the payload as an EIP-7702 set-code transaction that
// delegates every account of the payload to the smart account implementation.
func (g *smartAccountGenerator) signSetCodeTx(payload txPayload, from *Account, nonce uint64) (*types.Transaction, error) {
	authorizations := make([]types.SetCodeAuthorization, 0, len(payload.delegators))
	for _, delegator := range payload.delegators {
		authorization, err := types.SignSetCode(delegator.privateKey, types.SetCodeAuthorization{
			ChainID: *uint256.MustFromBig(delegator.chainID),
			Address: g.implAddress,
			Nonce:   delegator.getNextNonce(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to sign the set-code authorization; %w", err)
		}
		authorizations = append(authorizations, authorization)
	}

	value := payload.value
	if value == nil {
		value = big.NewInt(0)
	}
	tx := types.NewTx(&types.SetCodeTx{
		Nonce:     nonce,
		GasFeeCap: uint256.MustFromBig(gasFeeCap),
		GasTipCap: uint256.MustFromBig(gasTipCap),
		Gas:       payload.gasLimit,
		To:        *payload.to,
		Value:     uint256.MustFromBig(value),
		Data:      payload.data,
		AuthList:  authorizations,
	})
	return types.SignTx(tx, types.NewPragueSigner(from.chainID), from.privateKey)
}
