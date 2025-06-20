package app

import (
	"fmt"
	"github.com/0xsoniclabs/norma/driver/rpc"
	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
	"math/big"
	"sync/atomic"
	"time"
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

	txOpts.Nonce = new(big.Int).Add(txOpts.Nonce, big.NewInt(1))
	counterAddress, tx, _, err := contract.DeployCounter(txOpts, rpcClient)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy Counter contract; %w", err)
	}
	deployments = append(deployments, tx)

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

	accountFactory, err := NewAccountFactory(primaryAccount.chainID, feederId, appId)
	if err != nil {
		return nil, err
	}

	// parse ABI for generating txs data
	smartAccountAbi, err := contract.SmartAccountMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	counterAbi, err := contract.CounterMetaData.GetAbi()
	if err != nil {
		return nil, err
	}

	return &SmartAccountApplication{
		smartAccountAbi:         smartAccountAbi,
		counterAbi:              counterAbi,
		smartAccountImplAddress: smartAccountImplAddress,
		accountFactory:          accountFactory,
		counterAddress:          counterAddress,
	}, nil
}

// SmartAccountApplication represents one application deployed to the network - a SmartAccount implementation contract.
// Each created app should be used in a single thread only.
type SmartAccountApplication struct {
	smartAccountAbi         *abi.ABI
	counterAbi              *abi.ABI
	smartAccountImplAddress common.Address
	accountFactory          *AccountFactory
	counterAddress          common.Address
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
		accountsCircular, err := NewAccountsCircular(f.accountFactory, appContext.GetClient(), 1000)
		if err != nil {
			return nil, err
		}
		users[i] = &SmartAccountUser{
			smartAccountAbi:  f.smartAccountAbi,
			counterAbi:       f.counterAbi,
			sender:           workerAccount,
			counterAddr:      f.counterAddress,
			codeAddr:         f.smartAccountImplAddress,
			accountsCircular: accountsCircular,
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
	return users, nil
}

func (f *SmartAccountApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	counterContract, err := contract.NewCounter(f.counterAddress, rpcClient)
	if err != nil {
		return 0, fmt.Errorf("failed to get Counter contract representation; %w", err)
	}
	count, err := counterContract.GetCount(nil)
	if err != nil {
		return 0, err
	}
	return count.Uint64(), nil
}

// SmartAccountUser represents a user sending txs to transfer SmartAccount tokens.
// A generator is supposed to be used in a single thread.
type SmartAccountUser struct {
	smartAccountAbi  *abi.ABI
	counterAbi       *abi.ABI
	sender           *Account
	counterAddr      common.Address
	codeAddr         common.Address
	accountsCircular *AccountsCircular
	sentTxs          uint64
}

func (g *SmartAccountUser) GenerateTx() (*types.Transaction, error) {
	time.Sleep(3 * time.Second)

	// choose random recipients
	authAccounts, err := g.accountsCircular.GetAccounts(3)
	if err != nil {
		return nil, err
	}

	dataIncrement, err := g.counterAbi.Pack("incrementCounter")
	if err != nil || dataIncrement == nil {
		return nil, fmt.Errorf("failed to prepare increment user op data; %w", err)
	}

	// prepare tx data
	calls := []contract.SmartAccountCall{
		{
			To:    g.counterAddr,
			Value: new(big.Int),
			Data:  dataIncrement,
		},
	}
	data, err := g.smartAccountAbi.Pack("execute", calls)
	if err != nil || data == nil {
		return nil, fmt.Errorf("failed to prepare tx data; %w", err)
	}

	// prepare tx
	const gasLimit = 200_000
	tx, err := createSetCodeTx(g.sender, authAccounts[0].address, new(uint256.Int), data, gasLimit, authAccounts, g.codeAddr)
	if err == nil {
		atomic.AddUint64(&g.sentTxs, 1)
	}
	return tx, err
}

func (g *SmartAccountUser) GetSentTransactions() uint64 {
	return atomic.LoadUint64(&g.sentTxs)
}
