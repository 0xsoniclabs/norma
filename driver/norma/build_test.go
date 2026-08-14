package main

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/0xsoniclabs/norma/driver/docker"
	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/stretchr/testify/require"
)

func TestCollectScenarioFiles_Recursive(t *testing.T) {
	tmp := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmp, "a.yml"), []byte("name: a\nduration: 1\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "nested"), 0755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "nested", "b.yaml"), []byte("name: b\nduration: 1\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "nested", "ignore.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	got, err := collectScenarioFiles(tmp)
	if err != nil {
		t.Fatalf("collectScenarioFiles() failed: %v", err)
	}

	want := []string{
		filepath.Join(tmp, "a.yml"),
		filepath.Join(tmp, "nested", "b.yaml"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("invalid files\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestCollectScenarioFiles_EmptyDir(t *testing.T) {
	tmp := t.TempDir()

	got, err := collectScenarioFiles(tmp)
	if err != nil {
		t.Fatalf("collectScenarioFiles() failed: %v", err)
	}

	want := []string{}
	if !slices.Equal(got, want) {
		t.Fatalf("invalid files\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestCollectBuildableImages(t *testing.T) {
	tmp := t.TempDir()

	scenarioA := `
Name: scenario-a
Description: two nodes.
Scenario:
  - startNode: validator-a
    type: validator
    imageName: sonic:v2.1.2

  - startNode: node-a
    type: rpc
    imageName: sonic:local
`
	scenarioB := `
Name: scenario-b
Description: single node.
Scenario:
  - startNode: node-b
    type: validator
`

	pathA := filepath.Join(tmp, "a.yml")
	pathB := filepath.Join(tmp, "b.yml")
	if err := os.WriteFile(pathA, []byte(scenarioA), 0644); err != nil {
		t.Fatalf("failed to write scenario A: %v", err)
	}
	if err := os.WriteFile(pathB, []byte(scenarioB), 0644); err != nil {
		t.Fatalf("failed to write scenario B: %v", err)
	}

	got, err := collectBuildableImages([]string{pathA, pathB})
	if err != nil {
		t.Fatalf("collectBuildableImages() failed: %v", err)
	}

	// node-b omits imageName and so resolves to the default, sonic:local.
	want := []string{"sonic:local", "sonic:v2.1.2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("invalid images\ngot:  %#v\nwant: %#v", got, want)
	}
}

// mixedVersionScenario starts six nodes on five different client images: three
// pinned releases, the candidate the submodule builds, the remote main branch,
// and a node joining later that repeats one of the pinned releases. The genesis
// of such a run has to suit all of them at once, so what the build step leaves
// behind is what the version dependent parts of the genesis are read from.
const mixedVersionScenario = `
Name: mixed client versions
Description: six nodes on five client images.
Scenario:
  - startNode: validator-v215
    type: validator
    imageName: sonic:v2.1.5

  - startNode: validator-v216
    type: validator
    imageName: sonic:v2.1.6

  - startNode: validator-v220
    type: validator
    imageName: sonic:v2.2.0

  - startNode: validator-local
    type: validator

  - startNode: validator-main
    type: validator
    imageName: sonic:main

  - waitFor: 30s

  - startNode: rpc-v216
    type: rpc
    imageName: sonic:v2.1.6
`

// pinnedVersions is the version each pinned image of that scenario has to
// report. A released tag is the only image reference whose client version is
// known without running anything: "sonic:local" builds from the submodule and
// "sonic:main" from the remote default branch, so either of them can be of any
// age - including one that coincides with a release, or with the other.
var pinnedVersions = map[string]string{
	"sonic:v2.1.5": "2.1.5",
	"sonic:v2.1.6": "2.1.6",
	"sonic:v2.2.0": "2.2.0",
}

// TestBuild_ProvidesEveryClientVersionOfAScenario runs the build step of a
// scenario whose nodes disagree on their client version, and checks that all of
// those versions are there afterwards: every image is present with the client
// its reference asked for, and reading the versions of the scenario reports
// their union - the input the genesis is generated from.
func TestBuild_ProvidesEveryClientVersionOfAScenario(t *testing.T) {
	skipWithoutDocker(t)

	path := filepath.Join(t.TempDir(), "mixed_client_versions.yml")
	require.NoError(t, os.WriteFile(path, []byte(mixedVersionScenario), 0644))

	buildRoot, err := docker.ResolveBuildRoot(".")
	require.NoError(t, err)

	images, err := ensureClientImages(t.Context(), []string{path}, buildRoot, false)
	require.NoError(t, err)
	require.Equal(t, []string{
		"sonic:local", "sonic:main", "sonic:v2.1.5", "sonic:v2.1.6", "sonic:v2.2.0",
	}, images)

	// Every image now holds the client its reference asked for. Asking the
	// images directly keeps this independent of the build step: an image that
	// was never built cannot answer, since these tags exist nowhere but here.
	built := map[string]string{}
	for _, image := range images {
		built[image] = reportedClientVersion(t, image)
	}
	for image, want := range pinnedVersions {
		require.Equal(t, want, built[image], "client version of %s", image)
	}

	// Reading the versions of the scenario reports the client of every one of its
	// images: an image started by several nodes is asked once, whenever those
	// nodes join, while two references that turn out to hold the same client each
	// contribute an entry. The genesis only looks for the oldest among them, so
	// what has to hold is that none is missing.
	scenario, err := parser.ParseFile(path)
	require.NoError(t, err)
	versions, err := docker.ClientVersions(t.Context(), collectClientImages(&scenario))
	require.NoError(t, err)
	require.ElementsMatch(t, slices.Collect(maps.Values(built)), versions)
}

// reportedClientVersion runs the client of a locally available image to have it
// report its own version, without going through the code under test.
func reportedClientVersion(t *testing.T, imageRef string) string {
	t.Helper()
	output, err := exec.CommandContext(t.Context(), "docker", "run", "--rm",
		"--entrypoint", docker.SonicdBinaryPath, imageRef, "version").CombinedOutput()
	require.NoError(t, err, "failed to ask %s for its client version:\n%s",
		imageRef, output)

	match := regexp.MustCompile(`(?m)^Version:[ \t]*(\S+)`).FindStringSubmatch(string(output))
	require.NotNil(t, match, "no version reported by %s:\n%s",
		imageRef, strings.TrimSpace(string(output)))
	return match[1]
}

func skipWithoutDocker(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skipf("docker socket not available: %v", err)
	}
}

func TestWillBuildImage(t *testing.T) {
	tests := []struct {
		name      string
		image     string
		wantBuild bool
	}{
		{
			name:      "default sonic",
			image:     "sonic",
			wantBuild: true,
		},
		{
			name:      "local",
			image:     "sonic:local",
			wantBuild: true,
		},
		{
			name:      "version tag",
			image:     "sonic:v2.1.1",
			wantBuild: true,
		},
		{
			name:      "non sonic",
			image:     "alpine:latest",
			wantBuild: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := docker.WillBuildImage(tt.image); got != tt.wantBuild {
				t.Fatalf("invalid build classification\ngot:  %v\nwant: %v", got, tt.wantBuild)
			}
		})
	}
}
