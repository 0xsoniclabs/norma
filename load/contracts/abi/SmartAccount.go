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

// SmartAccountCall is an auto generated low-level Go binding around an user-defined struct.
type SmartAccountCall struct {
	To    common.Address
	Value *big.Int
	Data  []byte
}

// SmartAccountMetaData contains all meta data concerning the SmartAccount contract.
var SmartAccountMetaData = &bind.MetaData{
	ABI: "[{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"components\":[{\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"},{\"internalType\":\"bytes\",\"name\":\"data\",\"type\":\"bytes\"}],\"indexed\":false,\"internalType\":\"structSmartAccount.Call[]\",\"name\":\"calls\",\"type\":\"tuple[]\"}],\"name\":\"BatchExecuted\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"bytes\",\"name\":\"data\",\"type\":\"bytes\"}],\"name\":\"CallExecuted\",\"type\":\"event\"},{\"stateMutability\":\"payable\",\"type\":\"fallback\"},{\"inputs\":[{\"components\":[{\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"},{\"internalType\":\"bytes\",\"name\":\"data\",\"type\":\"bytes\"}],\"internalType\":\"structSmartAccount.Call[]\",\"name\":\"calls\",\"type\":\"tuple[]\"}],\"name\":\"execute\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"nonce\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"stateMutability\":\"payable\",\"type\":\"receive\"}]",
	Bin: "0x608060405234801561001057600080fd5b5061050c806100206000396000f3fe60806040526004361061002a5760003560e01c80633f707e6b14610033578063affed0e01461004657005b3661003157005b005b61003161004136600461023f565b61006e565b34801561005257600080fd5b5061005c60005481565b60405190815260200160405180910390f35b610078828261007c565b5050565b600080549081908061008d836102b4565b919050555060005b828110156100db576100c98484838181106100b2576100b26102db565b90506020028101906100c491906102f1565b61011b565b806100d3816102b4565b915050610095565b50807f280bb3599696acbf79fb8ffcde81a57337b52500f789600fbb1cff9b4cbaba39848460405161010e929190610356565b60405180910390a2505050565b600061012a6020830183610433565b6001600160a01b031660208301356101456040850185610455565b6040516101539291906104a3565b60006040518083038185875af1925050503d8060008114610190576040519150601f19603f3d011682016040523d82523d6000602084013e610195565b606091505b50509050806101da5760405162461bcd60e51b815260206004820152600d60248201526c10d85b1b081c995d995c9d1959609a1b604482015260640160405180910390fd5b6101e76020830183610433565b6001600160a01b0316337fed7e8f919df9cc0d0ad8b4057d084ebf319b630564d5da283e14751adc931f3a60208501356102246040870187610455565b604051610233939291906104b3565b60405180910390a35050565b6000806020838503121561025257600080fd5b823567ffffffffffffffff8082111561026a57600080fd5b818501915085601f83011261027e57600080fd5b81358181111561028d57600080fd5b8660208260051b85010111156102a257600080fd5b60209290920196919550909350505050565b6000600182016102d457634e487b7160e01b600052601160045260246000fd5b5060010190565b634e487b7160e01b600052603260045260246000fd5b60008235605e1983360301811261030757600080fd5b9190910192915050565b80356001600160a01b038116811461032857600080fd5b919050565b81835281816020850137506000828201602090810191909152601f909101601f19169091010190565b60208082528181018390526000906040808401600586901b8501820187855b8881101561042557878303603f190184528135368b9003605e1901811261039b57600080fd5b8a0160606001600160a01b036103b083610311565b168552878201358886015286820135601e198336030181126103d157600080fd5b90910187810191903567ffffffffffffffff8111156103ef57600080fd5b8036038313156103fe57600080fd5b8188870152610410828701828561032d565b96890196955050509186019150600101610375565b509098975050505050505050565b60006020828403121561044557600080fd5b61044e82610311565b9392505050565b6000808335601e1984360301811261046c57600080fd5b83018035915067ffffffffffffffff82111561048757600080fd5b60200191503681900382131561049c57600080fd5b9250929050565b8183823760009101908152919050565b8381526040602082015260006104cd60408301848661032d565b9594505050505056fea26469706673582212205f7b43460b082bfde02f1335e4f343d121499dfde73228435d8cb0120d06d44d64736f6c63430008130033",
}

// SmartAccountABI is the input ABI used to generate the binding from.
// Deprecated: Use SmartAccountMetaData.ABI instead.
var SmartAccountABI = SmartAccountMetaData.ABI

// SmartAccountBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use SmartAccountMetaData.Bin instead.
var SmartAccountBin = SmartAccountMetaData.Bin

// DeploySmartAccount deploys a new Ethereum contract, binding an instance of SmartAccount to it.
func DeploySmartAccount(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *SmartAccount, error) {
	parsed, err := SmartAccountMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(SmartAccountBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &SmartAccount{SmartAccountCaller: SmartAccountCaller{contract: contract}, SmartAccountTransactor: SmartAccountTransactor{contract: contract}, SmartAccountFilterer: SmartAccountFilterer{contract: contract}}, nil
}

// SmartAccount is an auto generated Go binding around an Ethereum contract.
type SmartAccount struct {
	SmartAccountCaller     // Read-only binding to the contract
	SmartAccountTransactor // Write-only binding to the contract
	SmartAccountFilterer   // Log filterer for contract events
}

// SmartAccountCaller is an auto generated read-only Go binding around an Ethereum contract.
type SmartAccountCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SmartAccountTransactor is an auto generated write-only Go binding around an Ethereum contract.
type SmartAccountTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SmartAccountFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type SmartAccountFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SmartAccountSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type SmartAccountSession struct {
	Contract     *SmartAccount     // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// SmartAccountCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type SmartAccountCallerSession struct {
	Contract *SmartAccountCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts       // Call options to use throughout this session
}

// SmartAccountTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type SmartAccountTransactorSession struct {
	Contract     *SmartAccountTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts       // Transaction auth options to use throughout this session
}

// SmartAccountRaw is an auto generated low-level Go binding around an Ethereum contract.
type SmartAccountRaw struct {
	Contract *SmartAccount // Generic contract binding to access the raw methods on
}

// SmartAccountCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type SmartAccountCallerRaw struct {
	Contract *SmartAccountCaller // Generic read-only contract binding to access the raw methods on
}

// SmartAccountTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type SmartAccountTransactorRaw struct {
	Contract *SmartAccountTransactor // Generic write-only contract binding to access the raw methods on
}

// NewSmartAccount creates a new instance of SmartAccount, bound to a specific deployed contract.
func NewSmartAccount(address common.Address, backend bind.ContractBackend) (*SmartAccount, error) {
	contract, err := bindSmartAccount(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &SmartAccount{SmartAccountCaller: SmartAccountCaller{contract: contract}, SmartAccountTransactor: SmartAccountTransactor{contract: contract}, SmartAccountFilterer: SmartAccountFilterer{contract: contract}}, nil
}

// NewSmartAccountCaller creates a new read-only instance of SmartAccount, bound to a specific deployed contract.
func NewSmartAccountCaller(address common.Address, caller bind.ContractCaller) (*SmartAccountCaller, error) {
	contract, err := bindSmartAccount(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &SmartAccountCaller{contract: contract}, nil
}

// NewSmartAccountTransactor creates a new write-only instance of SmartAccount, bound to a specific deployed contract.
func NewSmartAccountTransactor(address common.Address, transactor bind.ContractTransactor) (*SmartAccountTransactor, error) {
	contract, err := bindSmartAccount(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &SmartAccountTransactor{contract: contract}, nil
}

// NewSmartAccountFilterer creates a new log filterer instance of SmartAccount, bound to a specific deployed contract.
func NewSmartAccountFilterer(address common.Address, filterer bind.ContractFilterer) (*SmartAccountFilterer, error) {
	contract, err := bindSmartAccount(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &SmartAccountFilterer{contract: contract}, nil
}

// bindSmartAccount binds a generic wrapper to an already deployed contract.
func bindSmartAccount(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := SmartAccountMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_SmartAccount *SmartAccountRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _SmartAccount.Contract.SmartAccountCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_SmartAccount *SmartAccountRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SmartAccount.Contract.SmartAccountTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_SmartAccount *SmartAccountRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _SmartAccount.Contract.SmartAccountTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_SmartAccount *SmartAccountCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _SmartAccount.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_SmartAccount *SmartAccountTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SmartAccount.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_SmartAccount *SmartAccountTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _SmartAccount.Contract.contract.Transact(opts, method, params...)
}

// Nonce is a free data retrieval call binding the contract method 0xaffed0e0.
//
// Solidity: function nonce() view returns(uint256)
func (_SmartAccount *SmartAccountCaller) Nonce(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _SmartAccount.contract.Call(opts, &out, "nonce")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Nonce is a free data retrieval call binding the contract method 0xaffed0e0.
//
// Solidity: function nonce() view returns(uint256)
func (_SmartAccount *SmartAccountSession) Nonce() (*big.Int, error) {
	return _SmartAccount.Contract.Nonce(&_SmartAccount.CallOpts)
}

// Nonce is a free data retrieval call binding the contract method 0xaffed0e0.
//
// Solidity: function nonce() view returns(uint256)
func (_SmartAccount *SmartAccountCallerSession) Nonce() (*big.Int, error) {
	return _SmartAccount.Contract.Nonce(&_SmartAccount.CallOpts)
}

// Execute is a paid mutator transaction binding the contract method 0x3f707e6b.
//
// Solidity: function execute((address,uint256,bytes)[] calls) payable returns()
func (_SmartAccount *SmartAccountTransactor) Execute(opts *bind.TransactOpts, calls []SmartAccountCall) (*types.Transaction, error) {
	return _SmartAccount.contract.Transact(opts, "execute", calls)
}

// Execute is a paid mutator transaction binding the contract method 0x3f707e6b.
//
// Solidity: function execute((address,uint256,bytes)[] calls) payable returns()
func (_SmartAccount *SmartAccountSession) Execute(calls []SmartAccountCall) (*types.Transaction, error) {
	return _SmartAccount.Contract.Execute(&_SmartAccount.TransactOpts, calls)
}

// Execute is a paid mutator transaction binding the contract method 0x3f707e6b.
//
// Solidity: function execute((address,uint256,bytes)[] calls) payable returns()
func (_SmartAccount *SmartAccountTransactorSession) Execute(calls []SmartAccountCall) (*types.Transaction, error) {
	return _SmartAccount.Contract.Execute(&_SmartAccount.TransactOpts, calls)
}

// Fallback is a paid mutator transaction binding the contract fallback function.
//
// Solidity: fallback() payable returns()
func (_SmartAccount *SmartAccountTransactor) Fallback(opts *bind.TransactOpts, calldata []byte) (*types.Transaction, error) {
	return _SmartAccount.contract.RawTransact(opts, calldata)
}

// Fallback is a paid mutator transaction binding the contract fallback function.
//
// Solidity: fallback() payable returns()
func (_SmartAccount *SmartAccountSession) Fallback(calldata []byte) (*types.Transaction, error) {
	return _SmartAccount.Contract.Fallback(&_SmartAccount.TransactOpts, calldata)
}

// Fallback is a paid mutator transaction binding the contract fallback function.
//
// Solidity: fallback() payable returns()
func (_SmartAccount *SmartAccountTransactorSession) Fallback(calldata []byte) (*types.Transaction, error) {
	return _SmartAccount.Contract.Fallback(&_SmartAccount.TransactOpts, calldata)
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_SmartAccount *SmartAccountTransactor) Receive(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SmartAccount.contract.RawTransact(opts, nil) // calldata is disallowed for receive function
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_SmartAccount *SmartAccountSession) Receive() (*types.Transaction, error) {
	return _SmartAccount.Contract.Receive(&_SmartAccount.TransactOpts)
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_SmartAccount *SmartAccountTransactorSession) Receive() (*types.Transaction, error) {
	return _SmartAccount.Contract.Receive(&_SmartAccount.TransactOpts)
}

// SmartAccountBatchExecutedIterator is returned from FilterBatchExecuted and is used to iterate over the raw logs and unpacked data for BatchExecuted events raised by the SmartAccount contract.
type SmartAccountBatchExecutedIterator struct {
	Event *SmartAccountBatchExecuted // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SmartAccountBatchExecutedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SmartAccountBatchExecuted)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SmartAccountBatchExecuted)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SmartAccountBatchExecutedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SmartAccountBatchExecutedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SmartAccountBatchExecuted represents a BatchExecuted event raised by the SmartAccount contract.
type SmartAccountBatchExecuted struct {
	Nonce *big.Int
	Calls []SmartAccountCall
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterBatchExecuted is a free log retrieval operation binding the contract event 0x280bb3599696acbf79fb8ffcde81a57337b52500f789600fbb1cff9b4cbaba39.
//
// Solidity: event BatchExecuted(uint256 indexed nonce, (address,uint256,bytes)[] calls)
func (_SmartAccount *SmartAccountFilterer) FilterBatchExecuted(opts *bind.FilterOpts, nonce []*big.Int) (*SmartAccountBatchExecutedIterator, error) {

	var nonceRule []interface{}
	for _, nonceItem := range nonce {
		nonceRule = append(nonceRule, nonceItem)
	}

	logs, sub, err := _SmartAccount.contract.FilterLogs(opts, "BatchExecuted", nonceRule)
	if err != nil {
		return nil, err
	}
	return &SmartAccountBatchExecutedIterator{contract: _SmartAccount.contract, event: "BatchExecuted", logs: logs, sub: sub}, nil
}

// WatchBatchExecuted is a free log subscription operation binding the contract event 0x280bb3599696acbf79fb8ffcde81a57337b52500f789600fbb1cff9b4cbaba39.
//
// Solidity: event BatchExecuted(uint256 indexed nonce, (address,uint256,bytes)[] calls)
func (_SmartAccount *SmartAccountFilterer) WatchBatchExecuted(opts *bind.WatchOpts, sink chan<- *SmartAccountBatchExecuted, nonce []*big.Int) (event.Subscription, error) {

	var nonceRule []interface{}
	for _, nonceItem := range nonce {
		nonceRule = append(nonceRule, nonceItem)
	}

	logs, sub, err := _SmartAccount.contract.WatchLogs(opts, "BatchExecuted", nonceRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SmartAccountBatchExecuted)
				if err := _SmartAccount.contract.UnpackLog(event, "BatchExecuted", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseBatchExecuted is a log parse operation binding the contract event 0x280bb3599696acbf79fb8ffcde81a57337b52500f789600fbb1cff9b4cbaba39.
//
// Solidity: event BatchExecuted(uint256 indexed nonce, (address,uint256,bytes)[] calls)
func (_SmartAccount *SmartAccountFilterer) ParseBatchExecuted(log types.Log) (*SmartAccountBatchExecuted, error) {
	event := new(SmartAccountBatchExecuted)
	if err := _SmartAccount.contract.UnpackLog(event, "BatchExecuted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SmartAccountCallExecutedIterator is returned from FilterCallExecuted and is used to iterate over the raw logs and unpacked data for CallExecuted events raised by the SmartAccount contract.
type SmartAccountCallExecutedIterator struct {
	Event *SmartAccountCallExecuted // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SmartAccountCallExecutedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SmartAccountCallExecuted)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SmartAccountCallExecuted)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SmartAccountCallExecutedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SmartAccountCallExecutedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SmartAccountCallExecuted represents a CallExecuted event raised by the SmartAccount contract.
type SmartAccountCallExecuted struct {
	Sender common.Address
	To     common.Address
	Value  *big.Int
	Data   []byte
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterCallExecuted is a free log retrieval operation binding the contract event 0xed7e8f919df9cc0d0ad8b4057d084ebf319b630564d5da283e14751adc931f3a.
//
// Solidity: event CallExecuted(address indexed sender, address indexed to, uint256 value, bytes data)
func (_SmartAccount *SmartAccountFilterer) FilterCallExecuted(opts *bind.FilterOpts, sender []common.Address, to []common.Address) (*SmartAccountCallExecutedIterator, error) {

	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _SmartAccount.contract.FilterLogs(opts, "CallExecuted", senderRule, toRule)
	if err != nil {
		return nil, err
	}
	return &SmartAccountCallExecutedIterator{contract: _SmartAccount.contract, event: "CallExecuted", logs: logs, sub: sub}, nil
}

// WatchCallExecuted is a free log subscription operation binding the contract event 0xed7e8f919df9cc0d0ad8b4057d084ebf319b630564d5da283e14751adc931f3a.
//
// Solidity: event CallExecuted(address indexed sender, address indexed to, uint256 value, bytes data)
func (_SmartAccount *SmartAccountFilterer) WatchCallExecuted(opts *bind.WatchOpts, sink chan<- *SmartAccountCallExecuted, sender []common.Address, to []common.Address) (event.Subscription, error) {

	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _SmartAccount.contract.WatchLogs(opts, "CallExecuted", senderRule, toRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SmartAccountCallExecuted)
				if err := _SmartAccount.contract.UnpackLog(event, "CallExecuted", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseCallExecuted is a log parse operation binding the contract event 0xed7e8f919df9cc0d0ad8b4057d084ebf319b630564d5da283e14751adc931f3a.
//
// Solidity: event CallExecuted(address indexed sender, address indexed to, uint256 value, bytes data)
func (_SmartAccount *SmartAccountFilterer) ParseCallExecuted(log types.Log) (*SmartAccountCallExecuted, error) {
	event := new(SmartAccountCallExecuted)
	if err := _SmartAccount.contract.UnpackLog(event, "CallExecuted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
