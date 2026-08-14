package genesis

import (
	"encoding/binary"
	"testing"

	gas_subsidies_registry "github.com/0xsoniclabs/sonic/gossip/blockproc/subsidies/registry"
	"github.com/ethereum/go-ethereum/core/vm/runtime"
	"github.com/stretchr/testify/require"
)

func TestParseClientVersion_ReadsTheVersionsClientsReport(t *testing.T) {
	tests := map[string]clientVersion{
		"2.1.6":      {2, 1, 6, false},
		"2.2.0":      {2, 2, 0, false},
		"2.2.1-rc.1": {2, 2, 1, true},
		"2.3.0-dev":  {2, 3, 0, true},
		"10.0.11":    {10, 0, 11, false},
	}

	for reported, want := range tests {
		t.Run(reported, func(t *testing.T) {
			version, err := parseClientVersion(reported)
			require.NoError(t, err)
			require.Equal(t, want, version)
		})
	}
}

// TestParseClientVersion_RejectsAnUnreadableVersion covers the reason parsing
// reports an error at all: a version that cannot be read must not be taken for
// a recent one, because the registry that assumption installs forks every
// client that turns out to be older.
func TestParseClientVersion_RejectsAnUnreadableVersion(t *testing.T) {
	tests := []string{
		"",
		"v2.1.6",
		"2.1",
		"2.1.x",
		"sonic:local",
		"9a1a16c3",
	}

	for _, reported := range tests {
		t.Run(reported, func(t *testing.T) {
			_, err := parseClientVersion(reported)
			require.ErrorContains(t, err, "unable to read the client version")
		})
	}
}

func TestClientVersion_IsBeforeOrdersVersions(t *testing.T) {
	require := require.New(t)
	require.True(clientVersion{2, 1, 6, false}.isBefore(clientVersion{2, 2, 0, false}))
	require.True(clientVersion{1, 9, 9, false}.isBefore(clientVersion{2, 0, 0, false}))
	require.True(clientVersion{2, 2, 0, false}.isBefore(clientVersion{2, 2, 1, false}))
	require.False(clientVersion{2, 2, 0, false}.isBefore(clientVersion{2, 2, 0, false}))
	require.False(clientVersion{2, 2, 1, false}.isBefore(clientVersion{2, 2, 0, false}))
	require.False(clientVersion{3, 0, 0, false}.isBefore(clientVersion{2, 2, 0, false}))

	// A pre-release runs ahead of the release it is named after, but its code
	// is behind it: the change the release brings may not be in yet.
	require.True(clientVersion{2, 2, 0, true}.isBefore(clientVersion{2, 2, 0, false}))
	require.False(clientVersion{2, 2, 0, false}.isBefore(clientVersion{2, 2, 0, true}))
	require.False(clientVersion{2, 2, 0, true}.isBefore(clientVersion{2, 2, 0, true}))
	require.True(clientVersion{2, 2, 0, false}.isBefore(clientVersion{2, 2, 1, true}))
	require.False(clientVersion{2, 3, 0, true}.isBefore(clientVersion{2, 2, 0, false}))
}

// TestSubsidiesRegistryCode_SuitsTheOldestClientOfTheNetwork covers the reason
// this selection exists: a client before v2.2.0 rejects the registry the current
// Sonic sources ship, so any network containing one has to get the legacy
// registry, which the newer clients read as well.
func TestSubsidiesRegistryCode_SuitsTheOldestClientOfTheNetwork(t *testing.T) {
	legacy, err := legacyRegistryCode()
	require.NoError(t, err)
	current := gas_subsidies_registry.GetCode()
	require.NotEqual(t, current, legacy)

	tests := map[string]struct {
		clientVersions []string
		want           []byte
	}{
		"no client":                    {nil, current},
		"development build":            {[]string{"2.3.0-dev"}, current},
		"extended registry release":    {[]string{"2.2.0"}, current},
		"mixed new versions":           {[]string{"2.3.0-dev", "2.2.0", "2.2.1"}, current},
		"legacy release":               {[]string{"2.1.6"}, legacy},
		"legacy release joining later": {[]string{"2.3.0-dev", "2.2.0", "2.1.6"}, legacy},
		"legacy release first":         {[]string{"2.1.5", "2.3.0-dev"}, legacy},
		// The extended registry landed during the development of v2.2.0, so a
		// build named after that release does not have to be able to read it.
		"development build of the extended registry release": {
			[]string{"2.2.0-dev", "2.3.0-dev"}, legacy,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			code, err := subsidiesRegistryCode(test.clientVersions)
			require.NoError(t, err)
			require.Equal(t, test.want, code)
		})
	}
}

// TestSubsidiesRegistryCode_RejectsAnUnreadableClientVersion checks that no
// registry is chosen for a client whose version could not be read - whichever
// place in the network that client has. Deciding before every version is read
// would make the outcome depend on the order the clients come in.
func TestSubsidiesRegistryCode_RejectsAnUnreadableClientVersion(t *testing.T) {
	tests := map[string][]string{
		"only client":            {"sonic:local"},
		"after a current client": {"2.2.0", "sonic:local"},
		"after a legacy client":  {"2.1.6", "sonic:local"},
		"before a legacy client": {"sonic:local", "2.1.6"},
	}

	for name, clientVersions := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := subsidiesRegistryCode(clientVersions)
			require.ErrorContains(t, err, "unable to read the client version")
		})
	}
}

// TestSubsidiesRegistryCode_CurrentRegistryStillReturnsTheExtendedGasConfig
// pins the ABI of the registry the current Sonic sources ship. A third shape
// would need another entry in this selection, so this failing is the signal to
// revisit firstExtendedRegistryVersion.
func TestSubsidiesRegistryCode_CurrentRegistryStillReturnsTheExtendedGasConfig(t *testing.T) {
	require := require.New(t)

	code, err := subsidiesRegistryCode([]string{"2.3.0-dev"})
	require.NoError(err)
	require.Len(getGasConfigResult(t, code), 5*32)
}

// TestSubsidiesRegistryCode_LegacyRegistryReturnsTheOldGasConfig runs the
// checked-in contract to confirm it is the one clients before v2.2.0 can read:
// they require a getGasConfig result of exactly three words.
func TestSubsidiesRegistryCode_LegacyRegistryReturnsTheOldGasConfig(t *testing.T) {
	require := require.New(t)

	code, err := subsidiesRegistryCode([]string{"2.1.6"})
	require.NoError(err)
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
