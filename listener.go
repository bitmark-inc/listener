// Copyright (c) 2014-2015 Bitmark Inc.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// A TLS listener that can be stopped properly
//
// This implements a single TLS listening channel over an underlying
// TCP connection.  Multiple clients can connect.  A stop signal
// allows the server to disconnect the clients in an orderly way.
package listener

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// set true to enable debugging
const (
	//debug debugging = true
	debug debugging = false
)

type debugging bool

func (debug debugging) Printf(format string, arguments ...interface{}) {
	if debug {
		fmt.Printf(format, arguments...)
	}
}

// struct to hold the connection data
type Listener struct {
	tlsConfiguration *tls.Config
	callback         Callback
	limiter          *Limiter
	socket           *net.TCPListener
	waitGroup        *sync.WaitGroup
	clientCount      int32
	shutdown         chan bool
	argument         interface{}
}

// type to represent the connection as seen by the callback routine
//
// Behaves as io.ReadWriteCloser and contains non-exported fields
// allow access to the underlying go routine handling the connection.
type ClientConnection struct {
	io.ReadWriteCloser
	conn       *tls.Conn
	request    chan<- int
	queue      <-chan []byte
	closed     uint32
	readError  error // this save error to feed back to callback
	writeError error // this is to abort the main loop as rpc server currenty does not detect write errors
}

// the call back routine reads, writes and finally closes a connection
type Callback func(conn io.ReadWriteCloser, argument interface{})

// read from the Client connection
//
// This interfaces with the underlying listener to provide an orderly
// shutdown of the connection if the listener is stopped
func (conn *ClientConnection) Read(p []byte) (n int, err error) {
	if nil != conn.readError {
		debug.Printf("ERROR: %v\n", conn.readError)
		return 0, conn.readError
	}
	debug.Printf("Waiting for data...\n")
	conn.request <- cap(p) // send the size
	data := <-conn.queue   // fetch data
	debug.Printf("data = '%v'\n", data)
	bytesRead := len(data)
	if 0 == bytesRead {
		conn.readError = io.EOF
		return 0, io.EOF
	}
	debug.Printf("data len: %d, cap: %d\n", len(data), cap(data))
	debug.Printf("p1   len: %d, cap: %d\n", len(p), cap(p))
	copy(p, data[0:bytesRead])
	debug.Printf("p2   len: %d, cap: %d\n", len(p), cap(p))
	return bytesRead, nil
}

// Write to Client connection
//
// Just write to underlying TLS connection since writes are always
// allowed to complete.  The stop occurs just prior to actual read
// from the TLS connection so that the remote gets a reply to its
// outstanding request before shutdown.
func (conn *ClientConnection) Write(p []byte) (n int, err error) {
	debug.Printf("write len: %d, data: %v\n", len(p), p)
	n, err = conn.conn.Write(p)
	if nil != err {
		conn.writeError = err
	}
	debug.Printf("wrote bytes: %d  err: %v\n", n, err)
	return
}

// Close the Client connection
func (conn *ClientConnection) Close() error {
	if atomic.CompareAndSwapUint32(&conn.closed, 0, 1) {
		close(conn.request)
	}
	return nil
}

// Start a new listener instance
//
// Open a listining port and start a go routine to handle the actual
// accepts.  Return a struct that can be used to shutdown the listener
// and all active connections in an orderly manner.
//
// Note that tcpversion must be either "tcp4" or "tcp6"
// and the listenAddress must be the a valip IPv4:port or IPv6:port
func StartListening(tcpVersion string, listenAddress string, tlsConfiguration *tls.Config, limiter *Limiter, callback Callback, argument interface{}) (*Listener, error) {

	address, err := net.ResolveTCPAddr(tcpVersion, listenAddress)
	if err != nil {
		return nil, err
	}
	l, err := net.ListenTCP(tcpVersion, address)
	if err != nil {
		return nil, err
	}

	listener := Listener{
		callback:         callback,
		limiter:          limiter,
		tlsConfiguration: tlsConfiguration,
		socket:           l,
		shutdown:         make(chan bool),
		waitGroup:        &sync.WaitGroup{},
		clientCount:      0,
		argument:         argument,
	}
	go server(&listener)
	return &listener, nil
}

// Stop the listener
//
// Stops acceptiong new connections.  Stops all active connections
// just before their next read (their writes complete so a to respond
// to their last request).
func (listener *Listener) StopListening() error {
	debug.Printf("\nInitiate shutdown\n")
	close(listener.shutdown)

	debug.Printf("\nWait for connections to close\n")
	listener.waitGroup.Wait()
	return nil
}

// Get the current number of connections
func (listener *Listener) ConnectionCount() int32 {
	return listener.clientCount
}

// The server main loop
//
// Waits for incoming connections ans starts a go routine to handle them.
// Monitors for shutdown requests and does orderly termination.
func server(listener *Listener) {

	defer listener.socket.Close() // to stop listening on shutdown

	listener.waitGroup.Add(1)       // this is for the listen
	defer listener.waitGroup.Done() // this is for the listen

loop:
	for {
		select {
		case <-listener.shutdown:
			debug.Printf("Shutdown accept\n")
			break loop
		default:
		}

		listener.socket.SetDeadline(time.Now().Add(time.Second))
		conn, err := listener.socket.Accept()
		if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
			//debug.Printf("Accept timeout\n")
			continue
		}
		if err != nil {
			debug.Printf("Accept error: %v\n", err)
			continue
		}

		// restrict number of clients
		if nil != listener.limiter && !listener.limiter.Increment() {
			conn.Close()
			continue
		}

		// set keep alive etc.
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			tcpConn.SetKeepAlive(true)
			tcpConn.SetKeepAlivePeriod(5 * time.Minute)
			tcpConn.SetNoDelay(true)
		}

		// connection accepted - handle data
		listener.waitGroup.Add(1)                 // this counts the new connection
		atomic.AddInt32(&listener.clientCount, 1) // count the new client connection
		go func() {
			defer conn.Close() // ensure the low-level TCP socket will be closed
			tlsConn := tls.Server(conn, listener.tlsConfiguration)
			defer listener.waitGroup.Done() // this ends the new connection
			defer tlsConn.Close()
			if nil != listener.limiter {
				defer listener.limiter.Decrement()
			}

			// to feedback the number of bytes requested
			// corresponding close is done by client connection close routine
			request := make(chan int) //

			queue := make(chan []byte) // to send the read data to server
			defer close(queue)         // this will close the Client connection

			endConnection := make(chan bool)

			clientConnection := ClientConnection{
				conn:       tlsConn,
				request:    request,
				queue:      queue,
				closed:     0,
				readError:  nil, // to send status to callback
				writeError: nil, // any error here must terminate the connection
			}
			// start the connection handler
			go func() {
				defer atomic.AddInt32(&listener.clientCount, -1) // connection is finished
				defer close(endConnection)
				defer clientConnection.Close()
				listener.callback(&clientConnection, listener.argument)
			}()

			bytesRequested := 0
		serving:
			for {
				if 0 == bytesRequested {
					// wait for the byte count
					select {
					case <-listener.shutdown:
						debug.Printf("Shutdown serving connection\n")
						break serving
					case <-endConnection:
						debug.Printf("Shutdown serving connection\n")
						break serving
					case bytesRequested = <-request:
					}
				} else {
					// use current bytecount
					select {
					case <-listener.shutdown:
						debug.Printf("Shutdown serving connection\n")
						break serving
					case <-endConnection:
						debug.Printf("Shutdown serving connection\n")
						break serving
					default:
					}
				}
				debug.Printf("Requested bytes = %d\n", bytesRequested)

				// since the rpc callbacks do not appear to detect a write error
				// we detect it here
				if nil != clientConnection.writeError {
					debug.Printf("Connection closed by write error: %v\n", clientConnection.writeError)
					break
				}

				// only time out read, must not affect writes
				tlsConn.SetReadDeadline(time.Now().Add(time.Second))
				buffer := make([]byte, bytesRequested)
				n, err := tlsConn.Read(buffer)
				if nil != err {
					// timeout => loop around an try the same Read again
					if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
						// debug.Printf("Read timeout\n")
						debug.Printf("R ")
						continue
					}
					if io.EOF == err {
						debug.Printf("Connection closed by client: %v\n", err)
						break
					}
					if err != nil {
						debug.Printf("Read error: %v\n", err)
						break
					}
				}
				debug.Printf("buffer len: %d, cap: %d\n", n, cap(buffer))
				queue <- buffer[0:n]
				bytesRequested = 0
			}
			debug.Printf("Finish handler\n")
		}()
	}
	debug.Printf("Exiting server loop\n")
}
