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
	"testing"
)

func TestParseReportedVersion_ReadsTheVersionLine(t *testing.T) {
	tests := map[string]struct {
		output string
		want   string
	}{
		"release": {
			output: "sonic\nVersion: 2.1.6\nGit Commit: 9a1a16c3\nArchitecture: amd64\n",
			want:   "2.1.6",
		},
		"development build": {
			output: "sonic\nVersion: 2.3.0-dev\nGo Version: go1.26.3\n",
			want:   "2.3.0-dev",
		},
		"release candidate": {
			output: "sonic\nVersion: 2.2.1-rc.1\n",
			want:   "2.2.1-rc.1",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, found := parseReportedVersion([]byte(test.output))
			if !found {
				t.Fatalf("no version found in %q", test.output)
			}
			if got != test.want {
				t.Fatalf("invalid version, got %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseReportedVersion_ReportsOutputWithoutAVersion(t *testing.T) {
	tests := map[string]string{
		"empty":                 "",
		"other keys only":       "sonic\nGo Version: go1.26.3\n",
		"not at the line start": "sonic\n  Version: 2.1.6\n",
		"no value":              "sonic\nVersion:\n",
	}

	for name, output := range tests {
		t.Run(name, func(t *testing.T) {
			if version, found := parseReportedVersion([]byte(output)); found {
				t.Fatalf("unexpected version %q in %q", version, output)
			}
		})
	}
}

// TestClientVersions_AsksTheImagesForTheirVersion covers the reason the images
// are asked at all: their references do not carry a version.
func TestClientVersions_AsksTheImagesForTheirVersion(t *testing.T) {
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skipf("docker socket not available: %v", err)
	}

	buildRoot, err := ResolveBuildRoot(".")
	if err != nil {
		t.Fatalf("failed to resolve build root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(buildRoot, "sonic")); err != nil {
		t.Skipf("local sonic sources not available at %s: %v",
			filepath.Join(buildRoot, "sonic"), err)
	}

	// The same image twice: it is asked once, as one client version.
	versions, err := ClientVersions(t.Context(), []string{"sonic:local", "sonic:local"})
	if err != nil {
		t.Fatalf("failed to read the client version of sonic:local: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("invalid versions, got %v, want one entry", versions)
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(versions[0]) {
		t.Fatalf("unexpected client version %q", versions[0])
	}
}
