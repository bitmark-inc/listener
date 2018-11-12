// Copyright (c) 2014-2018 Bitmark Inc.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// A TLS listener that can be stopped properly
//
// This implements a single TLS listening channel over an underlying
// TCP connection.  Multiple clients can connect
package listener

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const oneMegaByte = 1048576

// struct to hold the connection data
type Limiter struct {
	sync.Mutex
	maximumClientCount int
	currentClientCount int
	rateLimiter        *rate.Limiter
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
		rateLimiter:        nil,
	}
}

// Create a new limiter
//
// Current count is initially zero and the maximum is specified as a
// parameter.
func NewBandwidthLimiter(maximumCount int, bandwidthbps float64) (limiter *Limiter) {
	if maximumCount <= 0 {
		panic("NewLimiter with zero or negative maximum")
	}
	return &Limiter{
		maximumClientCount: maximumCount,
		currentClientCount: 0,
		rateLimiter:        rate.NewLimiter(rate.Limit(bandwidthbps/8), oneMegaByte),
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
		panic("SetMaximum with zero or negative maximum")
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
// statistics/debugging display can look at a snapshot of limiter state.
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
// Decrement the current count on a Limiter iff the currentCount is
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

// rate limit
func (limiter *Limiter) RateLimit(bytesRequested int) bool {
	if nil != limiter.rateLimiter {
		r := limiter.rateLimiter.ReserveN(time.Now(), bytesRequested)
		if !r.OK() {
			return false
		}
		time.Sleep(r.Delay())
	}
	return true
}
