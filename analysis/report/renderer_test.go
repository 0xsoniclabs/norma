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

package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRender_BuildsTheRendererImage_OnlyWhenItIsMissing(t *testing.T) {
	inspect := "image inspect " + rendererImage
	build := "build -t " + rendererImage + " -"
	tests := map[string]struct {
		imageExists bool
		buildError  string
		wantCalls   []string
		wantRender  bool
	}{
		"an existing image is used": {
			imageExists: true,
			wantCalls:   []string{inspect},
			wantRender:  true,
		},
		"a missing image is built": {
			wantCalls:  []string{inspect, build},
			wantRender: true,
		},
		"a failed build is reported and nothing is rendered": {
			buildError: "pull access denied for rocker/tidyverse",
			wantCalls:  []string{inspect, build},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(fakeDocker), 0o755))
			if test.imageExists {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "image"), nil, 0o644))
			}
			if test.buildError != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "build-error"), []byte(test.buildError), 0o644))
			}
			t.Setenv("FAKE_DOCKER_DIR", dir)
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

			_, err := SingleEvalReport.Render(filepath.Join(dir, "measurements.csv"), t.TempDir(), "", "", "")

			calls, readErr := os.ReadFile(filepath.Join(dir, "calls"))
			require.NoError(t, readErr)
			lines := strings.Split(strings.TrimSpace(string(calls)), "\n")
			if test.wantRender {
				require.NoError(t, err)
				require.Len(t, lines, len(test.wantCalls)+1)
				require.True(t, strings.HasPrefix(lines[len(lines)-1], "run "), "last call: %s", lines[len(lines)-1])
				lines = lines[:len(lines)-1]
			} else {
				require.ErrorContains(t, err, test.buildError)
			}
			require.Equal(t, test.wantCalls, lines)

			if len(test.wantCalls) > 1 {
				dockerfile, err := os.ReadFile(filepath.Join(dir, "dockerfile"))
				require.NoError(t, err)
				require.Equal(t, rendererDockerfile, dockerfile)
			}
		})
	}
}

// fakeDocker logs its arguments, saves what a build reads from stdin, finds
// the renderer image only if the file "image" exists, and fails a build with
// the message in "build-error".
const fakeDocker = `#!/bin/sh
echo "$*" >> "$FAKE_DOCKER_DIR/calls"
case "$1" in
  image) [ -f "$FAKE_DOCKER_DIR/image" ] ;;
  build)
    cat > "$FAKE_DOCKER_DIR/dockerfile"
    if [ -f "$FAKE_DOCKER_DIR/build-error" ]; then cat "$FAKE_DOCKER_DIR/build-error"; exit 1; fi ;;
esac
`
