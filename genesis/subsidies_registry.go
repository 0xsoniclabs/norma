package genesis

import (
	"cmp"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	gas_subsidies_registry "github.com/0xsoniclabs/sonic/gossip/blockproc/subsidies/registry"
)

// The subsidies registry lives in the genesis state, and its ABI is part of the
// consensus rules: while processing a sponsorship request, every client calls
// getGasConfig and chooseFund on it. Both changed with v2.2.0 - getGasConfig
// grew from three to five return values, chooseFund from a bare fund id to a
// (mode, payload) pair. Clients from v2.2.0 on accept either shape, earlier ones
// require the old one exactly and treat a mismatch as "not sponsored", which
// forks them off the nodes that do sponsor the transaction. A network therefore
// gets the registry that its oldest client understands, and the newer clients
// read it through their backwards compatible path.
//
// Which shape a client reads follows from its version, and the versions are the
// ones the clients report for themselves (see docker.ClientVersions). An image
// reference is no substitute: "sonic:local" and "sonic:<commit hash>" build from
// sources of any age, so a network of them could be handed a registry none of
// its clients can read.

// legacyRegistryRelease is the release shipping the registry the clients before
// v2.2.0 understand. v2.1.5 shipped the same bytecode.
const legacyRegistryRelease = "v2.1.6"

// legacyRegistryCodeURL locates the on-chain bytecode of that release's registry
// in the Sonic repository. It is fetched when a network needs it, so that the
// genesis holds the artifact the release actually shipped instead of a copy of
// it aging inside Norma. Tests redirect this to a local server.
var legacyRegistryCodeURL = "https://raw.githubusercontent.com/0xsoniclabs/sonic/" +
	legacyRegistryRelease +
	"/gossip/blockproc/subsidies/registry/subsidies_contract.bin"

// legacyRegistryFetchTimeout bounds the fetch of the legacy registry bytecode.
const legacyRegistryFetchTimeout = 30 * time.Second

// firstExtendedRegistryVersion is the first client release reading both the
// legacy and the extended registry ABI.
var firstExtendedRegistryVersion = clientVersion{major: 2, minor: 2}

// subsidiesRegistryCode returns the bytecode of the subsidies registry to
// install in the genesis state of a network whose clients report the given
// versions.
func subsidiesRegistryCode(clientVersions []string) ([]byte, error) {
	legacy := false
	for _, reported := range clientVersions {
		version, err := parseClientVersion(reported)
		if err != nil {
			return nil, err
		}
		legacy = legacy || version.isBefore(firstExtendedRegistryVersion)
	}
	if legacy {
		return fetchLegacyRegistryCode()
	}
	return gas_subsidies_registry.GetCode(), nil
}

// fetchLegacyRegistryCode downloads the on-chain bytecode of the subsidies
// registry as shipped by legacyRegistryRelease.
func fetchLegacyRegistryCode() ([]byte, error) {
	fail := func(err error) error {
		return fmt.Errorf("failed to fetch the %s subsidies registry from %s: %w",
			legacyRegistryRelease, legacyRegistryCodeURL, err)
	}

	client := http.Client{Timeout: legacyRegistryFetchTimeout}
	response, err := client.Get(legacyRegistryCodeURL)
	if err != nil {
		return nil, fail(err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		return nil, fail(fmt.Errorf("unexpected status %s", response.Status))
	}

	encoded, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fail(err)
	}
	code, err := hex.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		return nil, fail(err)
	}
	if len(code) == 0 {
		return nil, fail(fmt.Errorf("no bytecode served"))
	}
	return code, nil
}

// clientVersion is the version a client reports for itself.
type clientVersion struct {
	major, minor, patch int
	// preRelease marks a release candidate or development build. It runs
	// under the name of a release that is not out yet, so it precedes it.
	preRelease bool
}

// isBefore reports whether this version precedes the other one.
func (v clientVersion) isBefore(other clientVersion) bool {
	if order := cmp.Or(
		cmp.Compare(v.major, other.major),
		cmp.Compare(v.minor, other.minor),
		cmp.Compare(v.patch, other.patch),
	); order != 0 {
		return order < 0
	}
	return v.preRelease && !other.preRelease
}

// reportedVersion matches a version as the Sonic version package formats it:
// three numbers, followed by "-rc.<n>" or "-dev" for a pre-release.
var reportedVersion = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(-\S+)?$`)

// parseClientVersion reads the version a client reported for itself. A version
// it cannot read is an error rather than an assumption: which registry a client
// can read follows from its version, and a wrong guess forks the network.
func parseClientVersion(reported string) (clientVersion, error) {
	match := reportedVersion.FindStringSubmatch(reported)
	if match == nil {
		return clientVersion{}, fmt.Errorf(
			"unable to read the client version %q", reported)
	}
	// The groups hold digits only.
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	return clientVersion{major, minor, patch, match[4] != ""}, nil
}
