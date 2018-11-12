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

// these two arrays need to correspond
var listenAddresses = []string{defaultListenAddress, "127.0.0.1:8223", "[::1]:8224", "[::1]:8225"}
var clientAddresses = []string{defaultAddress, "127.0.0.1:8223", "[::1]:8224", "[::1]:8225"}

// check that test data is ok
func TestMultiConnectionData(t *testing.T) {

	if len(listenAddresses) != len(clientAddresses) {
		t.Fatalf("len(listenAddresses): %d != len(clientAddresses):%d", len(listenAddresses), len(clientAddresses))
	}
	if clientLoops < len(clientAddresses) {
		t.Fatalf("clientLoops: %d < len(clientAddresses): %d", clientLoops, len(clientAddresses))
	}
	if 0 != clientLoops%len(clientAddresses) {
		t.Fatalf("clientLoops: %d mod len(clientAddresses): %d = %d  expect zero", clientLoops, len(clientAddresses), clientLoops%len(clientAddresses))
	}
}

// test fixed number of calls
func TestMultiConnection(t *testing.T) {

	defer teardown(t)
	if err := setup(t); nil != err {
		t.Errorf("failed to setup: %v", err)
		return
	}

	keyPair, err := tls.LoadX509KeyPair(certificateFileName, keyFileName)
	if err != nil {
		t.Fatalf("failed to load keypair: %v", err)
	}

	tlsConfiguration := tls.Config{
		Certificates: []tls.Certificate{
			keyPair,
		},
	}

	// make limiter to maximum parallel connections
	limiter := listener.NewLimiter(clientLoops)

	argument := &CallbackArguments{
		m: "multi listener message",
		n: 2,
		t: t,
	}

	lnr, err := listener.NewMultiListener("test", listenAddresses, &tlsConfiguration, limiter, callback)
	if err != nil {
		t.Errorf("failed to start listening: %v", err)
		return
	}
	defer lnr.Stop()
	lnr.Start(argument)

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
		go simpleClient(t, i, clientAddresses[i%len(clientAddresses)], &totalSuccesses)
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
func TestTimedMultiConnection(t *testing.T) {

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
		r[i] = DoTimedMultiConnection(t, b.bandwidth, tlsConfiguration)
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

func DoTimedMultiConnection(t *testing.T, bandwidth float64, tlsConfiguration *tls.Config) float64 {

	// make limiter to maximum parallel connections
	limiter := listener.NewBandwidthLimiter(clientLoops, bandwidth)

	argument := &CallbackArguments{
		m: "multi listener message",
		n: 2,
		t: t,
	}

	lnr, err := listener.NewMultiListener("test", listenAddresses, tlsConfiguration, limiter, callback)
	if err != nil {
		t.Fatalf("failed to start listening: %v", err)
	}
	defer lnr.Stop()
	lnr.Start(argument)

	// check that limiter shows zero
	if c := limiter.CurrentCount(); c != 0 {
		t.Errorf("limiter should be zero but shows: %d", c)
	}

	// check several clients in parallel
	successes := make([]int32, clientLoops)
	for i := 0; i < len(successes); i += 1 {
		successes[i] = 0
		go timedClient(t, i+1, clientAddresses[i%len(clientAddresses)], testDuration, &successes[i])
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
