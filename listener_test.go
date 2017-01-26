// Copyright (c) 2014-2017 Bitmark Inc.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package listener_test

import (
	"crypto/tls"
	"github.com/bitmark-inc/listener"
	"testing"
	"time"
)

// main test routine
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
		t.Errorf("failed to start listening: %v", err)
		return
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
