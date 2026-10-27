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

package controller

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/norma/load/app"
	"github.com/0xsoniclabs/norma/load/shaper"
	"github.com/ethereum/go-ethereum/core/types"
	"go.uber.org/mock/gomock"
)

func TestMockedTrafficGenerating(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	var demoTx types.Transaction

	numUsers := 2
	mockUser := app.NewMockUser(mockCtrl)

	// The client of the app context is shared by all apps, so it is not closed.
	mockedRpcClient := rpc.NewMockClient(mockCtrl)

	appContext := app.NewMockAppContext(mockCtrl)
	appContext.EXPECT().GetClient().Return(mockedRpcClient).AnyTimes()

	mockedNetwork := driver.NewMockNetwork(mockCtrl)

	mockedApp := app.NewMockApplication(mockCtrl)
	mockedApp.EXPECT().CreateUsers(appContext, numUsers).Return([]app.User{mockUser, mockUser}, nil)

	// app should be called 10-times to generate 10 txs
	mockUser.EXPECT().GenerateTx().Return(&demoTx, nil).MinTimes(5).MaxTimes(11)
	// network should be called 10-times to send 10 txs
	mockedNetwork.EXPECT().SendTransaction(&demoTx, gomock.Any()).MinTimes(5).MaxTimes(11)

	// use constant shaper
	constantShaper := shaper.NewConstantShaper(100) // 100 txs/sec

	appController, err := NewAppController("test-app", mockedApp, constantShaper, numUsers, appContext, mockedNetwork)
	if err != nil {
		t.Fatal(err)
	}

	// let the app run for 100 ms - should give 10 txs
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	// note: Run is supposed to run in a new thread
	err = appController.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStoppingAnAppKeepsTheOthersSending(t *testing.T) {
	ctrl := gomock.NewController(t)

	// The shared client has no expectations: closing it fails the test.
	shared := rpc.NewMockClient(ctrl)
	appContext := app.NewMockAppContext(ctrl)
	appContext.EXPECT().GetClient().Return(shared).AnyTimes()

	own := rpc.NewMockClient(ctrl)
	own.EXPECT().Close().AnyTimes()
	network := driver.NewMockNetwork(ctrl)
	network.EXPECT().DialRandomRpc().Return(own, nil).AnyTimes()

	var demoTx types.Transaction
	var sentByB atomic.Int64
	network.EXPECT().SendTransaction(&demoTx, gomock.Any()).AnyTimes().Do(
		func(_ *types.Transaction, source driver.TransactionSource) {
			if source.App == "B" {
				sentByB.Add(1)
			}
		})

	newController := func(name string, shaper shaper.Shaper) *AppController {
		user := app.NewMockUser(ctrl)
		user.EXPECT().GenerateTx().Return(&demoTx, nil).AnyTimes()
		user.EXPECT().GetSentTransactions().Return(uint64(0)).AnyTimes()
		application := app.NewMockApplication(ctrl)
		application.EXPECT().CreateUsers(appContext, 1).Return([]app.User{user}, nil)
		application.EXPECT().GetReceivedTransactions(own).Return(uint64(0), nil).AnyTimes()
		controller, err := NewAppController(name, application, shaper, 1, appContext, network)
		if err != nil {
			t.Fatal(err)
		}
		return controller
	}
	a := newController("A", shaper.NewConstantShaper(100))
	b := newController("B", shaper.NewAutoShaper(100, 0.2))

	ctxA, stopA := context.WithCancel(t.Context())
	ctxB, stopB := context.WithCancel(t.Context())
	var running sync.WaitGroup
	running.Go(func() { _ = a.Run(ctxA) })
	running.Go(func() { _ = b.Run(ctxB) })
	// The monitoring reads the received transactions while the shaper does.
	running.Go(func() {
		for ctxB.Err() == nil {
			_, _ = b.GetReceivedTransactions()
			time.Sleep(time.Millisecond)
		}
	})

	time.Sleep(100 * time.Millisecond)
	stopA()
	before := sentByB.Load()
	time.Sleep(1500 * time.Millisecond)
	stopB()
	running.Wait()

	if sentByB.Load() == before {
		t.Errorf("B stopped sending after A was stopped")
	}
}
