// Copyright (c) 2014-2018 Bitmark Inc.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package listener_test

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bitmark-inc/certgen"
	"github.com/bitmark-inc/logger"
)

// Common settings and functions for listener tests

// for single listener
const (
	defaultAddress       = "localhost:8222"
	defaultListenAddress = ":8222"
)

// for certificate and private key
const (
	keyFileName         = "listener.key"
	certificateFileName = "listener.crt"
	logFileName         = "listener.log"
)

// the number of test loops
const (
	staticCalls = 2
	clientLoops = 8
	clientCalls = 10

	testDuration = 65 * time.Second
)

type bwRatio struct {
	bandwidth float64 // mega bits per second
	ratio     float64 // for testing
}

var bandwidths = []bwRatio{
	{ // this is the comparison value
		bandwidth: 1 * 1000000, //  Mbps
		ratio:     0,           // zero ⇒ do not check
	},
	{
		bandwidth: 10 * 1000000, // Mbps
		ratio:     10,
	},
	{
		bandwidth: 24 * 1000000, // Mbps
		ratio:     24,
	},
}

// cleanup routine
func removeGeneratedFiles() {
	os.Remove(keyFileName)
	os.Remove(certificateFileName)
	os.Remove(logFileName)
}

func setup(t *testing.T) error {

	removeGeneratedFiles()

	// internal logger
	logging := logger.Configuration{
		Directory: ".",
		File:      logFileName,
		Size:      1048576,
		Count:     10,
		Console:   true,
		Levels: map[string]string{
			logger.DefaultTag: "info",
		},
	}

	// start logging
	if err := logger.Initialise(logging); nil != err {
		panic(fmt.Sprintf("logger setup failed with error: %s", err))
	}

	org := "just a self signed cert"
	validUntil := time.Now().Add(10 * 365 * 24 * time.Hour)
	cert, key, err := certgen.NewTLSCertPair(org, validUntil, false, nil)
	if err != nil {
		return err
	}

	if err = ioutil.WriteFile(certificateFileName, cert, 0666); err != nil {
		return err
	}
	if err = ioutil.WriteFile(keyFileName, key, 0600); err != nil {
		os.Remove(certificateFileName)
		return err
	}
	return nil
}

func teardown(t *testing.T) {
	logger.Finalise()
	removeGeneratedFiles()
}

// the test RPC type

type Args struct {
	A, B int
}

type Quotient struct {
	Quo, Rem int
}

type Arith int

func (t *Arith) Multiply(args *Args, reply *int) error {
	*reply = args.A * args.B
	return nil
}

func (t *Arith) Divide(args *Args, quo *Quotient) error {
	if args.B == 0 {
		return errors.New("divide by zero")
	}
	quo.Quo = args.A / args.B
	quo.Rem = args.A % args.B
	return nil
}

// argument type for callback
type CallbackArguments struct {
	t *testing.T
	n int
	m string
}

// listener callback
func callback(conn io.ReadWriteCloser, argument interface{}) {
	arguments := argument.(*CallbackArguments)

	if !(1 == arguments.n && "single listener message" == arguments.m || 2 == arguments.n && "multi listener message" == arguments.m) {
		arguments.t.Errorf("callback argument: %d = '%s'", arguments.n, arguments.m)
	}

	arith := new(Arith)

	server := rpc.NewServer()
	server.Register(arith)

	codec := jsonrpc.NewServerCodec(conn)
	defer codec.Close()
	server.ServeCodec(codec)
}

// used to test the listener
func simpleClient(t *testing.T, n int, address string, totalSuccesses *int32) {

	pem, err := ioutil.ReadFile(certificateFileName)
	if err != nil {
		t.Errorf("client %d, %s failed to read certificate: %v", n, address, err)
		return
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(pem)

	tlsConfiguration := tls.Config{
		RootCAs: pool,
	}

	conn, err := tls.Dial("tcp", address, &tlsConfiguration)

	if err != nil {
		t.Errorf("client %d, %s failed to dial: %v", n, address, err)
		return
	}
	defer conn.Close()

	args := &Args{7, 8}
	var reply int

	c := jsonrpc.NewClient(conn)

	for i := 0; i < clientCalls; i += 1 {
		err = c.Call("Arith.Multiply", args, &reply)
		if err != nil {
			t.Errorf("client %d, %s arith error: %v", n, address, err)
		}
		wanted := args.A * args.B
		if reply == wanted {
			atomic.AddInt32(totalSuccesses, 1)
		} else {
			t.Errorf("client %d, %s wanted: %d, got %v", n, address, wanted, reply)
		}

		// uncomment to debug
		t.Logf("client %d, %s [%d]: %d * %d = %d", n, address, i, args.A, args.B, reply)

		args.A += 3*i + n
		args.B += 5*i - n
	}
}

// used to test the listener
func timedClient(t *testing.T, n int, address string, dur time.Duration, totalSuccesses *int32) {

	pem, err := ioutil.ReadFile(certificateFileName)
	if err != nil {
		t.Errorf("client %d, %s failed to read certificate: %v", n, address, err)
		return
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(pem)

	tlsConfiguration := tls.Config{
		RootCAs: pool,
	}

	conn, err := tls.Dial("tcp", address, &tlsConfiguration)

	if err != nil {
		t.Errorf("client %d, %s failed to dial: %v", n, address, err)
		return
	}
	defer conn.Close()

	args := &Args{7, 8}
	var reply int

	c := jsonrpc.NewClient(conn)

	end := time.Now().Add(dur)
	for i := 0; time.Now().Before(end); i += 1 {
		err = c.Call("Arith.Multiply", args, &reply)
		if err != nil {
			t.Errorf("client %d, %s arith error: %v", n, address, err)
		}
		wanted := args.A * args.B
		if reply == wanted {
			atomic.AddInt32(totalSuccesses, 1)
		} else {
			t.Errorf("client %d, %s wanted: %d, got %v", n, address, wanted, reply)
		}

		// uncomment to debug
		//t.Logf("client %d, %s [%d]: %d * %d = %d", n, address, i, args.A, args.B, reply)

		args.A += 3*i + n
		args.B += 5*i - n
	}
}
