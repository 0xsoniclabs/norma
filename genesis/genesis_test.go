package genesis

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	gas_subsidies_registry "github.com/0xsoniclabs/sonic/gossip/blockproc/subsidies/registry"
	"github.com/0xsoniclabs/sonic/integration/makefakegenesis"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/stretchr/testify/require"
)

// TestGenerateJsonGenesis_BalancesAreReadableByOlderClients guards the encoding
// of account balances. Every node imports the genesis file with the sonictool of
// its own image, and clients up to v2.1.6 decode balances into a big.Int, which
// rejects the string the current Sonic sources encode them as. This test decodes
// a generated file the way those clients do.
func TestGenerateJsonGenesis_BalancesAreReadableByOlderClients(t *testing.T) {
	require := require.New(t)

	serveLegacyRegistryCode(t, "6001600101")

	path := filepath.Join(t.TempDir(), "genesis.json")
	rules := opera.FakeNetRules(opera.GetSonicUpgrades())
	require.NoError(GenerateJsonGenesis(
		path, []uint64{5_000_000}, &rules, []string{"2.1.6"}))

	content, err := os.ReadFile(path)
	require.NoError(err)

	var genesis struct {
		Accounts []struct {
			Name    string
			Balance *big.Int
		}
	}
	require.NoError(json.Unmarshal(content, &genesis))

	funded := 0
	for _, account := range genesis.Accounts {
		if account.Balance != nil && account.Balance.Sign() > 0 {
			funded++
		}
	}
	require.NotZero(funded, "no funded account found in the generated genesis")
}

// TestGenerateJsonGenesis_InstallsTheRegistryOfTheOldestClient checks that the
// registry selected for the network's client versions reaches the genesis file.
func TestGenerateJsonGenesis_InstallsTheRegistryOfTheOldestClient(t *testing.T) {
	tests := map[string][]string{
		"legacy":  {"2.3.0-dev", "2.1.6"},
		"current": {"2.3.0-dev", "2.2.0"},
	}

	for name, clientVersions := range tests {
		t.Run(name, func(t *testing.T) {
			require := require.New(t)
			serveLegacyRegistryCode(t, "6001600101")

			path := filepath.Join(t.TempDir(), "genesis.json")
			rules := opera.FakeNetRules(opera.GetSonicUpgrades())
			require.NoError(GenerateJsonGenesis(
				path, []uint64{5_000_000}, &rules, clientVersions))

			content, err := os.ReadFile(path)
			require.NoError(err)

			var genesis struct {
				Accounts []makefakegenesis.Account
			}
			require.NoError(json.Unmarshal(content, &genesis))

			want, err := subsidiesRegistryCode(clientVersions)
			require.NoError(err)

			found := false
			for _, account := range genesis.Accounts {
				if account.Address != gas_subsidies_registry.GetAddress() {
					continue
				}
				require.Equal(want, []byte(account.Code))
				found = true
			}
			require.True(found, "no subsidies registry account in the generated genesis")
		})
	}
}

func TestEncodeBalancesAsNumbers_UnquotesBalancesOnly(t *testing.T) {
	in := []byte(`{
  "Accounts": [
    {
      "Name": "validator_1",
      "Balance": "10000000000000000000000000000000",
      "Nonce": 1
    }
  ],
  "Rules": {
    "Name": "fake",
    "Economy": {
      "MinGasPrice": 1000000000
    }
  }
}`)
	want := []byte(`{
  "Accounts": [
    {
      "Name": "validator_1",
      "Balance": 10000000000000000000000000000000,
      "Nonce": 1
    }
  ],
  "Rules": {
    "Name": "fake",
    "Economy": {
      "MinGasPrice": 1000000000
    }
  }
}`)
	require.Equal(t, string(want), string(encodeBalancesAsNumbers(in)))
}
