// Copyright (c) 2014-2017 Bitmark Inc.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package listener_test

import (
	"github.com/bitmark-inc/listener"
	"testing"
)

const (
	maximumLimit1 = 5
	maximumLimit2 = 20
)

// main test routine
func TestLimiter(t *testing.T) {

	l := listener.NewLimiter(maximumLimit1)

	checkInc(t, l, maximumLimit1, true)
	checkInc(t, l, maximumLimit1, false)

	checkDec(t, l, maximumLimit1, true)
	checkDec(t, l, maximumLimit1, false)

	l.SetMaximum(maximumLimit2)

	checkInc(t, l, maximumLimit2, true)
	checkInc(t, l, maximumLimit2, false)

	checkDec(t, l, maximumLimit2, true)
	checkDec(t, l, maximumLimit2, false)
}

func checkInc(t *testing.T, l *listener.Limiter, maximumLimit int, state bool) {

	// these increments should all return true
	for i := 0; i < maximumLimit; i += 1 {
		if actual := l.Increment(); state != actual {
			t.Errorf("%d: increment state was: %v wanted: %v", i, actual, state)
		}
	}

	// check reached maximum correctly
	if maximumLimit != l.CurrentCount() {
		t.Errorf("failed to reach %d only counted to: %d", maximumLimit, l.CurrentCount())
	}
}

func checkDec(t *testing.T, l *listener.Limiter, maximumLimit int, state bool) {

	// these decrements should all return true
	for i := 0; i < maximumLimit; i += 1 {
		if actual := l.Decrement(); state != actual {
			t.Errorf("%d: decrement state was: %v wanted: %v", i, actual, state)
		}
	}

	// check zero
	if 0 != l.CurrentCount() {
		t.Errorf("failed to reach %d only counted to: %d", 0, l.CurrentCount())
	}

}
