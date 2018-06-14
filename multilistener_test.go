// Copyright (c) 2014-2017 Bitmark Inc.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package listener_test

import (
	"crypto/tls"
	"testing"
	"time"

	"github.com/bitmark-inc/listener"
)

// main test routine
func TestMultiConnection(t *testing.T) {

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

	// these two arrays need to correspond
	addresses := []string{defaultListenAddress, "127.0.0.1:8223", "[::]:8224"}
	clientAddresses := [...]string{defaultAddress, "127.0.0.1:8223", "[::1]:8224"}

	argument := &CallbackArguments{
		m: "multi listener message",
		n: 2,
		t: t,
	}

	lnr, err := listener.NewMultiListener("test", addresses, &tlsConfiguration, limiter, callback)
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
