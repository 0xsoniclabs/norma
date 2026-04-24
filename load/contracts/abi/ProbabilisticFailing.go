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
	ABI: "[{\"inputs\":[],\"name\":\"getCount\",\"outputs\":[{\"internalType\":\"int256\",\"name\":\"\",\"type\":\"int256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint8\",\"name\":\"failureProbability\",\"type\":\"uint8\"}],\"name\":\"incrementCounter\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
	Bin: "0x60806040526001600055348015601457600080fd5b50610223806100246000396000f3fe608060405234801561001057600080fd5b50600436106100355760003560e01c80627554551461003a578063a87d942c1461004f575b600080fd5b61004d61004836600461013c565b610069565b005b610057610126565b60405190815260200160405180910390f35b600080546040516bffffffffffffffffffffffff193360601b166020820152603481019190915242605482015260740160408051601f198184030181529190528051602090910120905060ff82166100c2606483610166565b101561010b5760405162461bcd60e51b8152602060048201526014602482015273141c9bd898589a5b1a5cdd1a58c81c995d995c9d60621b604482015260640160405180910390fd5b600160008082825461011d919061019e565b90915550505050565b6000600160005461013791906101c6565b905090565b60006020828403121561014e57600080fd5b813560ff8116811461015f57600080fd5b9392505050565b60008261018357634e487b7160e01b600052601260045260246000fd5b500690565b634e487b7160e01b600052601160045260246000fd5b80820182811260008312801582168215821617156101be576101be610188565b505092915050565b81810360008312801583831316838312821617156101e6576101e6610188565b509291505056fea2646970667358221220b801152c3af77199c5450d75941f5f1f72d2425985ad4da068c1796435106e9864736f6c634300081d0033",
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

// IncrementCounter is a paid mutator transaction binding the contract method 0x00755455.
//
// Solidity: function incrementCounter(uint8 failureProbability) returns()
func (_ProbabilisticFailing *ProbabilisticFailingTransactor) IncrementCounter(opts *bind.TransactOpts, failureProbability uint8) (*types.Transaction, error) {
	return _ProbabilisticFailing.contract.Transact(opts, "incrementCounter", failureProbability)
}

// IncrementCounter is a paid mutator transaction binding the contract method 0x00755455.
//
// Solidity: function incrementCounter(uint8 failureProbability) returns()
func (_ProbabilisticFailing *ProbabilisticFailingSession) IncrementCounter(failureProbability uint8) (*types.Transaction, error) {
	return _ProbabilisticFailing.Contract.IncrementCounter(&_ProbabilisticFailing.TransactOpts, failureProbability)
}

// IncrementCounter is a paid mutator transaction binding the contract method 0x00755455.
//
// Solidity: function incrementCounter(uint8 failureProbability) returns()
func (_ProbabilisticFailing *ProbabilisticFailingTransactorSession) IncrementCounter(failureProbability uint8) (*types.Transaction, error) {
	return _ProbabilisticFailing.Contract.IncrementCounter(&_ProbabilisticFailing.TransactOpts, failureProbability)
}
