package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
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

func TestCollectSonicImages(t *testing.T) {
	tmp := t.TempDir()

	scenarioA := `
name: scenario-a
duration: 5
validators:
  - name: validator-a
    imagename: sonic:v2.1.2
nodes:
  - name: node-a
    client:
      imagename: sonic:local
`
	scenarioB := `
name: scenario-b
duration: 3
nodes:
  - name: node-b
`

	pathA := filepath.Join(tmp, "a.yml")
	pathB := filepath.Join(tmp, "b.yml")
	if err := os.WriteFile(pathA, []byte(scenarioA), 0644); err != nil {
		t.Fatalf("failed to write scenario A: %v", err)
	}
	if err := os.WriteFile(pathB, []byte(scenarioB), 0644); err != nil {
		t.Fatalf("failed to write scenario B: %v", err)
	}

	got, err := collectSonicImages([]string{pathA, pathB})
	if err != nil {
		t.Fatalf("collectSonicImages() failed: %v", err)
	}

	want := []string{"sonic", "sonic:local", "sonic:v2.1.2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("invalid images\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestSonicBuildContext(t *testing.T) {
	tests := []struct {
		name      string
		image     string
		want      string
		wantError bool
	}{
		{
			name:  "default sonic",
			image: "sonic",
			want:  "https://github.com/0xsoniclabs/sonic.git",
		},
		{
			name:  "local",
			image: "sonic:local",
			want:  "sonic",
		},
		{
			name:  "version tag",
			image: "sonic:v2.1.1",
			want:  "https://github.com/0xsoniclabs/sonic.git#v2.1.1",
		},
		{
			name:      "invalid",
			image:     "alpine:latest",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sonicBuildContext(tt.image)
			if (err != nil) != tt.wantError {
				t.Fatalf("unexpected error state: %v", err)
			}
			if tt.wantError {
				return
			}
			if got != tt.want {
				t.Fatalf("invalid build context\ngot:  %q\nwant: %q", got, tt.want)
			}
		})
	}
}
