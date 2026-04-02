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
	ABI: "[{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"pubKeyX\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"pubKeyY\",\"type\":\"bytes32\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"getCount\",\"outputs\":[{\"internalType\":\"int256\",\"name\":\"\",\"type\":\"int256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"hash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"r\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"s\",\"type\":\"bytes32\"}],\"name\":\"incrementCounter\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
	Bin: "0x60c060405260015f553480156012575f5ffd5b5060405161034d38038061034d833981016040819052602f91603c565b60809190915260a052605d565b5f5f60408385031215604c575f5ffd5b505080516020909101519092909150565b60805160a0516102d161007c5f395f60bb01525f609501526102d15ff3fe608060405234801561000f575f5ffd5b5060043610610034575f3560e01c8063a87d942c14610038578063f501f9cd14610052575b5f5ffd5b610040610067565b60405190815260200160405180910390f35b6100656100603660046101d5565b61007b565b005b5f60015f546100769190610212565b905090565b6040805160208101859052908101839052606081018290527f000000000000000000000000000000000000000000000000000000000000000060808201527f000000000000000000000000000000000000000000000000000000000000000060a08201525f9081906101009060c00160408051601f198184030181529082905261010491610238565b5f60405180830381855afa9150503d805f811461013c576040519150601f19603f3d011682016040523d82523d5f602084013e610141565b606091505b5091509150818015610154575080516020145b801561016857506101648161024e565b6001145b6101b85760405162461bcd60e51b815260206004820152601760248201527f696e76616c696420502d323536207369676e6174757265000000000000000000604482015260640160405180910390fd5b60015f5f8282546101c99190610274565b90915550505050505050565b5f5f5f606084860312156101e7575f5ffd5b505081359360208301359350604090920135919050565b634e487b7160e01b5f52601160045260245ffd5b8181035f831280158383131683831282161715610231576102316101fe565b5092915050565b5f82518060208501845e5f920191825250919050565b8051602080830151919081101561026e575f198160200360031b1b821691505b50919050565b8082018281125f831280158216821582161715610293576102936101fe565b50509291505056fea2646970667358221220f8981fae62c19fac5f550854939efd540a796e5cd631517c1726aa769fdbb30364736f6c634300081d0033",
}

// OsakaCounterABI is the input ABI used to generate the binding from.
// Deprecated: Use OsakaCounterMetaData.ABI instead.
var OsakaCounterABI = OsakaCounterMetaData.ABI

// OsakaCounterBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use OsakaCounterMetaData.Bin instead.
var OsakaCounterBin = OsakaCounterMetaData.Bin

// DeployOsakaCounter deploys a new Ethereum contract, binding an instance of OsakaCounter to it.
func DeployOsakaCounter(auth *bind.TransactOpts, backend bind.ContractBackend, pubKeyX [32]byte, pubKeyY [32]byte) (common.Address, *types.Transaction, *OsakaCounter, error) {
	parsed, err := OsakaCounterMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(OsakaCounterBin), backend, pubKeyX, pubKeyY)
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

// IncrementCounter is a paid mutator transaction binding the contract method 0xf501f9cd.
//
// Solidity: function incrementCounter(bytes32 hash, bytes32 r, bytes32 s) returns()
func (_OsakaCounter *OsakaCounterTransactor) IncrementCounter(opts *bind.TransactOpts, hash [32]byte, r [32]byte, s [32]byte) (*types.Transaction, error) {
	return _OsakaCounter.contract.Transact(opts, "incrementCounter", hash, r, s)
}

// IncrementCounter is a paid mutator transaction binding the contract method 0xf501f9cd.
//
// Solidity: function incrementCounter(bytes32 hash, bytes32 r, bytes32 s) returns()
func (_OsakaCounter *OsakaCounterSession) IncrementCounter(hash [32]byte, r [32]byte, s [32]byte) (*types.Transaction, error) {
	return _OsakaCounter.Contract.IncrementCounter(&_OsakaCounter.TransactOpts, hash, r, s)
}

// IncrementCounter is a paid mutator transaction binding the contract method 0xf501f9cd.
//
// Solidity: function incrementCounter(bytes32 hash, bytes32 r, bytes32 s) returns()
func (_OsakaCounter *OsakaCounterTransactorSession) IncrementCounter(hash [32]byte, r [32]byte, s [32]byte) (*types.Transaction, error) {
	return _OsakaCounter.Contract.IncrementCounter(&_OsakaCounter.TransactOpts, hash, r, s)
}
