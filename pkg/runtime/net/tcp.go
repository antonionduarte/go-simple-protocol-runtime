package net

import (
	"bytes"
	"context"
	"net"
	"sync"
)

type (
	TCPLayer struct {
		outChannel        chan TransportMessage
		outChannelEvents  chan ConnEvents
		connectChan       chan TransportHost // TODO: Maybe do a TransportHost and a NetworkHost?
		activeConnections map[TransportHost]net.Conn
		mutex             sync.Mutex // Mutex to protect concurrent access to activeConnections
		self              Host
	}
)

// NewTCPLayer creates a new TCPLayer and starts the listener
func NewTCPLayer(self Host, ctx context.Context) *TCPLayer {
	tcpLayer := &TCPLayer{
		outChannel:        make(chan TransportMessage, 10),
		outChannelEvents:  make(chan ConnEvents, 1),
		connectChan:       make(chan TransportHost),
		activeConnections: make(map[TransportHost]net.Conn),
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
		_, err := conn.Write(networkMessage.Msg.Bytes())
		if err != nil {
			t.outChannelEvents <- ConnFailed
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
	t.removeActiveConn(host)
	t.outChannelEvents <- ConnDisconnected
}

// OutChannel returns the channel for outgoing messages
func (t *TCPLayer) OutChannel() chan TransportMessage {
	return t.outChannel
}

// OutChannelEvents returns the channel for outgoing events
func (t *TCPLayer) OutChannelEvents() chan ConnEvents {
	return t.outChannelEvents
}

// listen starts the goroutines that:
//
// handle tcp new connections and launch the appropriate contexts
// handle requests to connect to a remote server
func (t *TCPLayer) listen(ctx context.Context) {
	listener, err := net.Listen("tcp", t.self.ToString())
	if err != nil {
		// TODO: Propper logging of err
		return
	}
	go t.connectHandler(ctx)
	go t.listenerHandler(ctx, listener)
}

// connectHandler handles requests from the upward layers for new Connections.
// actually this was just made like this so I can pass it the ctx.Context
// in a functional manner.
func (t *TCPLayer) connectHandler(ctx context.Context) {
	for {
		select {
		case host := <-t.connectChan:
			conn, err := net.Dial("tcp", host.ToString())
			if err != nil {
				t.outChannelEvents <- ConnFailed
				return
			}
			t.outChannelEvents <- ConnConnected
			t.addActiveConn(conn, host)
			go t.connectionHandler(ctx, conn, host)
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
			return
		default:
			conn, err := listener.Accept()
			if err != nil {
				// TODO: Propper logging
				continue // doesn't launch the connection handler
			}
			addr := conn.RemoteAddr().(*net.TCPAddr)
			host := NewTransportHost(addr.Port, addr.IP.String())
			go t.connectionHandler(ctx, conn, host)
		}
	}
}

// handleConnection handles a single tcp connection
// TODO: Super problematic, we *never* close this even when a connection dies!
func (t *TCPLayer) connectionHandler(ctx context.Context, conn net.Conn, host TransportHost) {
	for {
		select {
		case <-ctx.Done():
			return // TODO - propper logging?;
		default:
			t.receiveMessage(conn, host)
		}
	}
}

// receiveMessage receives and processes a message and delivers it upwards
func (t *TCPLayer) receiveMessage(conn net.Conn, host TransportHost) {
	buf := make([]byte, 4096)
	numberBytes, err := conn.Read(buf)
	if err != nil {
		t.Disconnect(host)
		return
	}
	t.outChannel <- TransportMessage{host, *bytes.NewBuffer(buf[:numberBytes])}
}

// addActiveConn adds the active connection for the specified Host
// this function may be used simultaneously by several go routines.
func (t *TCPLayer) addActiveConn(conn net.Conn, host TransportHost) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.activeConnections[host] = conn
}

// removeActiveConn removes the active connection of the specified Host, if it exists
// this function may be used simultaneously by several go routines.
func (t *TCPLayer) removeActiveConn(host TransportHost) {
	t.mutex.Lock()
	conn, ok := t.activeConnections[host]
	if ok {
		delete(t.activeConnections, host)
		err := conn.Close()
		if err != nil {
			// TODO: propper logging
		}
	}
	t.mutex.Unlock()
}

// getActiveConn returns the active connection, and a bool indicating if it exists, for the specified Host
// this function may be used simultaneously by several go routines.
func (t *TCPLayer) getActiveConn(host TransportHost) (net.Conn, bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	conn, bool := t.activeConnections[host]
	return conn, bool
}
