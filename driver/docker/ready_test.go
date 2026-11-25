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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClient_WaitReady_ReturnsOnceTheDaemonAnswers_WhenItStartsLate(t *testing.T) {
	tests := map[string]struct {
		failures int64
		wantErr  string
	}{
		"answers at once":         {},
		"answers after two tries": {failures: 2},
		"never answers":           {failures: 1000, wantErr: "does not answer"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			client := newFakeDaemonClient(t, test.failures)

			err := client.WaitReady(t.Context(), 2*time.Second)

			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

// newFakeDaemonClient is a client of a daemon that fails its first pings.
func newFakeDaemonClient(t *testing.T, failures int64) *Client {
	t.Helper()
	var pings atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pings.Add(1) <= failures {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Api-Version", "1.47")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	client, err := NewClientAt("tcp://" + strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)
	return client
}
