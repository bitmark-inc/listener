// Copyright (c) 2014-2018 Bitmark Inc.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package listener_test

import (
	"crypto/tls"
	"math"
	"testing"
	"time"

	"github.com/bitmark-inc/listener"
)

// test fixed number of calls
func TestListener(t *testing.T) {

	defer teardown(t)
	if err := setup(t); nil != err {
		t.Errorf("failed to setup: %v", err)
		return
	}

	keyPair, err := tls.LoadX509KeyPair(certificateFileName, keyFileName)
	if err != nil {
		t.Errorf("failed to load keypair: %v", err)
		return
	}

	tlsConfiguration := tls.Config{
		Certificates: []tls.Certificate{
			keyPair,
		},
	}

	// make limiter to maximum parallel connections
	limiter := listener.NewLimiter(clientLoops)

	argument := &CallbackArguments{
		m: "single listener message",
		n: 1,
		t: t,
	}

	lnr, err := listener.StartListening("tcp", defaultListenAddress, &tlsConfiguration, limiter, callback, argument)
	if err != nil {
		t.Fatalf("failed to start listening: %v", err)
	}
	defer lnr.StopListening()

	// client increments this atomically on each success
	var totalSuccesses int32 = 0

	// check a single connection twice
	for i := 1; i <= staticCalls; i += 1 {
		simpleClient(t, -i, defaultAddress, &totalSuccesses)
	}

	// poll until server is idle
	for 0 != lnr.ConnectionCount() {
		time.Sleep(time.Second)
	}

	// check that limiter shows zero
	if c := limiter.CurrentCount(); c != 0 {
		t.Errorf("limiter should be zero but shows: %d", c)
	}

	// check several clients in parallel
	for i := 1; i <= clientLoops; i += 1 {
		go simpleClient(t, i, defaultAddress, &totalSuccesses)
	}

	// allow backgrounds to start so that this
	// does not stop server too soon
	time.Sleep(time.Second)

	// poll until server is idle
	for 0 != lnr.ConnectionCount() {
		time.Sleep(time.Second)
	}

	// check that there were the correct number of calls
	if expected := int32((clientLoops + staticCalls) * clientCalls); expected != totalSuccesses {
		t.Errorf("total calls expected: %d  actual calls: %d", expected, totalSuccesses)
	}

	// check that limiter shows zero
	if c := limiter.CurrentCount(); c != 0 {
		t.Errorf("limiter should be zero but shows: %d", c)
	}
}

// test rate
func TestTimedListener(t *testing.T) {

	defer teardown(t)
	if err := setup(t); nil != err {
		t.Errorf("failed to setup: %v", err)
		return
	}

	keyPair, err := tls.LoadX509KeyPair(certificateFileName, keyFileName)
	if err != nil {
		t.Errorf("failed to load keypair: %v", err)
		return
	}

	tlsConfiguration := &tls.Config{
		Certificates: []tls.Certificate{
			keyPair,
		},
	}

	r := make([]float64, len(bandwidths))
	for i, b := range bandwidths {
		r[i] = DoTimedListener(t, b.bandwidth, tlsConfiguration)
	}

loop:
	for i, b := range bandwidths {
		if b.ratio <= 0 {
			continue loop
		}
		ratio := r[i] / r[0]
		if math.Abs(ratio-b.ratio)/b.ratio >= 0.15 { // ±15%
			t.Errorf("actual: %f  expected: %f", ratio, b.ratio)
		}
	}
}

func DoTimedListener(t *testing.T, bandwidth float64, tlsConfiguration *tls.Config) float64 {

	// make limiter to maximum parallel connections
	limiter := listener.NewBandwidthLimiter(clientLoops, bandwidth)

	argument := &CallbackArguments{
		m: "single listener message",
		n: 1,
		t: t,
	}

	lnr, err := listener.StartListening("tcp", defaultListenAddress, tlsConfiguration, limiter, callback, argument)
	if err != nil {
		t.Fatalf("failed to start listening: %v", err)
	}
	defer lnr.StopListening()

	// check that limiter shows zero
	if c := limiter.CurrentCount(); c != 0 {
		t.Errorf("limiter should be zero but shows: %d", c)
	}

	// check several clients in parallel
	successes := make([]int32, clientLoops)
	for i := 0; i < len(successes); i += 1 {
		successes[i] = 0
		go timedClient(t, i+1, defaultAddress, testDuration, &successes[i])
	}

	// allow backgrounds to start so that this
	// does not stop server too soon
	time.Sleep(time.Second)

	// poll until server is idle
	for 0 != lnr.ConnectionCount() {
		time.Sleep(time.Second)
	}

	// check that there were the correct number of calls
	r := checkDeviation(t, successes)

	// check that limiter shows zero
	if c := limiter.CurrentCount(); c != 0 {
		t.Errorf("limiter should be zero but shows: %d", c)
	}

	return r
}

// check that there were the correct number of calls
func checkDeviation(t *testing.T, values []int32) float64 {

	total := 0.0
	n := len(values)

	for i, v := range values {
		t.Logf("calls[%d]: %d", i, v)
		total += float64(v)
	}
	mean := total / float64(n)

	sumsq := 0.0
	for _, v := range values {
		d := float64(v) - mean
		sumsq += d * d
	}
	standardDeviation := math.Sqrt(sumsq / float64(n-1))

	t.Logf("total: %f  mean: %f  standard deviation: %f", total, mean, standardDeviation)

	const m = 3
	limit := m * standardDeviation
	for i, v := range values {
		d := math.Abs(float64(v) - mean)
		if d > limit {
			t.Errorf("count[%d]: %d > %d × %f from: %f", i, v, m, standardDeviation, mean)
		}
	}
	return mean
}
