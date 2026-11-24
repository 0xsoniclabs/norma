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

package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRun_DeletesEverything_WhenTheRunEnds(t *testing.T) {
	tests := map[string]struct {
		env           map[string]string
		destroyFails  int
		cancelAfter   time.Duration
		wantErr       string
		wantDestroys  int
		wantCollected bool
	}{
		"the scenario passes": {
			wantDestroys:  1,
			wantCollected: true,
		},
		"the scenario fails": {
			env:           map[string]string{"FAKE_RUN_RC": "3"},
			wantErr:       "the scenario failed on the VM",
			wantDestroys:  1,
			wantCollected: true,
		},
		"the run is interrupted": {
			env:           map[string]string{"FAKE_RUN_SLEEP": "30"},
			cancelAfter:   2 * time.Second,
			wantErr:       "the scenario failed on the VM",
			wantDestroys:  1,
			wantCollected: true,
		},
		"the VM never answers": {
			env:          map[string]string{"FAKE_READY_SLEEP": "30"},
			wantErr:      "the VM was not ready after 1s",
			wantDestroys: 1,
		},
		"terraform destroy fails once": {
			destroyFails:  1,
			wantDestroys:  2,
			wantCollected: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fake := useFakeCloud(t, requiredPermissions)
			for k, v := range test.env {
				t.Setenv(k, v)
			}
			if test.destroyFails > 0 {
				require.NoError(t, os.WriteFile(filepath.Join(fake, "destroy-fails"), []byte{byte('0' + test.destroyFails)}, 0o644))
			}
			ctx := t.Context()
			if test.cancelAfter > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, test.cancelAfter)
				defer cancel()
			}
			cfg := testConfig(t)
			start := time.Now()

			collected, err := Run(ctx, cfg)

			require.Less(t, time.Since(start), 10*time.Second, "a hanging call held up the run")
			if test.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.wantErr)
			}
			require.NoFileExists(t, filepath.Join(fake, "exists"), "resources are left")
			calls := fakeCalls(t, fake)
			require.Equal(t, test.wantDestroys, countPrefix(calls, "terraform", " destroy "))
			if test.wantCollected {
				require.Len(t, collected, 1)
				require.FileExists(t, filepath.Join(cfg.OutputDir, "norma_data_eval_1_42", "measurements.csv"))
			}
		})
	}
}

func TestRun_ReportsLeftovers_WhenDestroyKeepsFailing(t *testing.T) {
	fake := useFakeCloud(t, requiredPermissions)
	require.NoError(t, os.WriteFile(filepath.Join(fake, "destroy-fails"), []byte("9"), 0o644))

	_, err := Run(t.Context(), testConfig(t))

	require.ErrorContains(t, err, "LEFTOVER resources")
	require.ErrorContains(t, err, "instances norma-run-x")
	require.Equal(t, 3, countPrefix(fakeCalls(t, fake), "terraform", " destroy "))
}

func TestRun_CreatesNothing_WhenTheRunIsRefused(t *testing.T) {
	tests := map[string]struct {
		granted []string
		cost    float64
		wantErr string
	}{
		"a permission is missing": {
			granted: requiredPermissions[1:],
			wantErr: "missing permissions in project test-project: iap.tunnelInstances.accessViaIAP",
		},
		"the worst-case cost is too high": {
			granted: requiredPermissions,
			cost:    6,
			wantErr: "the worst-case cost of $6.00 exceeds the maximum of $5.00",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fake := useFakeCloud(t, test.granted)
			cfg := testConfig(t)
			cfg.Plan.WorstCaseCost = test.cost

			_, err := Run(t.Context(), cfg)

			require.ErrorContains(t, err, test.wantErr)
			for _, call := range fakeCalls(t, fake) {
				require.False(t, strings.HasPrefix(call, "terraform") || strings.HasPrefix(call, "ssh"),
					"unexpected call: %s", call)
			}
			require.NoDirExists(t, filepath.Join(cfg.Root, "cloud-runs"))
		})
	}
}

func TestRun_PassesTheRunFlagsOn_WhenTheyNeedQuoting(t *testing.T) {
	prefix := "ssh cd /opt/norma && sudo ./build/norma run --skip-report-rendering -o /opt/norma/out --timeout 10m0s "
	tests := map[string]struct {
		flags []string
		tty   bool
		want  string
	}{
		"no flags": {
			want: prefix + "/opt/norma/scenario/s.yml",
		},
		"a value with spaces and quotes": {
			flags: []string{"--label", "it's mine"},
			want:  prefix + `--label 'it'\''s mine' /opt/norma/scenario/s.yml`,
		},
		"a terminal": {
			flags: []string{"--tui"},
			tty:   true,
			want:  prefix + "--tui /opt/norma/scenario/s.yml [tty]",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fake := useFakeCloud(t, requiredPermissions)
			cfg := testConfig(t)
			cfg.RunFlags, cfg.TTY = test.flags, test.tty

			_, err := Run(t.Context(), cfg)

			require.NoError(t, err)
			var runs []string
			for _, call := range fakeCalls(t, fake) {
				if strings.Contains(call, "norma run") {
					runs = append(runs, call)
				}
			}
			require.Equal(t, []string{test.want}, runs)
		})
	}
}

// testConfig is a run of one scenario from a minimal Norma tree.
func testConfig(t *testing.T) Config {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "Dockerfile"), []byte("FROM golang:1.27.3 AS build\n"), 0o644))
	scenario := filepath.Join(t.TempDir(), "s.yml")
	require.NoError(t, os.WriteFile(scenario, []byte("Name: s\n"), 0o644))
	return Config{
		Project: "test-project",
		Zone:    "us-central1-a",
		Plan: Plan{
			Machine:       Machine{Name: "n2-standard-4"},
			Timeout:       10 * time.Minute,
			TTL:           40 * time.Minute,
			WorstCaseCost: 0.2,
		},
		MaxCost:   5,
		Scenario:  scenario,
		OutputDir: t.TempDir(),
		Root:      root,
	}
}

// useFakeCloud puts fake terraform and gcloud first on PATH, serves the
// permission check with granted, and returns the directory the fakes record
// their calls in.
func useFakeCloud(t *testing.T, granted []string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "terraform"), []byte(fakeTerraform), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gcloud"), []byte(fakeGcloud), 0o755))
	t.Setenv("FAKE_DIR", dir)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TF_PLUGIN_CACHE_DIR", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string][]string{"permissions": granted})
	}))
	t.Cleanup(server.Close)
	oldURL, oldTimeout, oldInterval := iamURL, readyTimeout, pollInterval
	iamURL, readyTimeout, pollInterval = server.URL+"/%s", time.Second, 10*time.Millisecond
	t.Cleanup(func() { iamURL, readyTimeout, pollInterval = oldURL, oldTimeout, oldInterval })
	return dir
}

func fakeCalls(t *testing.T, dir string) []string {
	t.Helper()
	calls, err := os.ReadFile(filepath.Join(dir, "calls"))
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(calls)), "\n")
}

func countPrefix(calls []string, prefix, contains string) int {
	n := 0
	for _, call := range calls {
		if strings.HasPrefix(call, prefix) && strings.Contains(call, contains) {
			n++
		}
	}
	return n
}

// fakeTerraform records its calls; apply creates the resources, which a
// destroy removes unless "destroy-fails" says how many destroys still fail.
const fakeTerraform = `#!/bin/sh
echo "terraform $*" >> "$FAKE_DIR/calls"
case " $* " in
  *" apply "*) touch "$FAKE_DIR/exists" ;;
  *" destroy "*)
    n=$(cat "$FAKE_DIR/destroy-fails" 2>/dev/null || echo 0)
    if [ "$n" -gt 0 ]; then echo $((n - 1)) > "$FAKE_DIR/destroy-fails"; echo "destroy failed"; exit 1; fi
    rm -f "$FAKE_DIR/exists" ;;
esac
`

// fakeGcloud records the commands run over ssh, plays a VM that hangs for
// FAKE_READY_SLEEP seconds when asked whether it is ready, runs the scenario
// (exiting with FAKE_RUN_RC, or sleeping FAKE_RUN_SLEEP seconds) and produces
// one norma_data directory, and lists the VM while it exists.
const fakeGcloud = `#!/bin/sh
if [ "$1" = auth ]; then echo token; exit 0; fi
if [ "$2" = ssh ]; then
  cmd=""; tty=""; prev=""
  for a in "$@"; do
    [ "$prev" = "--command" ] && cmd="$a"
    [ "$a" = "-t" ] && tty=" [tty]"
    prev="$a"
  done
  echo "ssh $cmd$tty" >> "$FAKE_DIR/calls"
  case "$cmd" in
    "test -f /ready")
      if [ -n "$FAKE_READY_SLEEP" ]; then exec sleep "$FAKE_READY_SLEEP"; fi ;;
    *"norma run"*)
      if [ -n "$FAKE_RUN_SLEEP" ]; then exec sleep "$FAKE_RUN_SLEEP"; fi
      exit "${FAKE_RUN_RC:-0}" ;;
    *"tar --exclude"*)
      mkdir -p "$FAKE_DIR/remote/norma_data_eval_1_42"
      echo csv > "$FAKE_DIR/remote/norma_data_eval_1_42/measurements.csv"
      exec tar -C "$FAKE_DIR/remote" -czf - . ;;
    *"sudo tar -C"*) cat > /dev/null ;;
    *"df -h"*) echo usage ;;
  esac
  exit 0
fi
echo "gcloud $*" >> "$FAKE_DIR/calls"
if [ "$2" = instances ] && [ "$3" = list ] && [ -f "$FAKE_DIR/exists" ]; then echo norma-run-x; fi
`
