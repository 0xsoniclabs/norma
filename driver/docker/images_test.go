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
	"reflect"
	"testing"
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
				kind:      imageBuildSonicRemote,
				clientSrc: sonicRepositoryURL,
			},
		},
		{
			name:     "local image",
			imageRef: "sonic:local",
			want: imageBuildPlan{
				kind:      imageBuildSonicLocal,
				clientSrc: "sonic",
			},
		},
		{
			name:     "tagged remote image",
			imageRef: "sonic:v2.1.2",
			want: imageBuildPlan{
				kind:      imageBuildSonicRemote,
				clientSrc: sonicRepositoryURL + "#v2.1.2",
			},
		},
		{
			name:     "non sonic image is pull-only plan",
			imageRef: "alpine",
			want: imageBuildPlan{
				kind: imageBuildNone,
			},
		},
		{
			name:     "empty sonic tag falls back to pull-only",
			imageRef: "sonic:",
			want: imageBuildPlan{
				kind: imageBuildNone,
			},
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

func TestDeduplicateAndSort(t *testing.T) {
	input := []string{"sonic:v2.1", "", "sonic", "sonic:v2.1", "   ", "alpine"}
	want := []string{"alpine", "sonic", "sonic:v2.1"}

	got := deduplicateAndSort(input)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("invalid refs, got %v, want %v", got, want)
	}
}

func TestResolveBuildRoot(t *testing.T) {
	t.Run("finds root by walking parents", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
			t.Fatalf("failed to write Dockerfile: %v", err)
		}
		scriptsDir := filepath.Join(root, "scripts")
		if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
			t.Fatalf("failed to create scripts dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(scriptsDir, "run_sonic.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("failed to write run_sonic.sh: %v", err)
		}

		deep := filepath.Join(root, "a", "b", "c")
		if err := os.MkdirAll(deep, 0o755); err != nil {
			t.Fatalf("failed to create deep directory: %v", err)
		}

		got, err := resolveBuildRoot(deep)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != root {
			t.Fatalf("invalid root, got %q, want %q", got, root)
		}
	})

	t.Run("fails when root markers are missing", func(t *testing.T) {
		start := t.TempDir()
		_, err := resolveBuildRoot(start)
		if err == nil {
			t.Fatalf("expected error, got nil")
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

	buildRoot, err := resolveBuildRoot(".")
	if err != nil {
		t.Fatalf("failed to resolve build root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(buildRoot, "sonic")); err != nil {
		t.Skipf("local sonic sources not available at %s: %v", filepath.Join(buildRoot, "sonic"), err)
	}

	if err := buildImage(t.Context(), buildRoot, "sonic:testlocal", "sonic"); err != nil {
		t.Fatalf("failed to build sonic:testlocal image: %v", err)
	}

	cli, err := NewClient()
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}

	if _, _, err := cli.cli.ImageInspectWithRaw(t.Context(), "sonic:testlocal"); err != nil {
		t.Fatalf("image sonic:testlocal not found after build: %v", err)
	}
}

func TestEnsureImages_BuildsSonicLocal(t *testing.T) {
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skipf("docker socket not available: %v", err)
	}

	buildRoot, err := resolveBuildRoot(".")
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

	buildRoot, err := resolveBuildRoot(".")
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
