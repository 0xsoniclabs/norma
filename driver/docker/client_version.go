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

package docker

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"
)

// SonicdBinaryPath is the client binary inside a client image. Stage 2 of the
// Dockerfile copies the binaries into the root directory, and no WORKDIR is
// set, so the path is absolute and independent of any working directory.
const SonicdBinaryPath = "/sonicd"

// ClientVersions returns the version every one of the given images reports for
// its client, deduplicated along with the images.
//
// The images are asked rather than their references read, because a reference
// does not carry a version: "sonic:local" builds from a working copy and
// "sonic:<commit hash>" from an arbitrary commit, either of which may be older
// or newer than any release. Images that are not present yet are built or
// pulled first, so this can be called before any node has started.
func ClientVersions(ctx context.Context, imageRefs []string) ([]string, error) {
	refs := NormalizeImageRefs(imageRefs)
	if err := EnsureImages(ctx, refs, ""); err != nil {
		return nil, err
	}

	versions := make([]string, 0, len(refs))
	for _, ref := range refs {
		version, err := clientVersion(ctx, ref)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	slog.Info("resolved client versions", "refs", refs, "versions", versions)
	return versions, nil
}

// clientVersion runs the client of the given locally available image to have it
// report its own version.
func clientVersion(ctx context.Context, imageRef string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"--entrypoint", SonicdBinaryPath, imageRef, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to ask image %q for its client version: %w\n%s",
			imageRef, err, strings.TrimSpace(string(output)))
	}
	version, found := parseReportedVersion(output)
	if !found {
		return "", fmt.Errorf("image %q reported no client version:\n%s",
			imageRef, strings.TrimSpace(string(output)))
	}
	return version, nil
}

// versionLine matches the version in the output of `sonicd version`, which is
// documented to be machine readable and has carried this line since v2.0.0.
var versionLine = regexp.MustCompile(`(?m)^Version:[ \t]*(\S+)`)

// parseReportedVersion extracts the client version from that output.
func parseReportedVersion(output []byte) (string, bool) {
	match := versionLine.FindSubmatch(output)
	if match == nil {
		return "", false
	}
	return string(match[1]), true
}
