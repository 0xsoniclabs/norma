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
// Source: shaper.go

// Package shaper is a generated GoMock package.
package shaper

import (
	reflect "reflect"
	time "time"

	gomock "go.uber.org/mock/gomock"
)

// MockShaper is a mock of Shaper interface.
type MockShaper struct {
	ctrl     *gomock.Controller
	recorder *MockShaperMockRecorder
}

// MockShaperMockRecorder is the mock recorder for MockShaper.
type MockShaperMockRecorder struct {
	mock *MockShaper
}

// NewMockShaper creates a new mock instance.
func NewMockShaper(ctrl *gomock.Controller) *MockShaper {
	mock := &MockShaper{ctrl: ctrl}
	mock.recorder = &MockShaperMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockShaper) EXPECT() *MockShaperMockRecorder {
	return m.recorder
}

// GetNumMessagesInInterval mocks base method.
func (m *MockShaper) GetNumMessagesInInterval(start time.Time, duration time.Duration) float64 {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetNumMessagesInInterval", start, duration)
	ret0, _ := ret[0].(float64)
	return ret0
}

// GetNumMessagesInInterval indicates an expected call of GetNumMessagesInInterval.
func (mr *MockShaperMockRecorder) GetNumMessagesInInterval(start, duration interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetNumMessagesInInterval", reflect.TypeOf((*MockShaper)(nil).GetNumMessagesInInterval), start, duration)
}

// Start mocks base method.
func (m *MockShaper) Start(arg0 time.Time, arg1 LoadInfoSource) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Start", arg0, arg1)
}

// Start indicates an expected call of Start.
func (mr *MockShaperMockRecorder) Start(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Start", reflect.TypeOf((*MockShaper)(nil).Start), arg0, arg1)
}

// MockLoadInfoSource is a mock of LoadInfoSource interface.
type MockLoadInfoSource struct {
	ctrl     *gomock.Controller
	recorder *MockLoadInfoSourceMockRecorder
}

// MockLoadInfoSourceMockRecorder is the mock recorder for MockLoadInfoSource.
type MockLoadInfoSourceMockRecorder struct {
	mock *MockLoadInfoSource
}

// NewMockLoadInfoSource creates a new mock instance.
func NewMockLoadInfoSource(ctrl *gomock.Controller) *MockLoadInfoSource {
	mock := &MockLoadInfoSource{ctrl: ctrl}
	mock.recorder = &MockLoadInfoSourceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockLoadInfoSource) EXPECT() *MockLoadInfoSourceMockRecorder {
	return m.recorder
}

// GetReceivedTransactions mocks base method.
func (m *MockLoadInfoSource) GetReceivedTransactions() (uint64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetReceivedTransactions")
	ret0, _ := ret[0].(uint64)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetReceivedTransactions indicates an expected call of GetReceivedTransactions.
func (mr *MockLoadInfoSourceMockRecorder) GetReceivedTransactions() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetReceivedTransactions", reflect.TypeOf((*MockLoadInfoSource)(nil).GetReceivedTransactions))
}

// GetSentTransactions mocks base method.
func (m *MockLoadInfoSource) GetSentTransactions() (uint64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSentTransactions")
	ret0, _ := ret[0].(uint64)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetSentTransactions indicates an expected call of GetSentTransactions.
func (mr *MockLoadInfoSourceMockRecorder) GetSentTransactions() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSentTransactions", reflect.TypeOf((*MockLoadInfoSource)(nil).GetSentTransactions))
}
