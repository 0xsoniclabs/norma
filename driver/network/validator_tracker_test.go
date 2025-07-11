package network

import (
	"strings"
	"testing"
)

func TestValidatorTracker_Success(t *testing.T) {
	one := ValidatorId(1)
	two := ValidatorId(2)
	three := ValidatorId(3)
	ten := ValidatorId(10)
	tests := []struct {
		name string
		ins  []*ValidatorId
		outs []ValidatorId
	}{
		{
			name: "all-nils",
			ins:  []*ValidatorId{nil, nil, nil},
			outs: []ValidatorId{1, 2, 3},
		},
		{
			name: "one-two-three",
			ins:  []*ValidatorId{&one, &two, &three},
			outs: []ValidatorId{1, 2, 3},
		},
		{
			name: "one-three-ten",
			ins:  []*ValidatorId{&one, &three, &ten},
			outs: []ValidatorId{1, 3, 10},
		},
		{
			name: "one-three-any",
			ins:  []*ValidatorId{&one, &three, nil},
			outs: []ValidatorId{1, 3, 2},
		},
		{
			name: "ten-any-any",
			ins:  []*ValidatorId{&ten, nil, nil},
			outs: []ValidatorId{10, 1, 2},
		},
		{
			name: "two-any-any",
			ins:  []*ValidatorId{&two, nil, nil},
			outs: []ValidatorId{2, 1, 3},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tracker := NewDefaultValidatorIdTracker()
			for idx, in := range test.ins {
				next, err := tracker.GetNextAvailableId()
				if err != nil {
					t.Errorf("unexpected error; %v", err)
				}

				if in != nil {
					ok, err := tracker.IsIdAvailable(next)
					if err != nil {
						t.Errorf("unexpected error; %v", err)
					}
					if !ok {
						t.Errorf("unexpected error; %d already registered", next)
					}
					next = *in
				}

				if next != test.outs[idx] {
					t.Errorf("id mismatched; got: %d, want %d", next, test.outs[idx])
				}

				if err := tracker.NotifyRegisteredId(next); err != nil {
					t.Errorf("unexpected error; %v", err)
				}

			}
		})
	}
}

func TestValidatorTracker_IdOutofBound(t *testing.T) {
	tracker := newDefaultValidatorIdTracker(1, 5)
	for _, vid := range []ValidatorId{-1, 0, 6} {
		if _, err := tracker.IsIdAvailable(vid); err == nil || !strings.Contains(err.Error(), "invalid vid") {
			t.Errorf("unexpected error; %v", err)
		}

		if err := tracker.NotifyRegisteredId(vid); err == nil || !strings.Contains(err.Error(), "invalid vid") {
			t.Errorf("unexpected error; %v", err)
		}
	}
}

func TestValidatorTracker_NoMoreId(t *testing.T) {
	tracker := newDefaultValidatorIdTracker(1, 1)
	tracker.NotifyRegisteredId(1)
	if _, err := tracker.GetNextAvailableId(); err == nil || !strings.Contains(err.Error(), "no more available id") {
		t.Errorf("unexpected error; %v", err)
	}
}
