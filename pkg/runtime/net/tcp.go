package net

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

type (
	TCPLayer struct {
		outChannel        chan TransportMessage
		outChannelEvents  chan TransportConnEvents
		connectChan       chan TransportHost
		activeConnections map[TransportHost]*TCPConnection
		mutex             sync.Mutex // Mutex to protect concurrent access to activeConnections
		self              TransportHost
	}

	TCPConnection struct {
		tcpConn   net.Conn
		inChannel chan TransportConnEvents
	}
)

// NewTCPConnection creates a new TCPConnection
func NewTCPConnection(tcpConn net.Conn) *TCPConnection {
	return &TCPConnection{
		tcpConn:   tcpConn,
		inChannel: make(chan TransportConnEvents, 10),
	}
}

// NewTCPLayer creates a new TCPLayer and starts the listener
func NewTCPLayer(self TransportHost, ctx context.Context) *TCPLayer {
	tcpLayer := &TCPLayer{
		outChannel:        make(chan TransportMessage, 10),
		outChannelEvents:  make(chan TransportConnEvents),
		connectChan:       make(chan TransportHost),
		activeConnections: make(map[TransportHost]*TCPConnection),
		self:              self,
	}
	tcpLayer.listen(ctx) // Starting the listener in a goroutine
	return tcpLayer
}

// Send sends a message to the specified host
func (t *TCPLayer) Send(networkMessage TransportMessage, sendTo TransportHost) {
	conn, ok := t.getActiveConn(sendTo)
	if !ok {
		// TODO: Properly log this, trying to send message to non active connection! (propagate error? or just log?)
		return
	} else {
		_, err := conn.tcpConn.Write(networkMessage.Msg.Bytes())
		if err != nil {
			t.outChannelEvents <- TransportConnFailed
			t.Disconnect(networkMessage.Host) // TODO: Replace with sendTo Host
		}
	}
}

// Connect connects to the specified host
func (t *TCPLayer) Connect(host TransportHost) {
	t.connectChan <- host
}

// Disconnect disconnects from the specified host
func (t *TCPLayer) Disconnect(host TransportHost) {
	conn, ok := t.removeActiveConn(host)
	if ok {
		err := conn.tcpConn.Close()
		if err != nil {
			// TODO: Proper Logging
		}
		println("Does it block here 1?")
		t.outChannelEvents <- TransportConnDisconnected
		println("Does it block here 2?")
	} else {
		// TODO: Proper Logging
	}
}

// OutChannel returns the channel for outgoing messages
func (t *TCPLayer) OutChannel() chan TransportMessage {
	return t.outChannel
}

// OutChannelEvents returns the channel for outgoing events
func (t *TCPLayer) OutChannelEvents() chan TransportConnEvents {
	return t.outChannelEvents
}

// listen starts the goroutines that:
//
// handle tcp new connections and launch the appropriate contexts
// handle requests to connect to a remote server
func (t *TCPLayer) listen(ctx context.Context) {
	listener, err := net.Listen("tcp", t.self.ToString())
	if err != nil {
		// TODO: Proper logging of err
		return
	}
	go t.connectHandler(ctx)
	go t.listenerHandler(ctx, listener)
}

// connectHandler handles requests from the upward layers for new Connections.
// actually this was just made like this, so I can pass it the ctx.Context
// in a functional manner.
func (t *TCPLayer) connectHandler(ctx context.Context) {
	for {
		select {
		case host := <-t.connectChan:
			conn, err := net.Dial("tcp", host.ToString())
			if err != nil {
				t.outChannelEvents <- TransportConnFailed
				return
			}
			tcpConnection := NewTCPConnection(conn)
			t.outChannelEvents <- TransportConnConnected
			t.addActiveConn(tcpConnection, host)
			go t.connectionHandler(ctx, tcpConnection, host)
		}
	}
}

// listenerHandler handles new connections made to this layer's active listener.
func (t *TCPLayer) listenerHandler(ctx context.Context, listener net.Listener) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			err := listener.Close()
			if err != nil {
				// TODO: Proper logging
			}
			return
		default:
			conn, err := listener.Accept()
			if err != nil {
				// TODO: Proper logging
				continue // doesn't launch the connection handler
			}
			addr := conn.RemoteAddr().(*net.TCPAddr)
			host := NewTransportHost(addr.Port, addr.IP.String())
			tcpConnection := NewTCPConnection(conn)
			t.outChannelEvents <- TransportConnConnected
			t.addActiveConn(tcpConnection, host)
			go t.connectionHandler(ctx, tcpConnection, host)
		}
	}
}

// handleConnection handles a single tcp connection
func (t *TCPLayer) connectionHandler(ctx context.Context, conn *TCPConnection, host TransportHost) {
	for {
		select {
		case <-ctx.Done():
			return // TODO: Proper logging
		default:
			err := t.receiveMessage(conn.tcpConn, host)
			if err != nil {
				t.Disconnect(host)
				return
			}
		}
	}
}

// receiveMessage receives and processes a message and delivers it upwards
func (t *TCPLayer) receiveMessage(conn net.Conn, host TransportHost) error {
	buf := make([]byte, 4096)

	// Set a read deadline for non-blocking read
	err := conn.SetReadDeadline(time.Now().Add(time.Second * 1))
	if err != nil {
		return err // Indicates an error occurred
	}

	numberBytes, err := conn.Read(buf)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			// It's a timeout, not a disconnection; return false to indicate no fatal error occurred
			return nil
		}
		// For other errors, handle disconnection
		return err // Indicates a disconnection or fatal error occurred
	}

	// Process the received message
	t.outChannel <- TransportMessage{host, *bytes.NewBuffer(buf[:numberBytes])}
	return nil // No error occurred
}

// addActiveConn adds the active connection for the specified Host
// this function may be used simultaneously by several go routines.
func (t *TCPLayer) addActiveConn(conn *TCPConnection, host TransportHost) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.activeConnections[host] = conn
}

// removeActiveConn removes the active connection of the specified Host, if it exists
// this function may be used simultaneously by several go routines.
func (t *TCPLayer) removeActiveConn(host TransportHost) (*TCPConnection, bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	conn, ok := t.activeConnections[host]
	if ok {
		delete(t.activeConnections, host)
		return conn, true
	} else {
		return nil, false
	}
}

// getActiveConn returns the active connection, and a bool indicating if it exists, for the specified Host
// this function may be used simultaneously by several go routines.
func (t *TCPLayer) getActiveConn(host TransportHost) (*TCPConnection, bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	conn, ok := t.activeConnections[host]
	return conn, ok
}
