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

// OsakaCounterMetaData contains all meta data concerning the OsakaCounter contract.
var OsakaCounterMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"getCount\",\"outputs\":[{\"internalType\":\"int256\",\"name\":\"\",\"type\":\"int256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"incrementCounter\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
	Bin: "0xef0001010004020001001503000101070400000000800002608060405234e1000960015f555f6080ee005f80fdef000101000402000100b1040043000080000660806040526004361015e100035f80fd5f3560e01c80635b34b96614e1004c63a87d942c14e10003e0ffe234e100395f600319360112e1002c5f545f198101908113600116e1000a602090604051908152f3634e487b7160e01b5f52601160045260245ffd5f80fd5f80fd34e1003f5f600319360112e100325f5460018101905f600183129112908015821691151617e100055f555f80f3634e487b7160e01b5f52601160045260245ffd5f80fd5f80fda36469706673582212203564587fbc47280397a804be13bb3e57a7e170c8e9d0d5ba75a2f1d39c473c7e6c6578706572696d656e74616cf564736f6c63430008220041",
}

// OsakaCounterABI is the input ABI used to generate the binding from.
// Deprecated: Use OsakaCounterMetaData.ABI instead.
var OsakaCounterABI = OsakaCounterMetaData.ABI

// OsakaCounterBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use OsakaCounterMetaData.Bin instead.
var OsakaCounterBin = OsakaCounterMetaData.Bin

// DeployOsakaCounter deploys a new Ethereum contract, binding an instance of OsakaCounter to it.
func DeployOsakaCounter(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *OsakaCounter, error) {
	parsed, err := OsakaCounterMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(OsakaCounterBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &OsakaCounter{OsakaCounterCaller: OsakaCounterCaller{contract: contract}, OsakaCounterTransactor: OsakaCounterTransactor{contract: contract}, OsakaCounterFilterer: OsakaCounterFilterer{contract: contract}}, nil
}

// OsakaCounter is an auto generated Go binding around an Ethereum contract.
type OsakaCounter struct {
	OsakaCounterCaller     // Read-only binding to the contract
	OsakaCounterTransactor // Write-only binding to the contract
	OsakaCounterFilterer   // Log filterer for contract events
}

// OsakaCounterCaller is an auto generated read-only Go binding around an Ethereum contract.
type OsakaCounterCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// OsakaCounterTransactor is an auto generated write-only Go binding around an Ethereum contract.
type OsakaCounterTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// OsakaCounterFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type OsakaCounterFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// OsakaCounterSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type OsakaCounterSession struct {
	Contract     *OsakaCounter     // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// OsakaCounterCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type OsakaCounterCallerSession struct {
	Contract *OsakaCounterCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts       // Call options to use throughout this session
}

// OsakaCounterTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type OsakaCounterTransactorSession struct {
	Contract     *OsakaCounterTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts       // Transaction auth options to use throughout this session
}

// OsakaCounterRaw is an auto generated low-level Go binding around an Ethereum contract.
type OsakaCounterRaw struct {
	Contract *OsakaCounter // Generic contract binding to access the raw methods on
}

// OsakaCounterCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type OsakaCounterCallerRaw struct {
	Contract *OsakaCounterCaller // Generic read-only contract binding to access the raw methods on
}

// OsakaCounterTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type OsakaCounterTransactorRaw struct {
	Contract *OsakaCounterTransactor // Generic write-only contract binding to access the raw methods on
}

// NewOsakaCounter creates a new instance of OsakaCounter, bound to a specific deployed contract.
func NewOsakaCounter(address common.Address, backend bind.ContractBackend) (*OsakaCounter, error) {
	contract, err := bindOsakaCounter(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &OsakaCounter{OsakaCounterCaller: OsakaCounterCaller{contract: contract}, OsakaCounterTransactor: OsakaCounterTransactor{contract: contract}, OsakaCounterFilterer: OsakaCounterFilterer{contract: contract}}, nil
}

// NewOsakaCounterCaller creates a new read-only instance of OsakaCounter, bound to a specific deployed contract.
func NewOsakaCounterCaller(address common.Address, caller bind.ContractCaller) (*OsakaCounterCaller, error) {
	contract, err := bindOsakaCounter(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &OsakaCounterCaller{contract: contract}, nil
}

// NewOsakaCounterTransactor creates a new write-only instance of OsakaCounter, bound to a specific deployed contract.
func NewOsakaCounterTransactor(address common.Address, transactor bind.ContractTransactor) (*OsakaCounterTransactor, error) {
	contract, err := bindOsakaCounter(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &OsakaCounterTransactor{contract: contract}, nil
}

// NewOsakaCounterFilterer creates a new log filterer instance of OsakaCounter, bound to a specific deployed contract.
func NewOsakaCounterFilterer(address common.Address, filterer bind.ContractFilterer) (*OsakaCounterFilterer, error) {
	contract, err := bindOsakaCounter(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &OsakaCounterFilterer{contract: contract}, nil
}

// bindOsakaCounter binds a generic wrapper to an already deployed contract.
func bindOsakaCounter(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := OsakaCounterMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_OsakaCounter *OsakaCounterRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _OsakaCounter.Contract.OsakaCounterCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_OsakaCounter *OsakaCounterRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _OsakaCounter.Contract.OsakaCounterTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_OsakaCounter *OsakaCounterRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _OsakaCounter.Contract.OsakaCounterTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_OsakaCounter *OsakaCounterCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _OsakaCounter.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_OsakaCounter *OsakaCounterTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _OsakaCounter.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_OsakaCounter *OsakaCounterTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _OsakaCounter.Contract.contract.Transact(opts, method, params...)
}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(int256)
func (_OsakaCounter *OsakaCounterCaller) GetCount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _OsakaCounter.contract.Call(opts, &out, "getCount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(int256)
func (_OsakaCounter *OsakaCounterSession) GetCount() (*big.Int, error) {
	return _OsakaCounter.Contract.GetCount(&_OsakaCounter.CallOpts)
}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(int256)
func (_OsakaCounter *OsakaCounterCallerSession) GetCount() (*big.Int, error) {
	return _OsakaCounter.Contract.GetCount(&_OsakaCounter.CallOpts)
}

// IncrementCounter is a paid mutator transaction binding the contract method 0x5b34b966.
//
// Solidity: function incrementCounter() returns()
func (_OsakaCounter *OsakaCounterTransactor) IncrementCounter(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _OsakaCounter.contract.Transact(opts, "incrementCounter")
}

// IncrementCounter is a paid mutator transaction binding the contract method 0x5b34b966.
//
// Solidity: function incrementCounter() returns()
func (_OsakaCounter *OsakaCounterSession) IncrementCounter() (*types.Transaction, error) {
	return _OsakaCounter.Contract.IncrementCounter(&_OsakaCounter.TransactOpts)
}

// IncrementCounter is a paid mutator transaction binding the contract method 0x5b34b966.
//
// Solidity: function incrementCounter() returns()
func (_OsakaCounter *OsakaCounterTransactorSession) IncrementCounter() (*types.Transaction, error) {
	return _OsakaCounter.Contract.IncrementCounter(&_OsakaCounter.TransactOpts)
}
