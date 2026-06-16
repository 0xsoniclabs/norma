package genesis

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xsoniclabs/norma/load/accounts"
	"github.com/0xsoniclabs/sonic/integration/makefakegenesis"
	"github.com/0xsoniclabs/sonic/opera"
)

func TestGenerateJsonGenesis_ContainsPrefundedLoadAccounts(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "genesis.json")

	rules := opera.FakeNetRules(opera.GetSonicUpgrades())
	if err := GenerateJsonGenesis(path, []uint64{5_000_000}, &rules); err != nil {
		t.Fatalf("failed to generate genesis: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read genesis file: %v", err)
	}

	decoded := makefakegenesis.GenesisJson{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("failed to decode genesis file: %v", err)
	}

	balances := map[string]*big.Int{}
	for _, account := range decoded.Accounts {
		balances[account.Address.Hex()] = account.Balance
	}

	for i := uint64(0); i < accounts.PrefundedAccountsCount; i++ {
		address, err := accounts.DeriveAddress(i)
		if err != nil {
			t.Fatalf("failed to derive prefunded address %d: %v", i, err)
		}

		balance, exists := balances[address.Hex()]
		if !exists {
			t.Fatalf("prefunded address missing from genesis: %s", address)
		}
		if balance == nil {
			t.Fatalf("prefunded address has nil balance: %s", address)
		}
		if balance.Cmp(accounts.PrefundedAccountBalanceWei) != 0 {
			t.Fatalf("invalid balance for %s: got %s, want %s", address, balance, accounts.PrefundedAccountBalanceWei)
		}
	}
}
