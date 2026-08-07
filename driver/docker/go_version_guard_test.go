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
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// TestDockerfileGoVersion_SatisfiesSonicGoDirective guards the Dockerfile's
// default toolchain against the sonic go.mod directive. The build stage sets
// GOTOOLCHAIN=local, so a directive above the default GO_VERSION no longer
// silently downloads a newer compiler — it fails the docker build. This test
// turns that late, confusing failure into an immediate one naming the fix:
// raise the ARG GO_VERSION default in the Dockerfile.
func TestDockerfileGoVersion_SatisfiesSonicGoDirective(t *testing.T) {
	root, err := resolveBuildRoot(".")
	if err != nil {
		t.Fatalf("failed to resolve build root: %v", err)
	}

	dockerfile, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatalf("failed to read Dockerfile: %v", err)
	}
	match := regexp.MustCompile(`(?m)^ARG GO_VERSION=(\S+)$`).FindSubmatch(dockerfile)
	if match == nil {
		t.Fatalf("Dockerfile declares no ARG GO_VERSION default")
	}
	defaultVersion := string(match[1])

	gomodPath := filepath.Join(root, DefaultSonicLocalPath, "go.mod")
	gomod, err := os.ReadFile(gomodPath) //#nosec G304 -- path is derived from the build root
	if err != nil {
		t.Skipf("sonic submodule not available: %v", err)
	}
	match = regexp.MustCompile(`(?m)^go (\S+)$`).FindSubmatch(gomod)
	if match == nil {
		t.Fatalf("no go directive found in %s", gomodPath)
	}
	directive := string(match[1])

	if compareGoVersions(t, defaultVersion, directive) < 0 {
		t.Errorf("the Dockerfile builds with Go %s, but sonic/go.mod requires "+
			"go >= %s; with GOTOOLCHAIN=local the docker build will fail — "+
			"raise the ARG GO_VERSION default in the Dockerfile",
			defaultVersion, directive)
	}
}

// compareGoVersions orders two Go version strings like the Go toolchain
// does: for one minor version, "1.27" < "1.27rc2" < "1.27.0" < "1.27.1".
func compareGoVersions(t *testing.T, a, b string) int {
	t.Helper()
	va, vb := parseGoVersion(t, a), parseGoVersion(t, b)
	for i := range va {
		if va[i] != vb[i] {
			if va[i] < vb[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// parseGoVersion decomposes a version like "1.26.3", "1.27" or "1.27rc2"
// into a comparable tuple of major, minor, release kind (bare directive,
// pre-release, release) and pre-release or patch number.
func parseGoVersion(t *testing.T, v string) [4]int {
	t.Helper()
	match := regexp.MustCompile(`^(\d+)\.(\d+)(?:(?:rc|beta)(\d+)|\.(\d+))?$`).
		FindStringSubmatch(v)
	if match == nil {
		t.Fatalf("unparseable Go version %q", v)
	}
	num := func(s string) int {
		n, err := strconv.Atoi(s)
		if err != nil {
			t.Fatalf("unparseable Go version component %q in %q", s, v)
		}
		return n
	}
	parsed := [4]int{num(match[1]), num(match[2])}
	switch {
	case match[3] != "": // pre-release: above the bare directive, below any release
		parsed[2], parsed[3] = 1, num(match[3])
	case match[4] != "": // release with patch level
		parsed[2], parsed[3] = 2, num(match[4])
	}
	return parsed
}

func TestCompareGoVersions_OrdersLikeTheToolchain(t *testing.T) {
	ascending := []string{"1.26", "1.26rc1", "1.26rc2", "1.26.0", "1.26.3", "1.27beta1", "1.27.0", "2.0.0"}
	for i := range len(ascending) - 1 {
		if compareGoVersions(t, ascending[i], ascending[i+1]) >= 0 {
			t.Errorf("expected %q < %q", ascending[i], ascending[i+1])
		}
	}
	if compareGoVersions(t, "1.26.3", "1.26.3") != 0 {
		t.Errorf("expected %q == %q", "1.26.3", "1.26.3")
	}
}
