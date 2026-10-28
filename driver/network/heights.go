package network

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/0xsoniclabs/norma/driver/rpc"
)

// nodeResponseTimeout bounds how long a node may take to be dialled or to
// answer a read, so that a node that is down does not hold up the others.
var nodeResponseTimeout = 5 * time.Second

// Dialer is a node whose RPC endpoint can be dialled.
type Dialer interface {
	DialRpc(ctx context.Context) (rpc.Client, error)
	IsExpectedFailure() bool
}

// DialNodes connects to the given nodes not expected to fail, or to all of
// them if every node is, as DialRandomRpc chooses. Nodes that cannot be dialled
// in time are left out; it fails only if none can be.
func DialNodes[N Dialer](ctx context.Context, nodes []N) ([]rpc.Client, error) {
	reliable := make([]N, 0, len(nodes))
	for _, node := range nodes {
		if !node.IsExpectedFailure() {
			reliable = append(reliable, node)
		}
	}
	if len(reliable) == 0 {
		reliable = nodes
	}

	dialled := make([]rpc.Client, len(reliable))
	errs := make([]error, len(reliable))
	var wg sync.WaitGroup
	for i, node := range reliable {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(ctx, nodeResponseTimeout)
			defer cancel()
			dialled[i], errs[i] = node.DialRpc(ctx)
		})
	}
	wg.Wait()

	clients := make([]rpc.Client, 0, len(reliable))
	for i, client := range dialled {
		if errs[i] == nil {
			clients = append(clients, client)
		}
	}
	if len(clients) == 0 {
		return nil, fmt.Errorf("none of %d nodes could be dialled: %w", len(reliable), errors.Join(errs...))
	}
	return clients, nil
}

// CloseAll closes the given clients.
func CloseAll(clients []rpc.Client) {
	for _, client := range clients {
		client.Close()
	}
}

// MaxOf returns the highest value read from any of the clients. A node catching
// up reports a value behind the others, so the highest is the network's.
// Clients that fail or do not answer in time are left out; it fails only if no
// client answers. Reads that ignore the context are not waited for past it.
func MaxOf[T cmp.Ordered](
	ctx context.Context, clients []rpc.Client, read func(context.Context, rpc.Client) (T, error),
) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, nodeResponseTimeout)
	defer cancel()

	type result struct {
		value T
		err   error
	}
	results := make(chan result, len(clients))
	for _, client := range clients {
		go func() {
			value, err := read(ctx, client)
			results <- result{value, err}
		}()
	}

	var highest T
	var answered bool
	var errs []error
	for range clients {
		select {
		case r := <-results:
			if r.err != nil {
				errs = append(errs, r.err)
			} else if !answered || r.value > highest {
				highest, answered = r.value, true
			}
		case <-ctx.Done():
			errs = append(errs, ctx.Err())
		}
		if ctx.Err() != nil {
			break
		}
	}
	if !answered {
		return highest, fmt.Errorf("no node answered: %w", errors.Join(errs...))
	}
	return highest, nil
}
