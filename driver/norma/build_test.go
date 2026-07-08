package main

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/0xsoniclabs/norma/driver/docker"
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

	want := []string{"sonic", "sonic:local", "sonic:v2.1.2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("invalid images\ngot:  %#v\nwant: %#v", got, want)
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
