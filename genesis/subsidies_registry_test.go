package genesis

import (
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	gas_subsidies_registry "github.com/0xsoniclabs/sonic/gossip/blockproc/subsidies/registry"
	"github.com/ethereum/go-ethereum/core/vm/runtime"
	"github.com/stretchr/testify/require"
)

func TestParseClientVersion_ReadsTheVersionOfReleasedClientsOnly(t *testing.T) {
	tests := map[string]struct {
		want      clientVersion
		isRelease bool
	}{
		"sonic:v2.1.6":                     {clientVersion{2, 1, 6}, true},
		"sonic:v2.2.0":                     {clientVersion{2, 2, 0}, true},
		"sonic:v2.2.1-rc.1":                {clientVersion{2, 2, 1}, true},
		"sonic:v2.1.6_go1.27.0":            {clientVersion{2, 1, 6}, true},
		"sonic:v3.1":                       {clientVersion{3, 1, 0}, true},
		"ghcr.io/0xsoniclabs/sonic:v2.1.5": {clientVersion{2, 1, 5}, true},
		"registry:5000/sonic:v2.1.5":       {clientVersion{2, 1, 5}, true},
		"sonic:local":                      {clientVersion{}, false},
		"sonic:latest":                     {clientVersion{}, false},
		"sonic:9a1a16c3":                   {clientVersion{}, false},
		"sonic":                            {clientVersion{}, false},
		"registry:5000/sonic":              {clientVersion{}, false},
	}

	for image, test := range tests {
		t.Run(image, func(t *testing.T) {
			version, isRelease := parseClientVersion(image)
			require.Equal(t, test.isRelease, isRelease)
			require.Equal(t, test.want, version)
		})
	}
}

func TestClientVersion_IsBeforeOrdersReleases(t *testing.T) {
	require := require.New(t)
	require.True(clientVersion{2, 1, 6}.isBefore(clientVersion{2, 2, 0}))
	require.True(clientVersion{1, 9, 9}.isBefore(clientVersion{2, 0, 0}))
	require.True(clientVersion{2, 2, 0}.isBefore(clientVersion{2, 2, 1}))
	require.False(clientVersion{2, 2, 0}.isBefore(clientVersion{2, 2, 0}))
	require.False(clientVersion{2, 2, 1}.isBefore(clientVersion{2, 2, 0}))
	require.False(clientVersion{3, 0, 0}.isBefore(clientVersion{2, 2, 0}))
}

// TestSubsidiesRegistryCode_SuitsTheOldestClientOfTheNetwork covers the reason
// this selection exists: a client before v2.2.0 rejects the registry the current
// Sonic sources ship, so any network containing one has to get the legacy
// registry, which the newer clients read as well.
func TestSubsidiesRegistryCode_SuitsTheOldestClientOfTheNetwork(t *testing.T) {
	legacy := serveLegacyRegistryCode(t, "6001600101")
	current := gas_subsidies_registry.GetCode()

	tests := map[string][]string{
		"no client":                    nil,
		"local sources":                {"sonic:local"},
		"extended registry release":    {"sonic:v2.2.0"},
		"mixed new releases":           {"sonic:local", "sonic:v2.2.0", "sonic:v2.2.1"},
		"legacy release":               {"sonic:v2.1.6"},
		"legacy release joining later": {"sonic:local", "sonic:v2.2.0", "sonic:v2.1.6"},
		"legacy release first":         {"sonic:v2.1.5", "sonic:local"},
	}

	for name, images := range tests {
		t.Run(name, func(t *testing.T) {
			want := current
			for _, image := range images {
				if version, isRelease := parseClientVersion(image); isRelease &&
					version.isBefore(firstExtendedRegistryVersion) {
					want = legacy
				}
			}

			code, err := subsidiesRegistryCode(images)
			require.NoError(t, err)
			require.Equal(t, want, code)
		})
	}
}

func TestSubsidiesRegistryCode_ReportsAnUnusableFetchResult(t *testing.T) {
	tests := map[string]http.HandlerFunc{
		"missing file": func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusNotFound)
		},
		"no bytecode": func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte("\n"))
		},
		"not hexadecimal": func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte("not a contract"))
		},
	}

	for name, handler := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(handler)
			t.Cleanup(server.Close)
			redirectLegacyRegistryCodeURL(t, server.URL)

			_, err := subsidiesRegistryCode([]string{"sonic:v2.1.6"})
			require.ErrorContains(t, err,
				"failed to fetch the "+legacyRegistryRelease+" subsidies registry")
		})
	}
}

// TestSubsidiesRegistryCode_CurrentRegistryStillReturnsTheExtendedGasConfig
// pins the ABI of the registry the current Sonic sources ship. A third shape
// would need another entry in this selection, so this failing is the signal to
// revisit firstExtendedRegistryVersion.
func TestSubsidiesRegistryCode_CurrentRegistryStillReturnsTheExtendedGasConfig(t *testing.T) {
	require := require.New(t)

	code, err := subsidiesRegistryCode([]string{"sonic:local"})
	require.NoError(err)
	require.Len(getGasConfigResult(t, code), 5*32)
}

// TestSubsidiesRegistryCode_LegacyRegistryReturnsTheOldGasConfig runs the
// fetched contract to confirm it is the one clients before v2.2.0 can read: they
// require a getGasConfig result of exactly three words.
func TestSubsidiesRegistryCode_LegacyRegistryReturnsTheOldGasConfig(t *testing.T) {
	require := require.New(t)

	code, err := subsidiesRegistryCode([]string{"sonic:" + legacyRegistryRelease})
	if err != nil {
		t.Skip("fetching the legacy subsidies registry needs network access:", err)
	}
	require.Len(getGasConfigResult(t, code), 3*32)
}

// getGasConfigResult calls getGasConfig on the given contract bytecode.
func getGasConfigResult(t *testing.T, code []byte) []byte {
	t.Helper()
	input := binary.BigEndian.AppendUint32(nil,
		gas_subsidies_registry.GetGasConfigFunctionSelector)
	result, _, err := runtime.Execute(code, input, nil)
	require.NoError(t, err)
	return result
}

// serveLegacyRegistryCode redirects the fetch of the legacy registry to a local
// server handing out the given bytecode, and returns the decoded bytecode.
func serveLegacyRegistryCode(t *testing.T, codeInHex string) []byte {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(codeInHex + "\n"))
		}))
	t.Cleanup(server.Close)
	redirectLegacyRegistryCodeURL(t, server.URL)

	code, err := hex.DecodeString(codeInHex)
	require.NoError(t, err)
	return code
}

// redirectLegacyRegistryCodeURL points the fetch of the legacy registry at the
// given URL for the duration of the test.
func redirectLegacyRegistryCodeURL(t *testing.T, url string) {
	t.Helper()
	original := legacyRegistryCodeURL
	legacyRegistryCodeURL = url
	t.Cleanup(func() { legacyRegistryCodeURL = original })
}
