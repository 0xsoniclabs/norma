package app

import (
	"fmt"
	"github.com/0xsoniclabs/norma/driver/rpc"
)

type AccountsCircular struct {
	accounts []*Account
	current  int
}

func NewAccountsCircular(accountFactory *AccountFactory, rpcClient rpc.Client, size int) (*AccountsCircular, error) {
	accounts := make([]*Account, 0, size)
	for i := 0; i < size; i++ {
		acc, err := accountFactory.CreateAccount(rpcClient)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, acc)
	}
	return &AccountsCircular{
		accounts: accounts,
		current:  0,
	}, nil
}

func (c *AccountsCircular) GetAccounts(count int) ([]*Account, error) {
	if count > len(c.accounts) {
		return nil, fmt.Errorf("requested more accounts (%d) than available in the circular registry (%d)", count, len(c.accounts))
	}
	chosen := make([]*Account, 0, count)
	for len(chosen) < count {
		chosen = append(chosen, c.accounts[c.current])
		c.current = (c.current + 1) % len(c.accounts)
	}
	return chosen, nil
}
