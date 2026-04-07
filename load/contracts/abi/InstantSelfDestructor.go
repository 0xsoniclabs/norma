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

// InstantSelfDestructorFactoryMetaData contains all meta data concerning the InstantSelfDestructorFactory contract.
var InstantSelfDestructorFactoryMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"deployAndDestruct\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getCount\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
	Bin: "0x6080604052348015600f57600080fd5b506101e48061001f6000396000f3fe6080604052600436106100295760003560e01c806366e30ada1461002e578063a87d942c14610038575b600080fd5b610036610058565b005b34801561004457600080fd5b504760405190815260200160405180910390f35b346001146100a25760405162461bcd60e51b8152602060048201526013602482015272115e1c1958dd1959080c481dd95a481c185a59606a1b604482015260640160405180910390fd5b6000346040516100b190610127565b6040518091039082f09050801580156100ce573d6000803e3d6000fd5b509050806001600160a01b03166383197ef06040518163ffffffff1660e01b8152600401600060405180830381600087803b15801561010c57600080fd5b505af1158015610120573d6000803e3d6000fd5b5050505050565b607b806101348339019056fe6080604052606a8060116000396000f3fe6080604052348015600f57600080fd5b506004361060285760003560e01c806383197ef014602d575b600080fd5b603233ff5b00fea26469706673582212208f36515cd37e6678eb49ec96d5540a045e78db263768294de4dcfb4afe1e204b64736f6c634300081d0033a264697066735822122021b9aee6599e56f20a82b73bd750b24115cc12a84c03bfa06b5b4dd32c7a032564736f6c634300081d0033",
}

// InstantSelfDestructorFactoryABI is the input ABI used to generate the binding from.
// Deprecated: Use InstantSelfDestructorFactoryMetaData.ABI instead.
var InstantSelfDestructorFactoryABI = InstantSelfDestructorFactoryMetaData.ABI

// InstantSelfDestructorFactoryBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use InstantSelfDestructorFactoryMetaData.Bin instead.
var InstantSelfDestructorFactoryBin = InstantSelfDestructorFactoryMetaData.Bin

// DeployInstantSelfDestructorFactory deploys a new Ethereum contract, binding an instance of InstantSelfDestructorFactory to it.
func DeployInstantSelfDestructorFactory(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *InstantSelfDestructorFactory, error) {
	parsed, err := InstantSelfDestructorFactoryMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(InstantSelfDestructorFactoryBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &InstantSelfDestructorFactory{InstantSelfDestructorFactoryCaller: InstantSelfDestructorFactoryCaller{contract: contract}, InstantSelfDestructorFactoryTransactor: InstantSelfDestructorFactoryTransactor{contract: contract}, InstantSelfDestructorFactoryFilterer: InstantSelfDestructorFactoryFilterer{contract: contract}}, nil
}

// InstantSelfDestructorFactory is an auto generated Go binding around an Ethereum contract.
type InstantSelfDestructorFactory struct {
	InstantSelfDestructorFactoryCaller     // Read-only binding to the contract
	InstantSelfDestructorFactoryTransactor // Write-only binding to the contract
	InstantSelfDestructorFactoryFilterer   // Log filterer for contract events
}

// InstantSelfDestructorFactoryCaller is an auto generated read-only Go binding around an Ethereum contract.
type InstantSelfDestructorFactoryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// InstantSelfDestructorFactoryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type InstantSelfDestructorFactoryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// InstantSelfDestructorFactoryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type InstantSelfDestructorFactoryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// InstantSelfDestructorFactorySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type InstantSelfDestructorFactorySession struct {
	Contract     *InstantSelfDestructorFactory // Generic contract binding to set the session for
	CallOpts     bind.CallOpts                 // Call options to use throughout this session
	TransactOpts bind.TransactOpts             // Transaction auth options to use throughout this session
}

// InstantSelfDestructorFactoryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type InstantSelfDestructorFactoryCallerSession struct {
	Contract *InstantSelfDestructorFactoryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts                       // Call options to use throughout this session
}

// InstantSelfDestructorFactoryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type InstantSelfDestructorFactoryTransactorSession struct {
	Contract     *InstantSelfDestructorFactoryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts                       // Transaction auth options to use throughout this session
}

// InstantSelfDestructorFactoryRaw is an auto generated low-level Go binding around an Ethereum contract.
type InstantSelfDestructorFactoryRaw struct {
	Contract *InstantSelfDestructorFactory // Generic contract binding to access the raw methods on
}

// InstantSelfDestructorFactoryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type InstantSelfDestructorFactoryCallerRaw struct {
	Contract *InstantSelfDestructorFactoryCaller // Generic read-only contract binding to access the raw methods on
}

// InstantSelfDestructorFactoryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type InstantSelfDestructorFactoryTransactorRaw struct {
	Contract *InstantSelfDestructorFactoryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewInstantSelfDestructorFactory creates a new instance of InstantSelfDestructorFactory, bound to a specific deployed contract.
func NewInstantSelfDestructorFactory(address common.Address, backend bind.ContractBackend) (*InstantSelfDestructorFactory, error) {
	contract, err := bindInstantSelfDestructorFactory(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &InstantSelfDestructorFactory{InstantSelfDestructorFactoryCaller: InstantSelfDestructorFactoryCaller{contract: contract}, InstantSelfDestructorFactoryTransactor: InstantSelfDestructorFactoryTransactor{contract: contract}, InstantSelfDestructorFactoryFilterer: InstantSelfDestructorFactoryFilterer{contract: contract}}, nil
}

// NewInstantSelfDestructorFactoryCaller creates a new read-only instance of InstantSelfDestructorFactory, bound to a specific deployed contract.
func NewInstantSelfDestructorFactoryCaller(address common.Address, caller bind.ContractCaller) (*InstantSelfDestructorFactoryCaller, error) {
	contract, err := bindInstantSelfDestructorFactory(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &InstantSelfDestructorFactoryCaller{contract: contract}, nil
}

// NewInstantSelfDestructorFactoryTransactor creates a new write-only instance of InstantSelfDestructorFactory, bound to a specific deployed contract.
func NewInstantSelfDestructorFactoryTransactor(address common.Address, transactor bind.ContractTransactor) (*InstantSelfDestructorFactoryTransactor, error) {
	contract, err := bindInstantSelfDestructorFactory(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &InstantSelfDestructorFactoryTransactor{contract: contract}, nil
}

// NewInstantSelfDestructorFactoryFilterer creates a new log filterer instance of InstantSelfDestructorFactory, bound to a specific deployed contract.
func NewInstantSelfDestructorFactoryFilterer(address common.Address, filterer bind.ContractFilterer) (*InstantSelfDestructorFactoryFilterer, error) {
	contract, err := bindInstantSelfDestructorFactory(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &InstantSelfDestructorFactoryFilterer{contract: contract}, nil
}

// bindInstantSelfDestructorFactory binds a generic wrapper to an already deployed contract.
func bindInstantSelfDestructorFactory(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := InstantSelfDestructorFactoryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_InstantSelfDestructorFactory *InstantSelfDestructorFactoryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _InstantSelfDestructorFactory.Contract.InstantSelfDestructorFactoryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_InstantSelfDestructorFactory *InstantSelfDestructorFactoryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _InstantSelfDestructorFactory.Contract.InstantSelfDestructorFactoryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_InstantSelfDestructorFactory *InstantSelfDestructorFactoryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _InstantSelfDestructorFactory.Contract.InstantSelfDestructorFactoryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_InstantSelfDestructorFactory *InstantSelfDestructorFactoryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _InstantSelfDestructorFactory.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_InstantSelfDestructorFactory *InstantSelfDestructorFactoryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _InstantSelfDestructorFactory.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_InstantSelfDestructorFactory *InstantSelfDestructorFactoryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _InstantSelfDestructorFactory.Contract.contract.Transact(opts, method, params...)
}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(uint256)
func (_InstantSelfDestructorFactory *InstantSelfDestructorFactoryCaller) GetCount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _InstantSelfDestructorFactory.contract.Call(opts, &out, "getCount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(uint256)
func (_InstantSelfDestructorFactory *InstantSelfDestructorFactorySession) GetCount() (*big.Int, error) {
	return _InstantSelfDestructorFactory.Contract.GetCount(&_InstantSelfDestructorFactory.CallOpts)
}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(uint256)
func (_InstantSelfDestructorFactory *InstantSelfDestructorFactoryCallerSession) GetCount() (*big.Int, error) {
	return _InstantSelfDestructorFactory.Contract.GetCount(&_InstantSelfDestructorFactory.CallOpts)
}

// DeployAndDestruct is a paid mutator transaction binding the contract method 0x66e30ada.
//
// Solidity: function deployAndDestruct() payable returns()
func (_InstantSelfDestructorFactory *InstantSelfDestructorFactoryTransactor) DeployAndDestruct(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _InstantSelfDestructorFactory.contract.Transact(opts, "deployAndDestruct")
}

// DeployAndDestruct is a paid mutator transaction binding the contract method 0x66e30ada.
//
// Solidity: function deployAndDestruct() payable returns()
func (_InstantSelfDestructorFactory *InstantSelfDestructorFactorySession) DeployAndDestruct() (*types.Transaction, error) {
	return _InstantSelfDestructorFactory.Contract.DeployAndDestruct(&_InstantSelfDestructorFactory.TransactOpts)
}

// DeployAndDestruct is a paid mutator transaction binding the contract method 0x66e30ada.
//
// Solidity: function deployAndDestruct() payable returns()
func (_InstantSelfDestructorFactory *InstantSelfDestructorFactoryTransactorSession) DeployAndDestruct() (*types.Transaction, error) {
	return _InstantSelfDestructorFactory.Contract.DeployAndDestruct(&_InstantSelfDestructorFactory.TransactOpts)
}
