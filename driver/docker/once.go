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
	"sync"
)

// onceByKey runs a piece of work once per key and remembers its result. Callers
// arriving while it runs wait for it, or for their own context to end; work
// that fails is not remembered, so a later caller runs it again.
type onceByKey[V any] struct {
	mu   sync.Mutex
	runs map[string]*run[V]
}

type run[V any] struct {
	done  chan struct{}
	value V
	err   error
}

func (o *onceByKey[V]) do(ctx context.Context, key string, work func() (V, error)) (V, error) {
	for {
		o.mu.Lock()
		if o.runs == nil {
			o.runs = map[string]*run[V]{}
		}
		r, running := o.runs[key]
		if !running {
			r = &run[V]{done: make(chan struct{})}
			o.runs[key] = r
		}
		o.mu.Unlock()

		if !running {
			r.value, r.err = work()
			if r.err != nil {
				o.mu.Lock()
				delete(o.runs, key)
				o.mu.Unlock()
			}
			close(r.done)
			return r.value, r.err
		}

		select {
		case <-ctx.Done():
			var zero V
			return zero, ctx.Err()
		case <-r.done:
		}
		// The failure of another caller's run, such as its context ending,
		// is no verdict on this caller's; it runs the work itself.
		if r.err == nil {
			return r.value, nil
		}
	}
}
