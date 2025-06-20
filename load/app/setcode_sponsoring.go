package app

import (
	"context"
	"fmt"
	"math/big"

	"github.com/0xsoniclabs/norma/driver/rpc"
	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
)

// SetCodeApp is a test application demonstrating the use of
// EIP-7702 SetCode transaction type.
// https://eips.ethereum.org/EIPS/eip-7702
type SetCodeApp struct {
	accountFactory  *AccountFactory
	contract        *contract.Counter
	contractAddress common.Address
	rpcClient       rpc.Client
}

func NewSponsoring(context AppContext, feederId, appId uint32) (Application, error) {
	rpcClient := context.GetClient()
	primaryAccount := context.GetTreasure()

	accountFactory, err := NewAccountFactory(primaryAccount.chainID, feederId, appId)
	if err != nil {
		return nil, err
	}

	// Deploy the Counter contract to be used by this application.
	contract, receipt, err := DeployContract(context, contract.DeployCounter)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy Counter contract; %w", err)
	}

	return &SetCodeApp{
		rpcClient:       rpcClient,
		contract:        contract,
		contractAddress: receipt.ContractAddress,
		accountFactory:  accountFactory,
	}, nil
}

func (sca *SetCodeApp) CreateUsers(context AppContext, numUsers int) ([]User, error) {

	sponsorAccount, err := sca.accountFactory.CreateAccount(sca.rpcClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create sponsor account: %v", err)
	}

	fundsPerUser := big.NewInt(1_000)
	fundsPerUser = new(big.Int).Mul(fundsPerUser, big.NewInt(1_000_000_000_000_000_000)) // to wei
	err = context.FundAccounts([]common.Address{sponsorAccount.address}, fundsPerUser)
	if err != nil {
		return nil, fmt.Errorf("failed to fund sponsor account; %w", err)
	}

	ops, err := context.GetTransactOptions(sponsorAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction options: %w", err)
	}
	tmpTx, err := sca.contract.IncrementCounter(ops)
	if err != nil {
		return nil, fmt.Errorf("failed to create contract call transaction: %w", err)
	}

	users := make([]User, numUsers)
	for i := range numUsers {
		account, err := sca.accountFactory.CreateAccount(sca.rpcClient)
		if err != nil {
			return nil, fmt.Errorf("failed to create account: %v", err)
		}

		users[i] = &SetCodeUser{
			client:          sca.rpcClient,
			account:         account,
			contractAddress: sca.contractAddress,
			// extract contract call ABI from contract call transaction
			callData: tmpTx.Data(),
			sponsor:  sponsorAccount,
		}
	}

	return users, nil
}

func (*SetCodeApp) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	// TODO: make something meaningful here
	return 0, nil
}

type SetCodeUser struct {
	client          rpc.Client
	account         *Account
	sponsor         *Account
	contractAddress common.Address
	callData        []byte
	txCount         uint64
}

// GenerateTx for the SetCodeApp  generates transactions in the following sequence:
// 1. Self-sponsor an authorization to install a delegation designator in the local account.
// 2. Issue a number of calls to itself to execute such code in the local account.
// 3. Self-sponsor an authorization to remove the delegation designator in the local account.
func (u *SetCodeUser) GenerateTx() (*types.Transaction, error) {
	defer func() { u.txCount++ }()

	switch u.txCount % 6 {
	case 0:
		return u.GenerateInstallDelegationTx(u.contractAddress)
	case 1, 2, 3, 4:
		return u.CallDelegatedContract()
	case 5:
		return u.GenerateInstallDelegationTx(common.Address{})
	default:
		return nil, fmt.Errorf("logic error, invalid transaction count %d", u.txCount)
	}
}

func (u *SetCodeUser) GenerateInstallDelegationTx(delegator common.Address) (*types.Transaction, error) {

	nonce, err := u.client.NonceAt(context.Background(),
		u.account.address, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce: %v", err)
	}

	signer := types.NewPragueSigner(u.account.chainID)
	auth, err := types.SignSetCode(u.account.privateKey,
		types.SetCodeAuthorization{
			Address: delegator,
			Nonce:   nonce + 1,
			ChainID: *uint256.MustFromBig(u.account.chainID),
		})
	if err != nil {
		return nil, fmt.Errorf("failed to sign set code authorization: %v", err)
	}

	tx, err := types.SignNewTx(u.sponsor.privateKey, signer,
		&types.SetCodeTx{
			To:        u.account.address,
			GasFeeCap: new(uint256.Int).Mul(uint256.NewInt(10_000), uint256.NewInt(1e9)),
			Nonce:     u.sponsor.getNextNonce(),
			Gas:       60_000,
			AuthList:  []types.SetCodeAuthorization{auth},
		})
	if err != nil {
		return nil, fmt.Errorf("failed to sign set code transaction: %v", err)
	}
	return tx, nil
}

func (u *SetCodeUser) CallDelegatedContract() (*types.Transaction, error) {
	signer := types.NewPragueSigner(u.account.chainID)

	tx, err := types.SignNewTx(u.sponsor.privateKey, signer,
		&types.LegacyTx{
			To:       &u.account.address,
			GasPrice: new(big.Int).Mul(big.NewInt(10_000), big.NewInt(1e9)),
			Gas:      60_000,
			Nonce:    u.sponsor.getNextNonce(),
			Data:     u.callData,
		})
	if err != nil {
		return nil, fmt.Errorf("failed to sign call transaction: %v", err)
	}
	return tx, nil
}

func (u *SetCodeUser) GetSentTransactions() uint64 {
	return u.txCount
}
