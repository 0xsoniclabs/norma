package app

import (
	"math/big"
	"testing"

	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/mock/gomock"
)

func TestAccount_CreateAccount_AccountsUniq(t *testing.T) {
	const loops = 100

	ctrl := gomock.NewController(t)
	rpcClient := rpc.NewMockClient(ctrl)
	rpcClient.EXPECT().ChainID(gomock.Any()).AnyTimes().Return(big.NewInt(0xFA), nil)
	rpcClient.EXPECT().PendingNonceAt(gomock.Any(), gomock.Any()).AnyTimes().Return(uint64(0), nil)

	accounts := make(map[common.Address]struct{}, loops)
	for i := 0; i < loops; i++ {
		gen := NewAccountFactory(0, uint32(i))

		for j := 0; j < loops; j++ {
			account, err := gen.CreateAccount(rpcClient)
			if err != nil {
				t.Fatalf("cannot create account: %v", err)
			}

			if _, ok := accounts[account.address]; ok {
				t.Errorf("account address %v is not unique", account.address)
			}

			accounts[account.address] = struct{}{}
		}
	}

}
