package main

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"go.uber.org/mock/gomock"
)

func TestActiveNodes_TrackActiveNodes(t *testing.T) {
	nodes := activeNodes{
		data: make(map[driver.NodeID]struct{}),
	}

	shadow := make(map[driver.NodeID]struct{})
	const N = 100

	ctrl := gomock.NewController(t)
	for i := 0; i < N; i++ {
		id := fmt.Sprintf("%d", i)
		shadow[driver.NodeID(id)] = struct{}{}
		node := driver.NewMockNode(ctrl)
		node.EXPECT().GetLabel().Return(id)
		nodes.AfterNodeCreation(node)
	}

	for i := 0; i < N; i++ {
		r := rand.Intn(2)
		if r == 0 {
			id := fmt.Sprintf("%d", i)
			node := driver.NewMockNode(ctrl)
			node.EXPECT().GetLabel().Return(id)
			delete(shadow, driver.NodeID(id))
			nodes.BeforeNodeRemoval(node)
		}
	}

	for i := 0; i < N; i++ {
		id := driver.NodeID(fmt.Sprintf("%d", i))
		_, exists := shadow[id]
		if got, want := nodes.containsId(id), exists; got != want {
			t.Errorf("shadow and active nodes do not match: got: %v != want: %v", got, want)
		}
	}
}
