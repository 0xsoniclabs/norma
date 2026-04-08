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

// SelfDestructOldContractFactoryMetaData contains all meta data concerning the SelfDestructOldContractFactory contract.
var SelfDestructOldContractFactoryMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"stateMutability\":\"payable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"constructedContract\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"destructAndDeploy\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getCount\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
	Bin: "0x60806040526000346040516011906053565b6040518091039082f0905080158015602d573d6000803e3d6000fd5b50600080546001600160a01b0319166001600160a01b039290921691909117905550605f565b607b806102b983390190565b61024b8061006e6000396000f3fe6080604052600436106100345760003560e01c80631d4078ed1461003957806373d8000e14610043578063a87d942c14610080575b600080fd5b61004161009b565b005b34801561004f57600080fd5b50600054610063906001600160a01b031681565b6040516001600160a01b0390911681526020015b60405180910390f35b34801561008c57600080fd5b50604051478152602001610077565b346001146100e55760405162461bcd60e51b8152602060048201526013602482015272115e1c1958dd1959080c481dd95a481c185a59606a1b604482015260640160405180910390fd5b600080546040805163083197ef60e41b815290516001600160a01b03909216926383197ef09260048084019382900301818387803b15801561012657600080fd5b505af115801561013a573d6000803e3d6000fd5b5050505060003460405161014d9061018e565b6040518091039082f090508015801561016a573d6000803e3d6000fd5b50600080546001600160a01b0319166001600160a01b039290921691909117905550565b607b8061019b8339019056fe6080604052606a8060116000396000f3fe6080604052348015600f57600080fd5b506004361060285760003560e01c806383197ef014602d575b600080fd5b603233ff5b00fea26469706673582212200394e1001dc81d277699493980393e30ae6180b818a822ed8fbc0bc04189177164736f6c63430008220033a26469706673582212200c98a128225e4282ae922eec7bb181af0bc1d393409e23f4998af8e95dda372264736f6c634300082200336080604052606a8060116000396000f3fe6080604052348015600f57600080fd5b506004361060285760003560e01c806383197ef014602d575b600080fd5b603233ff5b00fea26469706673582212200394e1001dc81d277699493980393e30ae6180b818a822ed8fbc0bc04189177164736f6c63430008220033",
}

// SelfDestructOldContractFactoryABI is the input ABI used to generate the binding from.
// Deprecated: Use SelfDestructOldContractFactoryMetaData.ABI instead.
var SelfDestructOldContractFactoryABI = SelfDestructOldContractFactoryMetaData.ABI

// SelfDestructOldContractFactoryBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use SelfDestructOldContractFactoryMetaData.Bin instead.
var SelfDestructOldContractFactoryBin = SelfDestructOldContractFactoryMetaData.Bin

// DeploySelfDestructOldContractFactory deploys a new Ethereum contract, binding an instance of SelfDestructOldContractFactory to it.
func DeploySelfDestructOldContractFactory(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *SelfDestructOldContractFactory, error) {
	parsed, err := SelfDestructOldContractFactoryMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(SelfDestructOldContractFactoryBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &SelfDestructOldContractFactory{SelfDestructOldContractFactoryCaller: SelfDestructOldContractFactoryCaller{contract: contract}, SelfDestructOldContractFactoryTransactor: SelfDestructOldContractFactoryTransactor{contract: contract}, SelfDestructOldContractFactoryFilterer: SelfDestructOldContractFactoryFilterer{contract: contract}}, nil
}

// SelfDestructOldContractFactory is an auto generated Go binding around an Ethereum contract.
type SelfDestructOldContractFactory struct {
	SelfDestructOldContractFactoryCaller     // Read-only binding to the contract
	SelfDestructOldContractFactoryTransactor // Write-only binding to the contract
	SelfDestructOldContractFactoryFilterer   // Log filterer for contract events
}

// SelfDestructOldContractFactoryCaller is an auto generated read-only Go binding around an Ethereum contract.
type SelfDestructOldContractFactoryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SelfDestructOldContractFactoryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type SelfDestructOldContractFactoryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SelfDestructOldContractFactoryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type SelfDestructOldContractFactoryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SelfDestructOldContractFactorySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type SelfDestructOldContractFactorySession struct {
	Contract     *SelfDestructOldContractFactory // Generic contract binding to set the session for
	CallOpts     bind.CallOpts                   // Call options to use throughout this session
	TransactOpts bind.TransactOpts               // Transaction auth options to use throughout this session
}

// SelfDestructOldContractFactoryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type SelfDestructOldContractFactoryCallerSession struct {
	Contract *SelfDestructOldContractFactoryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts                         // Call options to use throughout this session
}

// SelfDestructOldContractFactoryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type SelfDestructOldContractFactoryTransactorSession struct {
	Contract     *SelfDestructOldContractFactoryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts                         // Transaction auth options to use throughout this session
}

// SelfDestructOldContractFactoryRaw is an auto generated low-level Go binding around an Ethereum contract.
type SelfDestructOldContractFactoryRaw struct {
	Contract *SelfDestructOldContractFactory // Generic contract binding to access the raw methods on
}

// SelfDestructOldContractFactoryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type SelfDestructOldContractFactoryCallerRaw struct {
	Contract *SelfDestructOldContractFactoryCaller // Generic read-only contract binding to access the raw methods on
}

// SelfDestructOldContractFactoryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type SelfDestructOldContractFactoryTransactorRaw struct {
	Contract *SelfDestructOldContractFactoryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewSelfDestructOldContractFactory creates a new instance of SelfDestructOldContractFactory, bound to a specific deployed contract.
func NewSelfDestructOldContractFactory(address common.Address, backend bind.ContractBackend) (*SelfDestructOldContractFactory, error) {
	contract, err := bindSelfDestructOldContractFactory(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &SelfDestructOldContractFactory{SelfDestructOldContractFactoryCaller: SelfDestructOldContractFactoryCaller{contract: contract}, SelfDestructOldContractFactoryTransactor: SelfDestructOldContractFactoryTransactor{contract: contract}, SelfDestructOldContractFactoryFilterer: SelfDestructOldContractFactoryFilterer{contract: contract}}, nil
}

// NewSelfDestructOldContractFactoryCaller creates a new read-only instance of SelfDestructOldContractFactory, bound to a specific deployed contract.
func NewSelfDestructOldContractFactoryCaller(address common.Address, caller bind.ContractCaller) (*SelfDestructOldContractFactoryCaller, error) {
	contract, err := bindSelfDestructOldContractFactory(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &SelfDestructOldContractFactoryCaller{contract: contract}, nil
}

// NewSelfDestructOldContractFactoryTransactor creates a new write-only instance of SelfDestructOldContractFactory, bound to a specific deployed contract.
func NewSelfDestructOldContractFactoryTransactor(address common.Address, transactor bind.ContractTransactor) (*SelfDestructOldContractFactoryTransactor, error) {
	contract, err := bindSelfDestructOldContractFactory(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &SelfDestructOldContractFactoryTransactor{contract: contract}, nil
}

// NewSelfDestructOldContractFactoryFilterer creates a new log filterer instance of SelfDestructOldContractFactory, bound to a specific deployed contract.
func NewSelfDestructOldContractFactoryFilterer(address common.Address, filterer bind.ContractFilterer) (*SelfDestructOldContractFactoryFilterer, error) {
	contract, err := bindSelfDestructOldContractFactory(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &SelfDestructOldContractFactoryFilterer{contract: contract}, nil
}

// bindSelfDestructOldContractFactory binds a generic wrapper to an already deployed contract.
func bindSelfDestructOldContractFactory(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := SelfDestructOldContractFactoryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_SelfDestructOldContractFactory *SelfDestructOldContractFactoryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _SelfDestructOldContractFactory.Contract.SelfDestructOldContractFactoryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_SelfDestructOldContractFactory *SelfDestructOldContractFactoryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SelfDestructOldContractFactory.Contract.SelfDestructOldContractFactoryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_SelfDestructOldContractFactory *SelfDestructOldContractFactoryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _SelfDestructOldContractFactory.Contract.SelfDestructOldContractFactoryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_SelfDestructOldContractFactory *SelfDestructOldContractFactoryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _SelfDestructOldContractFactory.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_SelfDestructOldContractFactory *SelfDestructOldContractFactoryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SelfDestructOldContractFactory.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_SelfDestructOldContractFactory *SelfDestructOldContractFactoryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _SelfDestructOldContractFactory.Contract.contract.Transact(opts, method, params...)
}

// ConstructedContract is a free data retrieval call binding the contract method 0x73d8000e.
//
// Solidity: function constructedContract() view returns(address)
func (_SelfDestructOldContractFactory *SelfDestructOldContractFactoryCaller) ConstructedContract(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _SelfDestructOldContractFactory.contract.Call(opts, &out, "constructedContract")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// ConstructedContract is a free data retrieval call binding the contract method 0x73d8000e.
//
// Solidity: function constructedContract() view returns(address)
func (_SelfDestructOldContractFactory *SelfDestructOldContractFactorySession) ConstructedContract() (common.Address, error) {
	return _SelfDestructOldContractFactory.Contract.ConstructedContract(&_SelfDestructOldContractFactory.CallOpts)
}

// ConstructedContract is a free data retrieval call binding the contract method 0x73d8000e.
//
// Solidity: function constructedContract() view returns(address)
func (_SelfDestructOldContractFactory *SelfDestructOldContractFactoryCallerSession) ConstructedContract() (common.Address, error) {
	return _SelfDestructOldContractFactory.Contract.ConstructedContract(&_SelfDestructOldContractFactory.CallOpts)
}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(uint256)
func (_SelfDestructOldContractFactory *SelfDestructOldContractFactoryCaller) GetCount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _SelfDestructOldContractFactory.contract.Call(opts, &out, "getCount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(uint256)
func (_SelfDestructOldContractFactory *SelfDestructOldContractFactorySession) GetCount() (*big.Int, error) {
	return _SelfDestructOldContractFactory.Contract.GetCount(&_SelfDestructOldContractFactory.CallOpts)
}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(uint256)
func (_SelfDestructOldContractFactory *SelfDestructOldContractFactoryCallerSession) GetCount() (*big.Int, error) {
	return _SelfDestructOldContractFactory.Contract.GetCount(&_SelfDestructOldContractFactory.CallOpts)
}

// DestructAndDeploy is a paid mutator transaction binding the contract method 0x1d4078ed.
//
// Solidity: function destructAndDeploy() payable returns()
func (_SelfDestructOldContractFactory *SelfDestructOldContractFactoryTransactor) DestructAndDeploy(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SelfDestructOldContractFactory.contract.Transact(opts, "destructAndDeploy")
}

// DestructAndDeploy is a paid mutator transaction binding the contract method 0x1d4078ed.
//
// Solidity: function destructAndDeploy() payable returns()
func (_SelfDestructOldContractFactory *SelfDestructOldContractFactorySession) DestructAndDeploy() (*types.Transaction, error) {
	return _SelfDestructOldContractFactory.Contract.DestructAndDeploy(&_SelfDestructOldContractFactory.TransactOpts)
}

// DestructAndDeploy is a paid mutator transaction binding the contract method 0x1d4078ed.
//
// Solidity: function destructAndDeploy() payable returns()
func (_SelfDestructOldContractFactory *SelfDestructOldContractFactoryTransactorSession) DestructAndDeploy() (*types.Transaction, error) {
	return _SelfDestructOldContractFactory.Contract.DestructAndDeploy(&_SelfDestructOldContractFactory.TransactOpts)
}
