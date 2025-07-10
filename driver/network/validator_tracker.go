package network

import "fmt"

type ValidatorId int

// ValidatorIdTracker tracks the validator id that is currently occupied in the network
// This replaces sfc100.LastValidatorID since we now need to be able to ask for specific id
type ValidatorIdTracker interface {
	GetNextAvailableId() (ValidatorId, error)
	IsIdAvailable(vid ValidatorId) (bool, error)
	NotifyRegisteredId(vid ValidatorId) error
}

// DefaultValidatorIdTracker is the default implementation of tracker
// to be composed into a network.
type DefaultValidatorIdTracker struct {
	tracker map[ValidatorId]bool
	next    ValidatorId
	first   ValidatorId
	last    ValidatorId
}

func (t *DefaultValidatorIdTracker) IsIdAvailable(vid ValidatorId) (bool, error) {
	registered, exist := t.tracker[vid]
	if !exist {
		return false, fmt.Errorf("invalid vid; %d < %d < %d", t.first, vid, t.last)
	}
	return !registered, nil
}

func (t *DefaultValidatorIdTracker) NotifyRegisteredId(vid ValidatorId) error {
	if _, exist := t.tracker[vid]; !exist {
		return fmt.Errorf("invalid vid; %d < %d < %d", t.first, vid, t.last)
	}
	t.tracker[vid] = true
	return nil
}

func (t *DefaultValidatorIdTracker) GetNextAvailableId() (ValidatorId, error) {
	for i := t.next; i <= t.last; i++ {
		if !t.tracker[i] {
			return i, nil
		}
	}
	return 0, fmt.Errorf("no more available id")
}

const DefaultFirstValidatorId ValidatorId = 1
const DefaultLastValidatorId ValidatorId = 100

func NewDefaultValidatorIdTracker() *DefaultValidatorIdTracker {
	return newDefaultValidatorIdTracker(DefaultFirstValidatorId, DefaultLastValidatorId)
}

func newDefaultValidatorIdTracker(first ValidatorId, last ValidatorId) *DefaultValidatorIdTracker {
	capacity := last - first + 1

	tracker := make(map[ValidatorId]bool, capacity)
	for i := first; i <= last; i++ {
		tracker[i] = false
	}

	return &DefaultValidatorIdTracker{tracker: tracker, next: first, first: first, last: last}
}
