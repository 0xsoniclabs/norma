package app

import (
	"context"
	"fmt"
	"github.com/0xsoniclabs/norma/driver/rpc"
	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
	"math/big"
	"math/rand"
	"sync/atomic"
)

// NewSmartAccountApplication deploys a new SmartAccount dapp to the chain.
func NewSmartAccountApplication(context AppContext, feederId, appId uint32) (Application, error) {
	rpcClient := context.GetClient()
	primaryAccount := context.GetTreasure()

	txOpts, err := context.GetTransactOptions(primaryAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to create txOpts for contract deploy; %w", err)
	}

	// Deploy SmartAccount impl
	smartAccountImplAddress, tx, _, err := contract.DeploySmartAccount(txOpts, rpcClient)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy SmartAccount impl; %w", err)
	}
	deployments := []*types.Transaction{tx}

	// wait until contracts are available on the chain
	for i, tx := range deployments {
		receipt, err := context.GetReceipt(tx.Hash())
		if err != nil {
			return nil, fmt.Errorf("failed to wait until the SmartAccount contract is deployed; %w", err)
		}
		if receipt.Status != types.ReceiptStatusSuccessful {
			return nil, fmt.Errorf("failed to deploy SmartAccount contract; transaction reverted; step %d", i)
		}
	}

	recipients, err := generateRecipientsAddresses()
	if err != nil {
		return nil, fmt.Errorf("failed to generate recipients addresses; %w", err)
	}

	accountFactory, err := NewAccountFactory(primaryAccount.chainID, feederId, appId)
	if err != nil {
		return nil, err
	}

	// parse ABI for generating txs data
	smartAccountAbi, err := contract.SmartAccountMetaData.GetAbi()
	if err != nil {
		return nil, err
	}

	return &SmartAccountApplication{
		smartAccountAbi:         smartAccountAbi,
		smartAccountImplAddress: smartAccountImplAddress,
		recipients:              recipients,
		accountFactory:          accountFactory,
	}, nil
}

// SmartAccountApplication represents one application deployed to the network - a SmartAccount implementation contract.
// Each created app should be used in a single thread only.
type SmartAccountApplication struct {
	smartAccountAbi         *abi.ABI
	smartAccountImplAddress common.Address
	recipients              []common.Address
	accountFactory          *AccountFactory
}

// CreateUsers creates a list of new users for the app.
func (f *SmartAccountApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {

	// Create a list of users.
	users := make([]User, numUsers)
	addresses := make([]common.Address, numUsers)
	for i := 0; i < numUsers; i++ {
		// Generate a new account for each worker - avoid account nonces related bottlenecks
		workerAccount, err := f.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		users[i] = &SmartAccountUser{
			abi:        f.smartAccountAbi,
			sender:     workerAccount,
			recipients: f.recipients,
		}
		addresses[i] = workerAccount.address
	}

	// Provide native currency to each user.
	fundsPerUser := big.NewInt(1_000)
	fundsPerUser = new(big.Int).Mul(fundsPerUser, big.NewInt(1_000_000_000_000_000_000)) // to wei
	err := appContext.FundAccounts(addresses, fundsPerUser)
	if err != nil {
		return nil, fmt.Errorf("failed to fund accounts; %w", err)
	}

	// SmartAccount into the user address
	authList := make([]types.SetCodeAuthorization, numUsers)
	for _, user := range users {
		auth := types.SetCodeAuthorization{
			ChainID: *uint256.MustFromBig(user.(*SmartAccountUser).sender.chainID),
			Address: f.smartAccountImplAddress,
			Nonce:   user.(*SmartAccountUser).sender.getNextNonce(),
		}
		auth, err = types.SignSetCode(user.(*SmartAccountUser).sender.privateKey, auth)
		if err != nil {
			return nil, fmt.Errorf("failed to sign SetCodeAuthorization; %w", err)
		}
		authList = append(authList, auth)
	}

	const gasLimit = 520000 // TODO
	receipt, err := appContext.Run(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		tx := types.NewTx(&types.SetCodeTx{
			Nonce:     opts.Nonce.Uint64(),
			GasFeeCap: new(uint256.Int).Mul(uint256.NewInt(10_000), uint256.NewInt(1e9)),
			GasTipCap: uint256.NewInt(0),
			Gas:       gasLimit,
			To:        appContext.GetTreasure().address,
			Value:     uint256.NewInt(0),
			Data:      nil,
			AuthList:  authList,
		})
		tx, err = types.SignTx(tx, types.NewPragueSigner(appContext.GetTreasure().chainID), appContext.GetTreasure().privateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to sign SetCodeTx; %w", err)
		}
		err = appContext.GetClient().SendTransaction(context.Background(), tx)
		if err != nil {
			return nil, fmt.Errorf("failed to send SetCodeTx; %w", err)
		}
		return tx, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to run SetCodeTx; %w", err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return nil, fmt.Errorf("failed to run SetCodeTx; transaction reverted")
	}

	fmt.Printf("SetCodeTx sucessfully\n") // TODO remove

	return users, nil
}

func (f *SmartAccountApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	totalReceived := uint64(0)
	for _, recipient := range f.recipients {
		recipientBalance, err := rpcClient.BalanceAt(context.Background(), recipient, nil)
		if err != nil {
			return 0, err
		}
		totalReceived += recipientBalance.Uint64()
	}
	return totalReceived, nil
}

// SmartAccountUser represents a user sending txs to transfer SmartAccount tokens.
// A generator is supposed to be used in a single thread.
type SmartAccountUser struct {
	abi        *abi.ABI
	sender     *Account
	recipients []common.Address
	sentTxs    uint64
}

func (g *SmartAccountUser) GenerateTx() (*types.Transaction, error) {
	// choose random recipient
	recipient := g.recipients[rand.Intn(len(g.recipients))]

	// prepare tx data
	calls := []contract.SmartAccountCall{
		{
			To:    recipient,
			Value: big.NewInt(1),
		},
	}
	smartNonce := new(big.Int).SetUint64(atomic.LoadUint64(&g.sentTxs))
	data, err := g.abi.Pack("execute", calls, smartNonce)
	if err != nil || data == nil {
		return nil, fmt.Errorf("failed to prepare tx data; %w", err)
	}

	// prepare tx
	const gasLimit = 52000 // TODO
	tx, err := createTx(g.sender, g.sender.address, big.NewInt(0), data, gasLimit)
	if err == nil {
		atomic.AddUint64(&g.sentTxs, 1)
	}
	return tx, err
}

func (g *SmartAccountUser) GetSentTransactions() uint64 {
	return atomic.LoadUint64(&g.sentTxs)
}
