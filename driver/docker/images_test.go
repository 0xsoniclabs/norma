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
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

func TestPlanImage(t *testing.T) {
	tests := []struct {
		name     string
		imageRef string
		want     imageBuildPlan
	}{
		{
			name:     "remote main image",
			imageRef: "sonic",
			want: imageBuildPlan{
				source: sonicRepositoryURL,
				gitRef: gitHeadRef,
			},
		},
		{
			name:     "latest is the default branch, not a git ref",
			imageRef: "sonic:latest",
			want: imageBuildPlan{
				source: sonicRepositoryURL,
				gitRef: gitHeadRef,
			},
		},
		{
			name:     "local image",
			imageRef: "sonic:local",
			want: imageBuildPlan{
				source: "sonic",
			},
		},
		{
			name:     "tagged remote image",
			imageRef: "sonic:v2.1.2",
			want: imageBuildPlan{
				source: sonicRepositoryURL,
				gitRef: "v2.1.2",
			},
		},
		{
			name:     "non sonic image is pull-only plan",
			imageRef: "alpine",
			want:     imageBuildPlan{},
		},
		{
			name:     "empty sonic tag falls back to pull-only",
			imageRef: "sonic:",
			want:     imageBuildPlan{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := planImage(tt.imageRef)
			if got != tt.want {
				t.Fatalf("invalid plan, got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSetSonicLocalPath_OverridesLocalBuildContext(t *testing.T) {
	original := SonicLocalPath()
	t.Cleanup(func() { SetSonicLocalPath(original) })

	if got := SonicLocalPath(); got != DefaultSonicLocalPath {
		t.Fatalf("unexpected default sonic local path: got %q, want %q",
			got, DefaultSonicLocalPath)
	}

	custom := "/tmp/custom-sonic"
	SetSonicLocalPath(custom)
	if got := SonicLocalPath(); got != custom {
		t.Fatalf("SonicLocalPath after set: got %q, want %q", got, custom)
	}

	plan := planImage("sonic:local")
	want := imageBuildPlan{source: custom}
	if plan != want {
		t.Fatalf("planImage(sonic:local) after override: got %+v, want %+v",
			plan, want)
	}

	SetSonicLocalPath("")
	if got := SonicLocalPath(); got != DefaultSonicLocalPath {
		t.Fatalf("SonicLocalPath after reset: got %q, want %q",
			got, DefaultSonicLocalPath)
	}
}

func TestNormalizeImageRefs_DeduplicatesAndSorts(t *testing.T) {
	input := []string{"sonic:v2.1", "", "sonic", "sonic:v2.1", "   ", "alpine"}
	want := []string{"alpine", "sonic", "sonic:v2.1"}

	got := NormalizeImageRefs(input)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("invalid refs, got %v, want %v", got, want)
	}
}

// writeNormaRootMarkers creates the files that identify dir as the Norma
// build root.
func writeNormaRootMarkers(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"),
		[]byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatalf("failed to write Dockerfile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module github.com/0xsoniclabs/norma\n\ngo 1.26.0\n"), 0o644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}
}

func TestResolveBuildRoot(t *testing.T) {
	t.Run("finds root by walking parents", func(t *testing.T) {
		root := t.TempDir()
		writeNormaRootMarkers(t, root)

		deep := filepath.Join(root, "a", "b", "c")
		if err := os.MkdirAll(deep, 0o755); err != nil {
			t.Fatalf("failed to create deep directory: %v", err)
		}

		got, err := ResolveBuildRoot(deep)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != root {
			t.Fatalf("invalid root, got %q, want %q", got, root)
		}
	})

	t.Run("fails when root markers are missing", func(t *testing.T) {
		start := t.TempDir()
		_, err := ResolveBuildRoot(start)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
	})

	// The vendored sonic sub-tree carries both a Dockerfile and a Makefile,
	// so generic markers would resolve to it and build the wrong project.
	t.Run("skips nested module with its own Dockerfile", func(t *testing.T) {
		root := t.TempDir()
		writeNormaRootMarkers(t, root)

		nested := filepath.Join(root, "sonic")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatalf("failed to create nested directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(nested, "Dockerfile"),
			[]byte("FROM scratch\n"), 0o644); err != nil {
			t.Fatalf("failed to write nested Dockerfile: %v", err)
		}
		if err := os.WriteFile(filepath.Join(nested, "Makefile"),
			[]byte("all:\n"), 0o644); err != nil {
			t.Fatalf("failed to write nested Makefile: %v", err)
		}
		if err := os.WriteFile(filepath.Join(nested, "go.mod"),
			[]byte("module github.com/0xsoniclabs/sonic\n"), 0o644); err != nil {
			t.Fatalf("failed to write nested go.mod: %v", err)
		}

		got, err := ResolveBuildRoot(nested)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != root {
			t.Fatalf("resolved to nested module, got %q, want %q", got, root)
		}
	})
}

func TestFileExists(t *testing.T) {
	t.Run("true for regular file", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "f.txt")
		if err := os.WriteFile(file, []byte("ok"), 0o644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
		if !fileExists(file) {
			t.Fatalf("expected true for file")
		}
	})

	t.Run("false for directory", func(t *testing.T) {
		dir := t.TempDir()
		if fileExists(dir) {
			t.Fatalf("expected false for directory")
		}
	})

	t.Run("false for missing path", func(t *testing.T) {
		if fileExists(filepath.Join(t.TempDir(), "does-not-exist")) {
			t.Fatalf("expected false for missing path")
		}
	})
}

// forgetResolvedImage drops what a test resolved from the images this run has
// resolved, so that it does not leak into another test.
func forgetResolvedImage(t *testing.T, imageRef string) {
	t.Cleanup(func() {
		resolvedImages.forget(imageRef)
		resolvedVersions.forget(imageRef)
	})
}

// TestOnceCache_ResolvesAKeyOnce covers why images are resolved at all: a
// reference like "sonic:local" or "sonic:main" names another image as soon as
// its sources move, and a run has to keep testing the one it was set up for.
func TestOnceCache_ResolvesAKeyOnce(t *testing.T) {
	const imageRef = "test:resolved-once"
	var cache onceCache

	var mutex sync.Mutex
	resolutions := 0
	resolve := func() (string, error) {
		mutex.Lock()
		defer mutex.Unlock()
		resolutions++
		return fmt.Sprintf("sha256:image-%d", resolutions), nil
	}

	var callers sync.WaitGroup
	for range 8 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			id, err := cache.get(t.Context(), imageRef, resolve)
			if err != nil {
				t.Errorf("failed to resolve %s: %v", imageRef, err)
				return
			}
			if id != "sha256:image-1" {
				t.Errorf("invalid image, got %q, want %q", id, "sha256:image-1")
			}
		}()
	}
	callers.Wait()

	if resolutions != 1 {
		t.Fatalf("resolved %d times, want once", resolutions)
	}
}

func TestOnceCache_ForgetsAFailedResolution(t *testing.T) {
	const imageRef = "test:resolution-failed"
	var cache onceCache

	resolutions := 0
	resolve := func() (string, error) {
		resolutions++
		if resolutions == 1 {
			return "", fmt.Errorf("injected failure")
		}
		return "sha256:image", nil
	}

	if _, err := cache.get(t.Context(), imageRef, resolve); err == nil {
		t.Fatalf("expected the injected failure, got none")
	}
	id, err := cache.get(t.Context(), imageRef, resolve)
	if err != nil {
		t.Fatalf("failed to resolve %s again: %v", imageRef, err)
	}
	if id != "sha256:image" {
		t.Fatalf("invalid image, got %q, want %q", id, "sha256:image")
	}
}

func TestParseRemoteRefs_ReadsTheCommitOfEveryRef(t *testing.T) {
	// An annotated tag is listed as the tag object and, marked with "^{}", as
	// the commit it points to - in either order.
	const tag = "1111111111111111111111111111111111111111"
	const tagged = "2222222222222222222222222222222222222222"
	const head = "3333333333333333333333333333333333333333"
	tagFirst := tag + "\trefs/tags/v1.0\n" +
		tagged + "\trefs/tags/v1.0^{}\n" +
		head + "\trefs/heads/main\n"
	commitFirst := tagged + "\trefs/tags/v1.0^{}\n" +
		tag + "\trefs/tags/v1.0\n" +
		head + "\trefs/heads/main\n"

	want := map[string]string{
		"refs/tags/v1.0":  tagged,
		"refs/heads/main": head,
	}
	for name, output := range map[string]string{
		"tag object first": tagFirst,
		"commit first":     commitFirst,
	} {
		t.Run(name, func(t *testing.T) {
			got := parseRemoteRefs([]byte(output))
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("invalid refs, got %v, want %v", got, want)
			}
		})
	}
}

func TestParseRemoteRefs_IgnoresOutputWithoutRefs(t *testing.T) {
	for name, output := range map[string]string{
		"empty":             "",
		"warning on stderr": "warning: redirecting to https://example.com/sonic.git/\n",
	} {
		t.Run(name, func(t *testing.T) {
			if got := parseRemoteRefs([]byte(output)); len(got) != 0 {
				t.Fatalf("unexpected refs %v in %q", got, output)
			}
		})
	}
}

// runGit runs a git command in the given repository and returns its output.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=norma", "GIT_AUTHOR_EMAIL=norma@example.com",
		"GIT_COMMITTER_NAME=norma", "GIT_COMMITTER_EMAIL=norma@example.com")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

// commitFile commits the given content and returns the commit it created.
func commitFile(t *testing.T, repository, content string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repository, "client.txt"),
		[]byte(content), 0o644); err != nil {
		t.Fatalf("failed to write client file: %v", err)
	}
	runGit(t, repository, "add", "client.txt")
	runGit(t, repository, "commit", "-m", content)
	return runGit(t, repository, "rev-parse", "HEAD")
}

// TestResolveCommit_ReadsWhatARefPointsToNow covers the reason a ref is
// resolved before an image is built: a branch names another commit as soon as
// it moves, so the ref alone does not say what an image contains.
func TestResolveCommit_ReadsWhatARefPointsToNow(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}

	repository := t.TempDir()
	runGit(t, repository, "init", "-q", "-b", "main")
	first := commitFile(t, repository, "first")
	runGit(t, repository, "tag", "-a", "v1.0", "-m", "release v1.0")
	runGit(t, repository, "tag", "v1.0-light")

	// An annotated tag resolves to the commit it points to, not to the tag
	// object, which is what a docker build context has to name.
	for name, ref := range map[string]string{
		"branch":          "main",
		"annotated tag":   "v1.0",
		"lightweight tag": "v1.0-light",
		"default branch":  gitHeadRef,
	} {
		t.Run(name, func(t *testing.T) {
			commit, err := resolveCommit(t.Context(), repository, ref)
			if err != nil {
				t.Fatalf("failed to resolve %q: %v", ref, err)
			}
			if commit != first {
				t.Fatalf("invalid commit for %q, got %q, want %q", ref, commit, first)
			}
		})
	}

	second := commitFile(t, repository, "second")
	commit, err := resolveCommit(t.Context(), repository, "main")
	if err != nil {
		t.Fatalf("failed to resolve the moved branch: %v", err)
	}
	if commit != second {
		t.Fatalf("invalid commit for the moved branch, got %q, want %q", commit, second)
	}
}

func TestResolveCommit_TakesAnUnknownRefForACommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}

	repository := t.TempDir()
	runGit(t, repository, "init", "-q", "-b", "main")
	commitFile(t, repository, "first")

	// A commit hash is a ref no repository lists, and the one image references
	// name directly.
	const hash = "0123456789abcdef0123456789abcdef01234567"
	commit, err := resolveCommit(t.Context(), repository, hash)
	if err != nil {
		t.Fatalf("failed to resolve a commit hash: %v", err)
	}
	if commit != hash {
		t.Fatalf("invalid commit, got %q, want %q", commit, hash)
	}

	if _, err := resolveCommit(t.Context(), repository, "no-such-branch"); err == nil {
		t.Fatalf("expected an error for an unknown ref, got none")
	}
}

func TestEnsureImages_EmptyRefs_NoOp(t *testing.T) {
	if err := EnsureImages(t.Context(), nil, ""); err != nil {
		t.Fatalf("EnsureImages should no-op for empty refs: %v", err)
	}

	if err := EnsureImages(t.Context(), []string{}, ""); err != nil {
		t.Fatalf("EnsureImages should no-op for empty refs slice: %v", err)
	}
}

func TestPullImage(t *testing.T) {
	// This test assumes docker is available and configured correctly, but does
	// not require any specific images to be present locally or remotely. Pulling
	// "hello-world" is a simple smoke test that should succeed in a working
	// environment and fail in a broken one.
	ctx := t.Context()
	cli, err := NewClient()
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}
	defer func() {
		_ = cli.Close()
	}()

	if err := pullImage(ctx, cli, "hello-world:latest"); err != nil {
		t.Fatalf("failed to pull image: %v", err)
	}
}

func TestBuildImage_Builds_SonicLocal(t *testing.T) {
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skipf("docker socket not available: %v", err)
	}

	buildRoot, err := ResolveBuildRoot(".")
	if err != nil {
		t.Fatalf("failed to resolve build root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(buildRoot, "sonic")); err != nil {
		t.Skipf("local sonic sources not available at %s: %v", filepath.Join(buildRoot, "sonic"), err)
	}

	id, err := buildImage(t.Context(), buildRoot, "sonic:testlocal", "sonic")
	if err != nil {
		t.Fatalf("failed to build sonic:testlocal image: %v", err)
	}

	cli, err := NewClient()
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}

	// The build reports the image it produced, and that image is what the run
	// keeps using once the tag it was built under names something else.
	if _, _, err := cli.cli.ImageInspectWithRaw(t.Context(), id); err != nil {
		t.Fatalf("image %s not found after build: %v", id, err)
	}
}

func TestEnsureImages_BuildsSonicLocal(t *testing.T) {
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skipf("docker socket not available: %v", err)
	}

	buildRoot, err := ResolveBuildRoot(".")
	if err != nil {
		t.Fatalf("failed to resolve build root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(buildRoot, "sonic")); err != nil {
		t.Skipf("local sonic sources not available at %s: %v", filepath.Join(buildRoot, "sonic"), err)
	}

	imageRef := "sonic:local"
	if err := EnsureImages(t.Context(), []string{imageRef}, buildRoot); err != nil {
		t.Fatalf("failed to ensure image %s: %v", imageRef, err)
	}

	cli, err := NewClient()
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}
	defer func() {
		_ = cli.Close()
	}()

	if _, _, err := cli.cli.ImageInspectWithRaw(t.Context(), imageRef); err != nil {
		t.Fatalf("image %s not found after EnsureImages build: %v", imageRef, err)
	}
}

func TestEnsureImages_PullsImage(t *testing.T) {
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skipf("docker socket not available: %v", err)
	}

	buildRoot, err := ResolveBuildRoot(".")
	if err != nil {
		t.Fatalf("failed to resolve build root: %v", err)
	}

	imageRef := "hello-world:latest"
	if err := EnsureImages(t.Context(), []string{imageRef}, buildRoot); err != nil {
		t.Fatalf("failed to ensure image %s: %v", imageRef, err)
	}

	cli, err := NewClient()
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}
	defer func() { _ = cli.Close() }()

	if _, _, err := cli.cli.ImageInspectWithRaw(t.Context(), imageRef); err != nil {
		t.Fatalf("image %s not found after EnsureImages pull: %v", imageRef, err)
	}
}

func TestEnsureImage_ReportsTheImageEveryCallerGets(t *testing.T) {
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skipf("docker socket not available: %v", err)
	}

	imageRef := "hello-world:latest"
	forgetResolvedImage(t, imageRef)

	first, err := EnsureImage(t.Context(), imageRef, "")
	if err != nil {
		t.Fatalf("failed to ensure image %s: %v", imageRef, err)
	}
	if !strings.HasPrefix(first, pinnedRepository+":") {
		t.Fatalf("invalid pinned reference %q", first)
	}
	if got := imageIDOf(t, first); pinnedImageRef(got) != first {
		t.Fatalf("reference %q names image %s", first, got)
	}

	second, err := EnsureImage(t.Context(), imageRef, "")
	if err != nil {
		t.Fatalf("failed to ensure image %s again: %v", imageRef, err)
	}
	if second != first {
		t.Fatalf("invalid image, got %q, want %q", second, first)
	}
}

func TestPinnedImageRef_NamesTheImageItself(t *testing.T) {
	const imageID = "sha256:0123456789abcdef"
	if got, want := pinnedImageRef(imageID), pinnedRepository+":0123456789abcdef"; got != want {
		t.Fatalf("invalid pinned reference, got %q, want %q", got, want)
	}
}

// TestEnsureImage_KeepsTheImageWhoseReferenceIsTaken covers the host a run
// shares with every other build on it: one of them takes the reference the run
// resolved its image from, and the image the network is running on has to stay.
func TestEnsureImage_KeepsTheImageWhoseReferenceIsTaken(t *testing.T) {
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skipf("docker socket not available: %v", err)
	}

	const imageRef = "hello-world:latest"
	forgetResolvedImage(t, imageRef)

	pinned, err := EnsureImage(t.Context(), imageRef, "")
	if err != nil {
		t.Fatalf("failed to ensure image %s: %v", imageRef, err)
	}
	imageID := imageIDOf(t, pinned)
	restoreImageRef(t, imageRef, imageID)

	// Another build of the reference names its own image, which for the image
	// of this run is what dropping the reference is: nothing but its pin names
	// it any more.
	dropImageRef(t, imageRef)

	again, err := EnsureImage(t.Context(), imageRef, "")
	if err != nil {
		t.Fatalf("failed to ensure image %s again: %v", imageRef, err)
	}
	if again != pinned {
		t.Fatalf("invalid reference, got %q, want %q", again, pinned)
	}
	if got := imageIDOf(t, again); got != imageID {
		t.Fatalf("invalid image, got %s, want %s", got, imageID)
	}
}

// TestEnsureImage_RestoresAnImageTheHostLost covers a prune between two node
// starts: the image the run is using is reclaimed, and the node still has to
// start on the client the rest of the network is running.
func TestEnsureImage_RestoresAnImageTheHostLost(t *testing.T) {
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skipf("docker socket not available: %v", err)
	}

	const imageRef = "hello-world:latest"
	forgetResolvedImage(t, imageRef)

	pinned, err := EnsureImage(t.Context(), imageRef, "")
	if err != nil {
		t.Fatalf("failed to ensure image %s: %v", imageRef, err)
	}
	imageID := imageIDOf(t, pinned)
	restoreImageRef(t, imageRef, imageID)

	dropImageRef(t, pinned)
	dropImageRef(t, imageRef)
	if _, found := lookupImage(t, imageID); found {
		t.Fatalf("image %s is still available", imageID)
	}

	again, err := EnsureImage(t.Context(), imageRef, "")
	if err != nil {
		t.Fatalf("failed to ensure image %s again: %v", imageRef, err)
	}
	if again != pinned {
		t.Fatalf("invalid reference, got %q, want %q", again, pinned)
	}
	if got := imageIDOf(t, again); got != imageID {
		t.Fatalf("invalid image, got %s, want %s", got, imageID)
	}
}

// lookupImage reports the image the given reference names on this host, if the
// host has it.
func lookupImage(t *testing.T, imageRef string) (string, bool) {
	t.Helper()
	cli, err := NewClient()
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}
	defer func() {
		_ = cli.Close()
	}()

	id, err := imageID(t.Context(), cli, imageRef)
	if client.IsErrNotFound(err) {
		return "", false
	}
	if err != nil {
		t.Fatalf("failed to read the image of %q: %v", imageRef, err)
	}
	return id, true
}

// imageIDOf reports the image the given reference names on this host.
func imageIDOf(t *testing.T, imageRef string) string {
	t.Helper()
	id, found := lookupImage(t, imageRef)
	if !found {
		t.Fatalf("image %q is not available", imageRef)
	}
	return id
}

// dropImageRef drops the given reference if the host has it, and with the last
// reference naming an image the image itself.
func dropImageRef(t *testing.T, imageRef string) {
	t.Helper()
	cli, err := NewClient()
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}
	defer func() {
		_ = cli.Close()
	}()

	_, err = cli.cli.ImageRemove(t.Context(), imageRef, image.RemoveOptions{})
	if err != nil && !client.IsErrNotFound(err) {
		t.Fatalf("failed to remove image %q: %v", imageRef, err)
	}
}

// restoreImageRef puts the given reference back on the image it named once the
// test is done, so that a test dropping it does not take it from another one.
func restoreImageRef(t *testing.T, imageRef, imageID string) {
	t.Cleanup(func() {
		cli, err := NewClient()
		if err != nil {
			t.Errorf("failed to create docker client: %v", err)
			return
		}
		defer func() {
			_ = cli.Close()
		}()
		if err := cli.cli.ImageTag(context.Background(), imageID, imageRef); err != nil {
			t.Errorf("failed to restore image %q: %v", imageRef, err)
		}
	})
}
