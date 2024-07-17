// Copyright 2024 Fantom Foundation
// This file is part of Norma System Testing Infrastructure for Sonic.
//
// Norma is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Norma is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with Norma. If not, see <http://www.gnu.org/licenses/>.

// Code generated by MockGen. DO NOT EDIT.
// Source: rpc.go

// Package rpc is a generated GoMock package.
package rpc

import (
	context "context"
	big "math/big"
	reflect "reflect"

	ethereum "github.com/ethereum/go-ethereum"
	common "github.com/ethereum/go-ethereum/common"
	types "github.com/ethereum/go-ethereum/core/types"
	gomock "go.uber.org/mock/gomock"
)

// MockRpcClient is a mock of RpcClient interface.
type MockRpcClient struct {
	ctrl     *gomock.Controller
	recorder *MockRpcClientMockRecorder
}

// MockRpcClientMockRecorder is the mock recorder for MockRpcClient.
type MockRpcClientMockRecorder struct {
	mock *MockRpcClient
}

// NewMockRpcClient creates a new mock instance.
func NewMockRpcClient(ctrl *gomock.Controller) *MockRpcClient {
	mock := &MockRpcClient{ctrl: ctrl}
	mock.recorder = &MockRpcClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockRpcClient) EXPECT() *MockRpcClientMockRecorder {
	return m.recorder
}

// BalanceAt mocks base method.
func (m *MockRpcClient) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "BalanceAt", ctx, account, blockNumber)
	ret0, _ := ret[0].(*big.Int)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// BalanceAt indicates an expected call of BalanceAt.
func (mr *MockRpcClientMockRecorder) BalanceAt(ctx, account, blockNumber interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BalanceAt", reflect.TypeOf((*MockRpcClient)(nil).BalanceAt), ctx, account, blockNumber)
}

// Call mocks base method.
func (m *MockRpcClient) Call(result interface{}, method string, args ...interface{}) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{result, method}
	for _, a := range args {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Call", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// Call indicates an expected call of Call.
func (mr *MockRpcClientMockRecorder) Call(result, method interface{}, args ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{result, method}, args...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Call", reflect.TypeOf((*MockRpcClient)(nil).Call), varargs...)
}

// CallContract mocks base method.
func (m *MockRpcClient) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CallContract", ctx, call, blockNumber)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CallContract indicates an expected call of CallContract.
func (mr *MockRpcClientMockRecorder) CallContract(ctx, call, blockNumber interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CallContract", reflect.TypeOf((*MockRpcClient)(nil).CallContract), ctx, call, blockNumber)
}

// Close mocks base method.
func (m *MockRpcClient) Close() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Close")
}

// Close indicates an expected call of Close.
func (mr *MockRpcClientMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockRpcClient)(nil).Close))
}

// CodeAt mocks base method.
func (m *MockRpcClient) CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CodeAt", ctx, contract, blockNumber)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CodeAt indicates an expected call of CodeAt.
func (mr *MockRpcClientMockRecorder) CodeAt(ctx, contract, blockNumber interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CodeAt", reflect.TypeOf((*MockRpcClient)(nil).CodeAt), ctx, contract, blockNumber)
}

// EstimateGas mocks base method.
func (m *MockRpcClient) EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "EstimateGas", ctx, call)
	ret0, _ := ret[0].(uint64)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// EstimateGas indicates an expected call of EstimateGas.
func (mr *MockRpcClientMockRecorder) EstimateGas(ctx, call interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EstimateGas", reflect.TypeOf((*MockRpcClient)(nil).EstimateGas), ctx, call)
}

// FilterLogs mocks base method.
func (m *MockRpcClient) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FilterLogs", ctx, query)
	ret0, _ := ret[0].([]types.Log)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FilterLogs indicates an expected call of FilterLogs.
func (mr *MockRpcClientMockRecorder) FilterLogs(ctx, query interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FilterLogs", reflect.TypeOf((*MockRpcClient)(nil).FilterLogs), ctx, query)
}

// HeaderByNumber mocks base method.
func (m *MockRpcClient) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HeaderByNumber", ctx, number)
	ret0, _ := ret[0].(*types.Header)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// HeaderByNumber indicates an expected call of HeaderByNumber.
func (mr *MockRpcClientMockRecorder) HeaderByNumber(ctx, number interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HeaderByNumber", reflect.TypeOf((*MockRpcClient)(nil).HeaderByNumber), ctx, number)
}

// NonceAt mocks base method.
func (m *MockRpcClient) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NonceAt", ctx, account, blockNumber)
	ret0, _ := ret[0].(uint64)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NonceAt indicates an expected call of NonceAt.
func (mr *MockRpcClientMockRecorder) NonceAt(ctx, account, blockNumber interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NonceAt", reflect.TypeOf((*MockRpcClient)(nil).NonceAt), ctx, account, blockNumber)
}

// PendingCodeAt mocks base method.
func (m *MockRpcClient) PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PendingCodeAt", ctx, account)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// PendingCodeAt indicates an expected call of PendingCodeAt.
func (mr *MockRpcClientMockRecorder) PendingCodeAt(ctx, account interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PendingCodeAt", reflect.TypeOf((*MockRpcClient)(nil).PendingCodeAt), ctx, account)
}

// PendingNonceAt mocks base method.
func (m *MockRpcClient) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PendingNonceAt", ctx, account)
	ret0, _ := ret[0].(uint64)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// PendingNonceAt indicates an expected call of PendingNonceAt.
func (mr *MockRpcClientMockRecorder) PendingNonceAt(ctx, account interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PendingNonceAt", reflect.TypeOf((*MockRpcClient)(nil).PendingNonceAt), ctx, account)
}

// SendTransaction mocks base method.
func (m *MockRpcClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SendTransaction", ctx, tx)
	ret0, _ := ret[0].(error)
	return ret0
}

// SendTransaction indicates an expected call of SendTransaction.
func (mr *MockRpcClientMockRecorder) SendTransaction(ctx, tx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SendTransaction", reflect.TypeOf((*MockRpcClient)(nil).SendTransaction), ctx, tx)
}

// SubscribeFilterLogs mocks base method.
func (m *MockRpcClient) SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SubscribeFilterLogs", ctx, query, ch)
	ret0, _ := ret[0].(ethereum.Subscription)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SubscribeFilterLogs indicates an expected call of SubscribeFilterLogs.
func (mr *MockRpcClientMockRecorder) SubscribeFilterLogs(ctx, query, ch interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SubscribeFilterLogs", reflect.TypeOf((*MockRpcClient)(nil).SubscribeFilterLogs), ctx, query, ch)
}

// SuggestGasPrice mocks base method.
func (m *MockRpcClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SuggestGasPrice", ctx)
	ret0, _ := ret[0].(*big.Int)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SuggestGasPrice indicates an expected call of SuggestGasPrice.
func (mr *MockRpcClientMockRecorder) SuggestGasPrice(ctx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SuggestGasPrice", reflect.TypeOf((*MockRpcClient)(nil).SuggestGasPrice), ctx)
}

// SuggestGasTipCap mocks base method.
func (m *MockRpcClient) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SuggestGasTipCap", ctx)
	ret0, _ := ret[0].(*big.Int)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SuggestGasTipCap indicates an expected call of SuggestGasTipCap.
func (mr *MockRpcClientMockRecorder) SuggestGasTipCap(ctx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SuggestGasTipCap", reflect.TypeOf((*MockRpcClient)(nil).SuggestGasTipCap), ctx)
}
