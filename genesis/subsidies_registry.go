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
var firstExtendedRegistryVersion = clientVersion{2, 2, 0}

// subsidiesRegistryCode returns the bytecode of the subsidies registry to
// install in the genesis state of a network running the given client images.
func subsidiesRegistryCode(clientImages []string) ([]byte, error) {
	for _, image := range clientImages {
		version, isRelease := parseClientVersion(image)
		if isRelease && version.isBefore(firstExtendedRegistryVersion) {
			return fetchLegacyRegistryCode()
		}
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

// clientVersion is the release version of a client, as named by an image tag.
type clientVersion struct {
	major, minor, patch int
}

// isBefore reports whether this version was released before the other one.
func (v clientVersion) isBefore(other clientVersion) bool {
	return cmp.Or(
		cmp.Compare(v.major, other.major),
		cmp.Compare(v.minor, other.minor),
		cmp.Compare(v.patch, other.patch),
	) < 0
}

// releaseTag matches the image tag of a released client. The tag may carry a
// suffix, like a release candidate number or a Go toolchain pin.
var releaseTag = regexp.MustCompile(`^v(\d+)\.(\d+)(?:\.(\d+))?`)

// parseClientVersion extracts the release version from a client image
// reference. It reports false for references that do not name a release -
// "sonic:local", "sonic:latest", a commit hash, no tag at all - which all build
// from sources newer than every release.
func parseClientVersion(image string) (clientVersion, bool) {
	name := image[strings.LastIndex(image, "/")+1:]
	tag := ""
	if colon := strings.LastIndex(name, ":"); colon >= 0 {
		tag = name[colon+1:]
	}
	match := releaseTag.FindStringSubmatch(tag)
	if match == nil {
		return clientVersion{}, false
	}
	// The groups hold digits only; an absent patch group converts to zero.
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	return clientVersion{major, minor, patch}, true
}
