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

// ClzCounterMetaData contains all meta data concerning the ClzCounter contract.
var ClzCounterMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"getCount\",\"outputs\":[{\"internalType\":\"int256\",\"name\":\"\",\"type\":\"int256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"expectedClz\",\"type\":\"uint256\"}],\"name\":\"incrementCounter\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
	Bin: "0x60015f55608980600e5f395ff3fe600436106085575f3560e01c8063f41a27b914602e5763a87d942c146022575f80fd5b5f195f54015f5260205ff35b604436106085576024356004351e0360495760015f54015f55005b62461bcd60e51b5f52602060045260136024527f434c5a20726573756c74206d69736d617463680000000000000000000000000060445260645ffd5b5f80fd",
}

// ClzCounterABI is the input ABI used to generate the binding from.
// Deprecated: Use ClzCounterMetaData.ABI instead.
var ClzCounterABI = ClzCounterMetaData.ABI

// ClzCounterBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use ClzCounterMetaData.Bin instead.
var ClzCounterBin = ClzCounterMetaData.Bin

// DeployClzCounter deploys a new Ethereum contract, binding an instance of ClzCounter to it.
func DeployClzCounter(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *ClzCounter, error) {
	parsed, err := ClzCounterMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(ClzCounterBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &ClzCounter{ClzCounterCaller: ClzCounterCaller{contract: contract}, ClzCounterTransactor: ClzCounterTransactor{contract: contract}, ClzCounterFilterer: ClzCounterFilterer{contract: contract}}, nil
}

// ClzCounter is an auto generated Go binding around an Ethereum contract.
type ClzCounter struct {
	ClzCounterCaller     // Read-only binding to the contract
	ClzCounterTransactor // Write-only binding to the contract
	ClzCounterFilterer   // Log filterer for contract events
}

// ClzCounterCaller is an auto generated read-only Go binding around an Ethereum contract.
type ClzCounterCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ClzCounterTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ClzCounterTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ClzCounterFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ClzCounterFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ClzCounterSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ClzCounterSession struct {
	Contract     *ClzCounter       // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// ClzCounterCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ClzCounterCallerSession struct {
	Contract *ClzCounterCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts     // Call options to use throughout this session
}

// ClzCounterTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ClzCounterTransactorSession struct {
	Contract     *ClzCounterTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts     // Transaction auth options to use throughout this session
}

// ClzCounterRaw is an auto generated low-level Go binding around an Ethereum contract.
type ClzCounterRaw struct {
	Contract *ClzCounter // Generic contract binding to access the raw methods on
}

// ClzCounterCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ClzCounterCallerRaw struct {
	Contract *ClzCounterCaller // Generic read-only contract binding to access the raw methods on
}

// ClzCounterTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ClzCounterTransactorRaw struct {
	Contract *ClzCounterTransactor // Generic write-only contract binding to access the raw methods on
}

// NewClzCounter creates a new instance of ClzCounter, bound to a specific deployed contract.
func NewClzCounter(address common.Address, backend bind.ContractBackend) (*ClzCounter, error) {
	contract, err := bindClzCounter(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &ClzCounter{ClzCounterCaller: ClzCounterCaller{contract: contract}, ClzCounterTransactor: ClzCounterTransactor{contract: contract}, ClzCounterFilterer: ClzCounterFilterer{contract: contract}}, nil
}

// NewClzCounterCaller creates a new read-only instance of ClzCounter, bound to a specific deployed contract.
func NewClzCounterCaller(address common.Address, caller bind.ContractCaller) (*ClzCounterCaller, error) {
	contract, err := bindClzCounter(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ClzCounterCaller{contract: contract}, nil
}

// NewClzCounterTransactor creates a new write-only instance of ClzCounter, bound to a specific deployed contract.
func NewClzCounterTransactor(address common.Address, transactor bind.ContractTransactor) (*ClzCounterTransactor, error) {
	contract, err := bindClzCounter(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ClzCounterTransactor{contract: contract}, nil
}

// NewClzCounterFilterer creates a new log filterer instance of ClzCounter, bound to a specific deployed contract.
func NewClzCounterFilterer(address common.Address, filterer bind.ContractFilterer) (*ClzCounterFilterer, error) {
	contract, err := bindClzCounter(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ClzCounterFilterer{contract: contract}, nil
}

// bindClzCounter binds a generic wrapper to an already deployed contract.
func bindClzCounter(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := ClzCounterMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ClzCounter *ClzCounterRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ClzCounter.Contract.ClzCounterCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ClzCounter *ClzCounterRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ClzCounter.Contract.ClzCounterTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ClzCounter *ClzCounterRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ClzCounter.Contract.ClzCounterTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ClzCounter *ClzCounterCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ClzCounter.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ClzCounter *ClzCounterTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ClzCounter.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ClzCounter *ClzCounterTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ClzCounter.Contract.contract.Transact(opts, method, params...)
}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(int256)
func (_ClzCounter *ClzCounterCaller) GetCount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _ClzCounter.contract.Call(opts, &out, "getCount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(int256)
func (_ClzCounter *ClzCounterSession) GetCount() (*big.Int, error) {
	return _ClzCounter.Contract.GetCount(&_ClzCounter.CallOpts)
}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(int256)
func (_ClzCounter *ClzCounterCallerSession) GetCount() (*big.Int, error) {
	return _ClzCounter.Contract.GetCount(&_ClzCounter.CallOpts)
}

// IncrementCounter is a paid mutator transaction binding the contract method 0xf41a27b9.
//
// Solidity: function incrementCounter(uint256 value, uint256 expectedClz) returns()
func (_ClzCounter *ClzCounterTransactor) IncrementCounter(opts *bind.TransactOpts, value *big.Int, expectedClz *big.Int) (*types.Transaction, error) {
	return _ClzCounter.contract.Transact(opts, "incrementCounter", value, expectedClz)
}

// IncrementCounter is a paid mutator transaction binding the contract method 0xf41a27b9.
//
// Solidity: function incrementCounter(uint256 value, uint256 expectedClz) returns()
func (_ClzCounter *ClzCounterSession) IncrementCounter(value *big.Int, expectedClz *big.Int) (*types.Transaction, error) {
	return _ClzCounter.Contract.IncrementCounter(&_ClzCounter.TransactOpts, value, expectedClz)
}

// IncrementCounter is a paid mutator transaction binding the contract method 0xf41a27b9.
//
// Solidity: function incrementCounter(uint256 value, uint256 expectedClz) returns()
func (_ClzCounter *ClzCounterTransactorSession) IncrementCounter(value *big.Int, expectedClz *big.Int) (*types.Transaction, error) {
	return _ClzCounter.Contract.IncrementCounter(&_ClzCounter.TransactOpts, value, expectedClz)
}
