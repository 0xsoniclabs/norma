// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package abi

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// ProbabilisticFailingMetaData contains all meta data concerning the ProbabilisticFailing contract.
var ProbabilisticFailingMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"getCount\",\"outputs\":[{\"internalType\":\"int256\",\"name\":\"\",\"type\":\"int256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint8\",\"name\":\"failureProbability\",\"type\":\"uint8\"},{\"internalType\":\"uint32\",\"name\":\"seed\",\"type\":\"uint32\"}],\"name\":\"incrementCounter\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
	Bin: "0x60806040526001600055348015601457600080fd5b50610257806100246000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c806319dde1691461003b578063a87d942c14610050575b600080fd5b61004e610049366004610152565b61006a565b005b61005861013c565b60405190815260200160405180910390f35b600080546040516bffffffffffffffffffffffff193360601b16602082015260348101919091524260548201526001600160e01b031960e084901b16607482015260780160408051601f198184030181529190528051602090910120905060ff83166100d760648361019a565b10156101205760405162461bcd60e51b8152602060048201526014602482015273141c9bd898589a5b1a5cdd1a58c81c995d995c9d60621b604482015260640160405180910390fd5b600160008082825461013291906101d2565b9091555050505050565b6000600160005461014d91906101fa565b905090565b6000806040838503121561016557600080fd5b823560ff8116811461017657600080fd5b9150602083013563ffffffff8116811461018f57600080fd5b809150509250929050565b6000826101b757634e487b7160e01b600052601260045260246000fd5b500690565b634e487b7160e01b600052601160045260246000fd5b80820182811260008312801582168215821617156101f2576101f26101bc565b505092915050565b818103600083128015838313168383128216171561021a5761021a6101bc565b509291505056fea26469706673582212202f43548103680a5d9716504a1450dbece641976702e5b3c616621c8b77491d0f64736f6c634300081d0033",
}

// ProbabilisticFailingABI is the input ABI used to generate the binding from.
// Deprecated: Use ProbabilisticFailingMetaData.ABI instead.
var ProbabilisticFailingABI = ProbabilisticFailingMetaData.ABI

// ProbabilisticFailingBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use ProbabilisticFailingMetaData.Bin instead.
var ProbabilisticFailingBin = ProbabilisticFailingMetaData.Bin

// DeployProbabilisticFailing deploys a new Ethereum contract, binding an instance of ProbabilisticFailing to it.
func DeployProbabilisticFailing(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *ProbabilisticFailing, error) {
	parsed, err := ProbabilisticFailingMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(ProbabilisticFailingBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &ProbabilisticFailing{ProbabilisticFailingCaller: ProbabilisticFailingCaller{contract: contract}, ProbabilisticFailingTransactor: ProbabilisticFailingTransactor{contract: contract}, ProbabilisticFailingFilterer: ProbabilisticFailingFilterer{contract: contract}}, nil
}

// ProbabilisticFailing is an auto generated Go binding around an Ethereum contract.
type ProbabilisticFailing struct {
	ProbabilisticFailingCaller     // Read-only binding to the contract
	ProbabilisticFailingTransactor // Write-only binding to the contract
	ProbabilisticFailingFilterer   // Log filterer for contract events
}

// ProbabilisticFailingCaller is an auto generated read-only Go binding around an Ethereum contract.
type ProbabilisticFailingCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ProbabilisticFailingTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ProbabilisticFailingTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ProbabilisticFailingFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ProbabilisticFailingFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ProbabilisticFailingSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ProbabilisticFailingSession struct {
	Contract     *ProbabilisticFailing // Generic contract binding to set the session for
	CallOpts     bind.CallOpts         // Call options to use throughout this session
	TransactOpts bind.TransactOpts     // Transaction auth options to use throughout this session
}

// ProbabilisticFailingCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ProbabilisticFailingCallerSession struct {
	Contract *ProbabilisticFailingCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts               // Call options to use throughout this session
}

// ProbabilisticFailingTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ProbabilisticFailingTransactorSession struct {
	Contract     *ProbabilisticFailingTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts               // Transaction auth options to use throughout this session
}

// ProbabilisticFailingRaw is an auto generated low-level Go binding around an Ethereum contract.
type ProbabilisticFailingRaw struct {
	Contract *ProbabilisticFailing // Generic contract binding to access the raw methods on
}

// ProbabilisticFailingCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ProbabilisticFailingCallerRaw struct {
	Contract *ProbabilisticFailingCaller // Generic read-only contract binding to access the raw methods on
}

// ProbabilisticFailingTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ProbabilisticFailingTransactorRaw struct {
	Contract *ProbabilisticFailingTransactor // Generic write-only contract binding to access the raw methods on
}

// NewProbabilisticFailing creates a new instance of ProbabilisticFailing, bound to a specific deployed contract.
func NewProbabilisticFailing(address common.Address, backend bind.ContractBackend) (*ProbabilisticFailing, error) {
	contract, err := bindProbabilisticFailing(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &ProbabilisticFailing{ProbabilisticFailingCaller: ProbabilisticFailingCaller{contract: contract}, ProbabilisticFailingTransactor: ProbabilisticFailingTransactor{contract: contract}, ProbabilisticFailingFilterer: ProbabilisticFailingFilterer{contract: contract}}, nil
}

// NewProbabilisticFailingCaller creates a new read-only instance of ProbabilisticFailing, bound to a specific deployed contract.
func NewProbabilisticFailingCaller(address common.Address, caller bind.ContractCaller) (*ProbabilisticFailingCaller, error) {
	contract, err := bindProbabilisticFailing(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ProbabilisticFailingCaller{contract: contract}, nil
}

// NewProbabilisticFailingTransactor creates a new write-only instance of ProbabilisticFailing, bound to a specific deployed contract.
func NewProbabilisticFailingTransactor(address common.Address, transactor bind.ContractTransactor) (*ProbabilisticFailingTransactor, error) {
	contract, err := bindProbabilisticFailing(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ProbabilisticFailingTransactor{contract: contract}, nil
}

// NewProbabilisticFailingFilterer creates a new log filterer instance of ProbabilisticFailing, bound to a specific deployed contract.
func NewProbabilisticFailingFilterer(address common.Address, filterer bind.ContractFilterer) (*ProbabilisticFailingFilterer, error) {
	contract, err := bindProbabilisticFailing(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ProbabilisticFailingFilterer{contract: contract}, nil
}

// bindProbabilisticFailing binds a generic wrapper to an already deployed contract.
func bindProbabilisticFailing(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := ProbabilisticFailingMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ProbabilisticFailing *ProbabilisticFailingRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ProbabilisticFailing.Contract.ProbabilisticFailingCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ProbabilisticFailing *ProbabilisticFailingRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ProbabilisticFailing.Contract.ProbabilisticFailingTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ProbabilisticFailing *ProbabilisticFailingRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ProbabilisticFailing.Contract.ProbabilisticFailingTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ProbabilisticFailing *ProbabilisticFailingCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ProbabilisticFailing.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ProbabilisticFailing *ProbabilisticFailingTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ProbabilisticFailing.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ProbabilisticFailing *ProbabilisticFailingTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ProbabilisticFailing.Contract.contract.Transact(opts, method, params...)
}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(int256)
func (_ProbabilisticFailing *ProbabilisticFailingCaller) GetCount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _ProbabilisticFailing.contract.Call(opts, &out, "getCount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(int256)
func (_ProbabilisticFailing *ProbabilisticFailingSession) GetCount() (*big.Int, error) {
	return _ProbabilisticFailing.Contract.GetCount(&_ProbabilisticFailing.CallOpts)
}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(int256)
func (_ProbabilisticFailing *ProbabilisticFailingCallerSession) GetCount() (*big.Int, error) {
	return _ProbabilisticFailing.Contract.GetCount(&_ProbabilisticFailing.CallOpts)
}

// IncrementCounter is a paid mutator transaction binding the contract method 0x19dde169.
//
// Solidity: function incrementCounter(uint8 failureProbability, uint32 seed) returns()
func (_ProbabilisticFailing *ProbabilisticFailingTransactor) IncrementCounter(opts *bind.TransactOpts, failureProbability uint8, seed uint32) (*types.Transaction, error) {
	return _ProbabilisticFailing.contract.Transact(opts, "incrementCounter", failureProbability, seed)
}

// IncrementCounter is a paid mutator transaction binding the contract method 0x19dde169.
//
// Solidity: function incrementCounter(uint8 failureProbability, uint32 seed) returns()
func (_ProbabilisticFailing *ProbabilisticFailingSession) IncrementCounter(failureProbability uint8, seed uint32) (*types.Transaction, error) {
	return _ProbabilisticFailing.Contract.IncrementCounter(&_ProbabilisticFailing.TransactOpts, failureProbability, seed)
}

// IncrementCounter is a paid mutator transaction binding the contract method 0x19dde169.
//
// Solidity: function incrementCounter(uint8 failureProbability, uint32 seed) returns()
func (_ProbabilisticFailing *ProbabilisticFailingTransactorSession) IncrementCounter(failureProbability uint8, seed uint32) (*types.Transaction, error) {
	return _ProbabilisticFailing.Contract.IncrementCounter(&_ProbabilisticFailing.TransactOpts, failureProbability, seed)
}
