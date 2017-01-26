// Copyright (c) 2014-2017 Bitmark Inc.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package listener

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/bitmark-inc/logger"
	"net"
	"sync/atomic"
)

const (
	stateStopped = 0
	stateStarted = 1
)

// structure to hold multi-listener instance
type MultiListener struct {
	state            int32
	log              *logger.L
	tlsConfiguration *tls.Config
	callback         Callback
	limiter          *Limiter
	listeners        []*channel
}

// holds one channel
type channel struct {
	tcp      string
	address  string
	listener *Listener
}

// Create a new multi-listener
//
// This will fail if any bad host:port strings are given.
func NewMultiListener(name string,
	listenAddresses []string,
	tlsConfiguration *tls.Config,
	limiter *Limiter,
	callback Callback) (*MultiListener, error) {

	log := logger.New(name)

	server := MultiListener{
		state:            stateStopped,
		log:              log,
		tlsConfiguration: tlsConfiguration,
		callback:         callback,
		limiter:          limiter,
		listeners:        nil,
	}

	// to hold all listeners
	listeners := make([]*channel, 0, len(listenAddresses))

	// ensure all listeners are valid IP addresses
	for _, hostPort := range listenAddresses {
		host, _, err := net.SplitHostPort(hostPort)
		if nil != err {
			e := fmt.Sprintf("Invalid host:port string: '%s'", hostPort)
			log.Critical(e)
			return nil, errors.New(e)
		}

		tcp := "tcp"
		if "" != host {

			// only IPs are supported not hostnames
			IP := net.ParseIP(host)
			if nil == IP {
				e := fmt.Sprintf("Invalid IP Address string: '%s'", host)
				log.Critical(e)
				return nil, errors.New(e)
			}
			tcp = "tcp4"
			if nil == IP.To4() {
				tcp = "tcp6"
			}
		}
		c := &channel{
			tcp:      tcp,
			address:  hostPort,
			listener: nil,
		}
		listeners = append(listeners, c)

	}
	if 0 == len(listeners) {
		e := "No listen addresses were found"
		log.Critical(e)
		return nil, errors.New(e)
	}

	server.listeners = listeners

	return &server, nil
}

// Start the server
func (server *MultiListener) Start(argument interface{}) {

	// no need to start if already started
	if stateStarted == atomic.SwapInt32(&server.state, stateStarted) {
		server.log.Warn("already started")
		return
	}

	// start all listeners
	for i, l := range server.listeners {

		// start one listener
		listener, err := StartListening(l.tcp, l.address, server.tlsConfiguration, server.limiter, server.callback, argument)
		if err != nil {
			server.log.Warnf("listen %d failed for %s/%s error: %v", i, l.tcp, l.address, err)
			continue
		}

		l.listener = listener
	}
}

// Stop the rpc server
func (server *MultiListener) Stop() {

	// no need to stop if already stopped
	if stateStopped == atomic.SwapInt32(&server.state, stateStopped) {
		server.log.Warn("already stopped")
		return
	}

	server.log.Info("stop requested")

	// stop all listeners
	for i, l := range server.listeners {

		if nil != l.listener {
			server.log.Infof("stopping %d: %s/%s", i, l.tcp, l.address)
			// stop one listener
			err := l.listener.StopListening()
			if err != nil {
				server.log.Warnf("stop %d failed for %s/%s error: %v", i, l.tcp, l.address, err)
			}

			l.listener = nil
		}
	}
}

// Total up the connections
func (server *MultiListener) ConnectionCount() int32 {

	count := int32(0)

	for _, l := range server.listeners {

		if nil != l.listener {
			count += l.listener.ConnectionCount()
		}
	}
	return count
}
