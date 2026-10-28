package network

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// A node that does not answer is left out, also when its read ignores the
// context: the other nodes still say how far the network is.
func TestMaxOf_LeavesOutNodesThatFailOrHang(t *testing.T) {
	original := nodeResponseTimeout
	nodeResponseTimeout = 50 * time.Millisecond
	t.Cleanup(func() { nodeResponseTimeout = original })

	ctrl := gomock.NewController(t)
	clients := []rpc.Client{rpc.NewMockClient(ctrl), rpc.NewMockClient(ctrl), rpc.NewMockClient(ctrl)}
	hang := make(chan struct{})
	t.Cleanup(func() { close(hang) })
	reads := map[rpc.Client]func() (uint64, error){
		clients[0]: func() (uint64, error) { return 7, nil },
		clients[1]: func() (uint64, error) { return 0, errors.New("down") },
		clients[2]: func() (uint64, error) { <-hang; return 100, nil },
	}

	got, err := MaxOf(t.Context(), clients, func(_ context.Context, client rpc.Client) (uint64, error) {
		return reads[client]()
	})
	require.NoError(t, err)
	require.Equal(t, uint64(7), got)
}

func TestMaxOf_Fails_WhenNoNodeAnswers(t *testing.T) {
	clients := []rpc.Client{rpc.NewMockClient(gomock.NewController(t))}
	_, err := MaxOf(t.Context(), clients, func(context.Context, rpc.Client) (uint64, error) {
		return 0, errors.New("down")
	})
	require.ErrorContains(t, err, "no node answered: down")
}
