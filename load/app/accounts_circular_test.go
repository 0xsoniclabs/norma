package app

import (
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/mock/gomock"
	"math/big"
	"testing"
)

func TestAccountsCircularPool(t *testing.T) {
	ctrl := gomock.NewController(t)
	rpcClient := rpc.NewMockClient(ctrl)
	rpcClient.EXPECT().NonceAt(gomock.Any(), gomock.Any(), gomock.Any()).Return(uint64(0), nil).AnyTimes()

	accountFactory, err := NewAccountFactory(big.NewInt(123), 45, 67)
	if err != nil {
		t.Fatal(err)
	}
	circularPool, err := NewAccountsCircularPool(accountFactory, rpcClient, 5)
	if err != nil {
		t.Fatal(err)
	}

	firstSet, err := circularPool.GetAccounts(3)
	if err != nil {
		t.Fatal(err)
	}

	secondSet, err := circularPool.GetAccounts(3)
	if err != nil {
		t.Fatal(err)
	}

	allSet := append(firstSet, secondSet...)

	if !isUnique(allSet[0:5]) {
		t.Fatalf("not circular")
	}
	if firstSet[0].address != secondSet[2].address {
		t.Fatalf("not circular")
	}
}

func isUnique(accounts []*Account) bool {
	existing := make(map[common.Address]bool)
	for _, account := range accounts {
		if existing[account.address] {
			return false
		}
		existing[account.address] = true
	}
	return true
}
