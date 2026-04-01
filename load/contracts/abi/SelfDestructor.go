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

// SelfDestructorFactoryMetaData contains all meta data concerning the SelfDestructorFactory contract.
var SelfDestructorFactoryMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"constructedContract\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"deployOrDestruct\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getCount\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
	Bin: "0x6080604052348015600f57600080fd5b5061030f8061001f6000396000f3fe6080604052600436106100345760003560e01c80633cb239671461003957806373d8000e14610043578063a87d942c14610080575b600080fd5b6100416100a3565b005b34801561004f57600080fd5b50600054610063906001600160a01b031681565b6040516001600160a01b0390911681526020015b60405180910390f35b34801561008c57600080fd5b506100956101b8565b604051908152602001610077565b346001146100ed5760405162461bcd60e51b8152602060048201526013602482015272115e1c1958dd1959080c481dd95a481c185a59606a1b604482015260640160405180910390fd5b6000546001600160a01b031661014d5760003460405161010c906101df565b6040518091039082f0905080158015610129573d6000803e3d6000fd5b50600080546001600160a01b0319166001600160a01b039290921691909117905550565b600080546040805163083197ef60e41b815290516001600160a01b03909216926383197ef09260048084019382900301818387803b15801561018e57600080fd5b505af11580156101a2573d6000803e3d6000fd5b5050600080546001600160a01b03191690555050565b6000805447906001600160a01b0316156101da576101d76001826101eb565b90505b919050565b60a18061021383390190565b8082018082111561020c57634e487b7160e01b600052601160045260246000fd5b9291505056fe608060405260908060116000396000f3fe6080604052348015600f57600080fd5b506004361060285760003560e01c806383197ef014602d575b600080fd5b603233ff5b00fea26469706673582212200fa84dcc53875cbe84f17fe9eb06e64e6f9fcfa09051effee70da1b145a2bb7564736f6c637828302e382e32352d646576656c6f702e323032342e322e32342b636f6d6d69742e64626137353465630059a2646970667358221220e71c789d7437c53882fcfd7946c4b1b7249bf26698c136fc4eef1490747bbfdd64736f6c637828302e382e32352d646576656c6f702e323032342e322e32342b636f6d6d69742e64626137353465630059",
}

// SelfDestructorFactoryABI is the input ABI used to generate the binding from.
// Deprecated: Use SelfDestructorFactoryMetaData.ABI instead.
var SelfDestructorFactoryABI = SelfDestructorFactoryMetaData.ABI

// SelfDestructorFactoryBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use SelfDestructorFactoryMetaData.Bin instead.
var SelfDestructorFactoryBin = SelfDestructorFactoryMetaData.Bin

// DeploySelfDestructorFactory deploys a new Ethereum contract, binding an instance of SelfDestructorFactory to it.
func DeploySelfDestructorFactory(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *SelfDestructorFactory, error) {
	parsed, err := SelfDestructorFactoryMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(SelfDestructorFactoryBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &SelfDestructorFactory{SelfDestructorFactoryCaller: SelfDestructorFactoryCaller{contract: contract}, SelfDestructorFactoryTransactor: SelfDestructorFactoryTransactor{contract: contract}, SelfDestructorFactoryFilterer: SelfDestructorFactoryFilterer{contract: contract}}, nil
}

// SelfDestructorFactory is an auto generated Go binding around an Ethereum contract.
type SelfDestructorFactory struct {
	SelfDestructorFactoryCaller     // Read-only binding to the contract
	SelfDestructorFactoryTransactor // Write-only binding to the contract
	SelfDestructorFactoryFilterer   // Log filterer for contract events
}

// SelfDestructorFactoryCaller is an auto generated read-only Go binding around an Ethereum contract.
type SelfDestructorFactoryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SelfDestructorFactoryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type SelfDestructorFactoryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SelfDestructorFactoryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type SelfDestructorFactoryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SelfDestructorFactorySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type SelfDestructorFactorySession struct {
	Contract     *SelfDestructorFactory // Generic contract binding to set the session for
	CallOpts     bind.CallOpts          // Call options to use throughout this session
	TransactOpts bind.TransactOpts      // Transaction auth options to use throughout this session
}

// SelfDestructorFactoryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type SelfDestructorFactoryCallerSession struct {
	Contract *SelfDestructorFactoryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts                // Call options to use throughout this session
}

// SelfDestructorFactoryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type SelfDestructorFactoryTransactorSession struct {
	Contract     *SelfDestructorFactoryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts                // Transaction auth options to use throughout this session
}

// SelfDestructorFactoryRaw is an auto generated low-level Go binding around an Ethereum contract.
type SelfDestructorFactoryRaw struct {
	Contract *SelfDestructorFactory // Generic contract binding to access the raw methods on
}

// SelfDestructorFactoryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type SelfDestructorFactoryCallerRaw struct {
	Contract *SelfDestructorFactoryCaller // Generic read-only contract binding to access the raw methods on
}

// SelfDestructorFactoryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type SelfDestructorFactoryTransactorRaw struct {
	Contract *SelfDestructorFactoryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewSelfDestructorFactory creates a new instance of SelfDestructorFactory, bound to a specific deployed contract.
func NewSelfDestructorFactory(address common.Address, backend bind.ContractBackend) (*SelfDestructorFactory, error) {
	contract, err := bindSelfDestructorFactory(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &SelfDestructorFactory{SelfDestructorFactoryCaller: SelfDestructorFactoryCaller{contract: contract}, SelfDestructorFactoryTransactor: SelfDestructorFactoryTransactor{contract: contract}, SelfDestructorFactoryFilterer: SelfDestructorFactoryFilterer{contract: contract}}, nil
}

// NewSelfDestructorFactoryCaller creates a new read-only instance of SelfDestructorFactory, bound to a specific deployed contract.
func NewSelfDestructorFactoryCaller(address common.Address, caller bind.ContractCaller) (*SelfDestructorFactoryCaller, error) {
	contract, err := bindSelfDestructorFactory(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &SelfDestructorFactoryCaller{contract: contract}, nil
}

// NewSelfDestructorFactoryTransactor creates a new write-only instance of SelfDestructorFactory, bound to a specific deployed contract.
func NewSelfDestructorFactoryTransactor(address common.Address, transactor bind.ContractTransactor) (*SelfDestructorFactoryTransactor, error) {
	contract, err := bindSelfDestructorFactory(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &SelfDestructorFactoryTransactor{contract: contract}, nil
}

// NewSelfDestructorFactoryFilterer creates a new log filterer instance of SelfDestructorFactory, bound to a specific deployed contract.
func NewSelfDestructorFactoryFilterer(address common.Address, filterer bind.ContractFilterer) (*SelfDestructorFactoryFilterer, error) {
	contract, err := bindSelfDestructorFactory(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &SelfDestructorFactoryFilterer{contract: contract}, nil
}

// bindSelfDestructorFactory binds a generic wrapper to an already deployed contract.
func bindSelfDestructorFactory(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := SelfDestructorFactoryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_SelfDestructorFactory *SelfDestructorFactoryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _SelfDestructorFactory.Contract.SelfDestructorFactoryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_SelfDestructorFactory *SelfDestructorFactoryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SelfDestructorFactory.Contract.SelfDestructorFactoryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_SelfDestructorFactory *SelfDestructorFactoryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _SelfDestructorFactory.Contract.SelfDestructorFactoryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_SelfDestructorFactory *SelfDestructorFactoryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _SelfDestructorFactory.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_SelfDestructorFactory *SelfDestructorFactoryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SelfDestructorFactory.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_SelfDestructorFactory *SelfDestructorFactoryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _SelfDestructorFactory.Contract.contract.Transact(opts, method, params...)
}

// ConstructedContract is a free data retrieval call binding the contract method 0x73d8000e.
//
// Solidity: function constructedContract() view returns(address)
func (_SelfDestructorFactory *SelfDestructorFactoryCaller) ConstructedContract(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _SelfDestructorFactory.contract.Call(opts, &out, "constructedContract")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// ConstructedContract is a free data retrieval call binding the contract method 0x73d8000e.
//
// Solidity: function constructedContract() view returns(address)
func (_SelfDestructorFactory *SelfDestructorFactorySession) ConstructedContract() (common.Address, error) {
	return _SelfDestructorFactory.Contract.ConstructedContract(&_SelfDestructorFactory.CallOpts)
}

// ConstructedContract is a free data retrieval call binding the contract method 0x73d8000e.
//
// Solidity: function constructedContract() view returns(address)
func (_SelfDestructorFactory *SelfDestructorFactoryCallerSession) ConstructedContract() (common.Address, error) {
	return _SelfDestructorFactory.Contract.ConstructedContract(&_SelfDestructorFactory.CallOpts)
}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(uint256)
func (_SelfDestructorFactory *SelfDestructorFactoryCaller) GetCount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _SelfDestructorFactory.contract.Call(opts, &out, "getCount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(uint256)
func (_SelfDestructorFactory *SelfDestructorFactorySession) GetCount() (*big.Int, error) {
	return _SelfDestructorFactory.Contract.GetCount(&_SelfDestructorFactory.CallOpts)
}

// GetCount is a free data retrieval call binding the contract method 0xa87d942c.
//
// Solidity: function getCount() view returns(uint256)
func (_SelfDestructorFactory *SelfDestructorFactoryCallerSession) GetCount() (*big.Int, error) {
	return _SelfDestructorFactory.Contract.GetCount(&_SelfDestructorFactory.CallOpts)
}

// DeployOrDestruct is a paid mutator transaction binding the contract method 0x3cb23967.
//
// Solidity: function deployOrDestruct() payable returns()
func (_SelfDestructorFactory *SelfDestructorFactoryTransactor) DeployOrDestruct(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SelfDestructorFactory.contract.Transact(opts, "deployOrDestruct")
}

// DeployOrDestruct is a paid mutator transaction binding the contract method 0x3cb23967.
//
// Solidity: function deployOrDestruct() payable returns()
func (_SelfDestructorFactory *SelfDestructorFactorySession) DeployOrDestruct() (*types.Transaction, error) {
	return _SelfDestructorFactory.Contract.DeployOrDestruct(&_SelfDestructorFactory.TransactOpts)
}

// DeployOrDestruct is a paid mutator transaction binding the contract method 0x3cb23967.
//
// Solidity: function deployOrDestruct() payable returns()
func (_SelfDestructorFactory *SelfDestructorFactoryTransactorSession) DeployOrDestruct() (*types.Transaction, error) {
	return _SelfDestructorFactory.Contract.DeployOrDestruct(&_SelfDestructorFactory.TransactOpts)
}
