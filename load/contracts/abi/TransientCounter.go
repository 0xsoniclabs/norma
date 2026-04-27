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

// TransientCounterMetaData contains all meta data concerning the TransientCounter contract.
var TransientCounterMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"getCount\",\"outputs\":[{\"internalType\":\"int256\",\"name\":\"\",\"type\":\"int256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"incrementCounter\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"incrementCounterTwice\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
	Bin: "0x608060405260015f553480156012575f5ffd5b506101e4806100205f395ff3fe608060405234801561000f575f5ffd5b506004361061003f575f3560e01c8063581332d3146100435780635b34b9661461004d578063a87d942c14610055575b5f5ffd5b61004b61006f565b005b61004b61010d565b61005d610139565b60405190815260200160405180910390f35b306001600160a01b0316635b34b9666040518163ffffffff1660e01b81526004015f604051808303815f87803b1580156100a7575f5ffd5b505af11580156100b9573d5f5f3e3d5ffd5b50505050306001600160a01b0316635b34b9666040518163ffffffff1660e01b81526004015f604051808303815f87803b1580156100f5575f5ffd5b505af1158015610107573d5f5f3e3d5ffd5b50505050565b6112345c801561011a5750565b60016112345d60015f5f8282546101319190610161565b909155505050565b5f60015f546101489190610188565b905090565b634e487b7160e01b5f52601160045260245ffd5b8082018281125f8312801582168215821617156101805761018061014d565b505092915050565b8181035f8312801583831316838312821617156101a7576101a761014d565b509291505056fea2646970667358221220e4aebed2aa6d1584504f8faf06ed14955ee73e0de8a52e570449c518a1df401764736f6c634300081d0033",
}

// TransientCounterABI is the input ABI used to generate the binding from.
// Deprecated: Use TransientCounterMetaData.ABI instead.
var TransientCounterABI = TransientCounterMetaData.ABI

// TransientCounterBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use TransientCounterMetaData.Bin instead.
var TransientCounterBin = TransientCounterMetaData.Bin

// DeployTransientCounter deploys a new Ethereum contract, binding an instance of TransientCounter to it.
func DeployTransientCounter(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *TransientCounter, error) {
	parsed, err := TransientCounterMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(TransientCounterBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &TransientCounter{TransientCounterCaller: TransientCounterCaller{contract: contract}, TransientCounterTransactor: TransientCounterTransactor{contract: contract}, TransientCounterFilterer: TransientCounterFilterer{contract: contract}}, nil
}

// TransientCounter is an auto generated Go binding around an Ethereum contract.
type TransientCounter struct {
	TransientCounterCaller     // Read-only binding to the contract
	TransientCounterTransactor // Write-only binding to the contract
	TransientCounterFilterer   // Log filterer for contract events
}

// TransientCounterCaller is an auto generated read-only Go binding around an Ethereum contract.
type TransientCounterCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// TransientCounterTransactor is an auto generated write-only Go binding around an Ethereum contract.
type TransientCounterTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// TransientCounterFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type TransientCounterFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// TransientCounterSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type TransientCounterSession struct {
	Contract     *TransientCounter // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// TransientCounterCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type TransientCounterCallerSession struct {
	Contract *TransientCounterCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts           // Call options to use throughout this session
}

// TransientCounterTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type TransientCounterTransactorSession struct {
	Contract     *TransientCounterTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts           // Transaction auth options to use throughout this session
}

// TransientCounterRaw is an auto generated low-level Go binding around an Ethereum contract.
type TransientCounterRaw struct {
	Contract *TransientCounter // Generic contract binding to access the raw methods on
}

// TransientCounterCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type TransientCounterCallerRaw struct {
	Contract *TransientCounterCaller // Generic read-only contract binding to access the raw methods on
}

// TransientCounterTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type TransientCounterTransactorRaw struct {
	Contract *TransientCounterTransactor // Generic write-only contract binding to access the raw methods on
}

// NewTransientCounter creates a new instance of TransientCounter, bound to a specific deployed contract.
func NewTransientCounter(address common.Address, backend bind.ContractBackend) (*TransientCounter, error) {
	contract, err := bindTransientCounter(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &TransientCounter{TransientCounterCaller: TransientCounterCaller{contract: contract}, TransientCounterTransactor: TransientCounterTransactor{contract: contract}, TransientCounterFilterer: TransientCounterFilterer{contract: contract}}, nil
}

// NewTransientCounterCaller creates a new read-only instance of TransientCounter, bound to a specific deployed contract.
func NewTransientCounterCaller(address common.Address, caller bind.ContractCaller) (*TransientCounterCaller, error) {
	contract, err := bindTransientCounter(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &TransientCounterCaller{contract: contract}, nil
}

// NewTransientCounterTransactor creates a new write-only instance of TransientCounter, bound to a specific deployed contract.
func NewTransientCounterTransactor(address common.Address, transactor bind.ContractTransactor) (*TransientCounterTransactor, error) {
	contract, err := bindTransientCounter(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &TransientCounterTransactor{contract: contract}, nil
}

// NewTransientCounterFilterer creates a new log filterer instance of TransientCounter, bound to a specific deployed contract.
func NewTransientCounterFilterer(address common.Address, filterer bind.ContractFilterer) (*TransientCounterFilterer, error) {
	contract, err := bindTransientCounter(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &TransientCounterFilterer{contract: contract}, nil
}

// bindTransientCounter binds a generic wrapper to an already deployed contract.
func bindTransientCounter(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := TransientCounterMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_TransientCounter *TransientCounterRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _TransientCounter.Contract.TransientCounterCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_TransientCounter *TransientCounterRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _TransientCounter.Contract.TransientCounterTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_TransientCounter *TransientCounterRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _TransientCounter.Contract.TransientCounterTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_TransientCounter *TransientCounterCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _TransientCounter.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_TransientCounter *TransientCounterTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _TransientCounter.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_TransientCounter *TransientCounterTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _TransientCounter.Contract.contract.Transact(opts, method, params...)
}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(int256)
func (_TransientCounter *TransientCounterCaller) GetCount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _TransientCounter.contract.Call(opts, &out, "getCount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(int256)
func (_TransientCounter *TransientCounterSession) GetCount() (*big.Int, error) {
	return _TransientCounter.Contract.GetCount(&_TransientCounter.CallOpts)
}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(int256)
func (_TransientCounter *TransientCounterCallerSession) GetCount() (*big.Int, error) {
	return _TransientCounter.Contract.GetCount(&_TransientCounter.CallOpts)
}

// IncrementCounter is a paid mutator transaction binding the contract method 0x5b34b966.
//
// Solidity: function incrementCounter() returns()
func (_TransientCounter *TransientCounterTransactor) IncrementCounter(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _TransientCounter.contract.Transact(opts, "incrementCounter")
}

// IncrementCounter is a paid mutator transaction binding the contract method 0x5b34b966.
//
// Solidity: function incrementCounter() returns()
func (_TransientCounter *TransientCounterSession) IncrementCounter() (*types.Transaction, error) {
	return _TransientCounter.Contract.IncrementCounter(&_TransientCounter.TransactOpts)
}

// IncrementCounter is a paid mutator transaction binding the contract method 0x5b34b966.
//
// Solidity: function incrementCounter() returns()
func (_TransientCounter *TransientCounterTransactorSession) IncrementCounter() (*types.Transaction, error) {
	return _TransientCounter.Contract.IncrementCounter(&_TransientCounter.TransactOpts)
}

// IncrementCounterTwice is a paid mutator transaction binding the contract method 0x581332d3.
//
// Solidity: function incrementCounterTwice() returns()
func (_TransientCounter *TransientCounterTransactor) IncrementCounterTwice(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _TransientCounter.contract.Transact(opts, "incrementCounterTwice")
}

// IncrementCounterTwice is a paid mutator transaction binding the contract method 0x581332d3.
//
// Solidity: function incrementCounterTwice() returns()
func (_TransientCounter *TransientCounterSession) IncrementCounterTwice() (*types.Transaction, error) {
	return _TransientCounter.Contract.IncrementCounterTwice(&_TransientCounter.TransactOpts)
}

// IncrementCounterTwice is a paid mutator transaction binding the contract method 0x581332d3.
//
// Solidity: function incrementCounterTwice() returns()
func (_TransientCounter *TransientCounterTransactorSession) IncrementCounterTwice() (*types.Transaction, error) {
	return _TransientCounter.Contract.IncrementCounterTwice(&_TransientCounter.TransactOpts)
}
