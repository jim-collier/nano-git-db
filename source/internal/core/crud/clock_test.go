// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package crud

import (
	"testing"
	"time"
)

// now() must be strictly increasing even under rapid calls or a wall clock
// stepping backwards - replay order depends on it.
func TestNowIsStrictlyMonotonic(t *testing.T) {
	a, _, _ := newAPI(t)
	prev := a.now()
	for i := 0; i < 1000; i++ {
		cur := a.now()
		if cur <= prev {
			t.Fatalf("timestamp went backwards: %s then %s", prev, cur)
		}
		prev = cur
	}
}

func TestNowSurvivesClockStepBack(t *testing.T) {
	a, _, _ := newAPI(t)
	future := time.Now().UTC().Add(time.Hour).Format(tsLayout)
	a.lastDate = future
	got := a.now()
	if got <= future {
		t.Fatalf("now() = %s, want > pinned %s", got, future)
	}
}

func TestNewIDIsTimeOrdered(t *testing.T) {
	prev := newID()
	for i := 0; i < 100; i++ {
		cur := newID()
		if cur < prev {
			// v7 ids embed a ms timestamp + monotonic counter; within one
			// process they must not sort backwards.
			t.Fatalf("id went backwards: %s then %s", prev, cur)
		}
		prev = cur
	}
}
