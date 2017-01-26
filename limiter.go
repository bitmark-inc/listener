// Copyright (c) 2014-2017 Bitmark Inc.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// A TLS listener that can be stopped properly
//
// This implements a single TLS listening channel over an underlying
// TCP connection.  Multiple clients can connect
package listener

import (
	"sync"
)

// struct to hold the connection data
type Limiter struct {
	sync.Mutex
	maximumClientCount int
	currentClientCount int
}

// Create a new limiter
//
// Current count is initially zero and the maximum is specified as a
// parameter.
func NewLimiter(maximumCount int) (limiter *Limiter) {
	if maximumCount <= 0 {
		panic("NewLimiter with zero or negative maximum")
	}
	return &Limiter{
		maximumClientCount: maximumCount,
		currentClientCount: 0,
	}
}

// Set maximum count
//
// Change the maximum count, returning the current maximum.  This will
// not affect the current count in any way so reducing the maximum
// below the current will only serve to cause increment return false.
// Decrement will have to be used to bring the limiter down.
func (limiter *Limiter) SetMaximum(maximumCount int) int {
	if maximumCount <= 0 {
		panic("SetLimiter with zero or negative maximum")
	}
	limiter.Lock()
	defer limiter.Unlock()
	old := limiter.maximumClientCount
	limiter.maximumClientCount = maximumCount
	return old
}

// Get current count
//
// Read the current count, note that this may not be correct if other
// go routines are accessing the limiter.  This is really intended so
// statistics/debugging display can look at a snapshote of limiter state.
func (limiter *Limiter) CurrentCount() int {
	limiter.Lock()
	defer limiter.Unlock()
	return limiter.currentClientCount
}

// Increment count
//
// Increment the current count on a Limiter iff the currentCount is
// less than maximum.
func (limiter *Limiter) Increment() bool {
	limiter.Lock()
	defer limiter.Unlock()
	if limiter.currentClientCount < limiter.maximumClientCount {
		limiter.currentClientCount += 1
		return true
	}
	return false
}

// Decrement count
//
// Deccrement the current count on a Limiter iff the currentCount is
// greater than zero.
func (limiter *Limiter) Decrement() bool {
	limiter.Lock()
	defer limiter.Unlock()
	if limiter.currentClientCount > 0 {
		limiter.currentClientCount -= 1
		return true
	}
	return false
}
