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
		sendChan          chan TCPSendMessage
		connectChan       chan TransportHost
		disconnectChan    chan TransportHost
		outChannel        chan TransportMessage
		outChannelEvents  chan TransportEvent
		activeConnections map[TransportHost]net.Conn
		mutex             sync.Mutex // Mutex to protect concurrent access to activeConnections
		self              TransportHost
	}

	TCPSendMessage struct {
		host TransportHost
		msg  TransportMessage
	}
)

// NewTCPLayer creates a new TCPLayer and starts the listener
func NewTCPLayer(self TransportHost, ctx context.Context) *TCPLayer {
	tcpLayer := &TCPLayer{
		outChannel:        make(chan TransportMessage, 10),
		outChannelEvents:  make(chan TransportEvent),
		activeConnections: make(map[TransportHost]net.Conn),
		sendChan:          make(chan TCPSendMessage),
		connectChan:       make(chan TransportHost),
		disconnectChan:    make(chan TransportHost),
		self:              self,
	}
	tcpLayer.listen(ctx)     // Starting the listener in a goroutine
	go tcpLayer.handler(ctx) // Starting the handler
	return tcpLayer
}

// Send sends sessionMsg to the specified host
func (t *TCPLayer) Send(msg TransportMessage, sendTo TransportHost) {
	t.sendChan <- TCPSendMessage{sendTo, msg}
}

// Connect connects to the specified host
func (t *TCPLayer) Connect(host TransportHost) {
	t.connectChan <- host
}

// Disconnect disconnects the TCPLayer from the specified host
func (t *TCPLayer) Disconnect(host TransportHost) {
	t.disconnectChan <- host
}

// OutChannel returns the channel for outgoing messages
func (t *TCPLayer) OutChannel() chan TransportMessage {
	return t.outChannel
}

// OutTransportEvents returns the channel for outgoing events
func (t *TCPLayer) OutTransportEvents() chan TransportEvent {
	return t.outChannelEvents
}

// send sends a message to the specified host
func (t *TCPLayer) send(networkMessage TransportMessage, sendTo TransportHost) {
	conn, ok := t.getActiveConn(sendTo)
	if !ok {
		// TODO: Properly log this, trying to send message to non active connection! (propagate error? or just log?)
		return
	} else {
		_, err := conn.Write(networkMessage.Msg.Bytes())
		if err != nil {
			t.outChannelEvents <- &TransportFailed{sendTo}
			t.Disconnect(networkMessage.Host) // TODO: Replace with sendTo Host
		}
	}
}

// disconnect disconnects from the specified host
func (t *TCPLayer) disconnect(host TransportHost) {
	conn, ok := t.removeActiveConn(host)
	if ok {
		err := conn.Close()
		if err != nil {
			// TODO: Proper Logging
		}
		t.outChannelEvents <- &TransportDisconnected{host}
	} else {
		// TODO: Proper Logging
	}
}

// connect handles requests from the upward layers for new Connections.
// actually this was just made like this, so I can pass it the ctx.Context
// in a functional manner.
func (t *TCPLayer) connect(host TransportHost, ctx context.Context) {
	conn, err := net.Dial("tcp", host.ToString())
	if err != nil {
		t.outChannelEvents <- &TransportFailed{host}
		return
	}
	t.outChannelEvents <- &TransportConnected{host}
	t.addActiveConn(conn, host)
	go t.connectionHandler(ctx, conn, host)
}

// distributes messages to this layer to the appropriate functions
func (t *TCPLayer) handler(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case host := <-t.disconnectChan:
			t.disconnect(host)
		case host := <-t.connectChan:
			t.connect(host, ctx)
		case send := <-t.sendChan:
			t.send(send.msg, send.host)
		}
	}
}

// listen starts the goroutines that
// handle tcp new connections and launch the appropriate contexts
func (t *TCPLayer) listen(ctx context.Context) {
	listener, err := net.Listen("tcp", t.self.ToString())
	if err != nil {
		// TODO: Proper logging of err
		return
	}
	go t.listenerHandler(ctx, listener)
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
			host := NewTransportHost(conn.RemoteAddr().(*net.TCPAddr).Port, conn.RemoteAddr().(*net.TCPAddr).IP.String())
			t.outChannelEvents <- &TransportConnected{host}
			t.addActiveConn(conn, host)
			go t.connectionHandler(ctx, conn, host)
		}
	}
}

// handleConnection handles a single tcp connection
func (t *TCPLayer) connectionHandler(ctx context.Context, conn net.Conn, host TransportHost) {
	for {
		select {
		case <-ctx.Done():
			err := conn.Close()
			if err != nil {
				return // TODO: Proper logging
			}
			return // TODO: Proper logging
		default:
			err := t.receiveMessage(conn, host)
			if err != nil {
				t.disconnect(host)
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
func (t *TCPLayer) addActiveConn(conn net.Conn, host TransportHost) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.activeConnections[host] = conn
}

// removeActiveConn removes the active connection of the specified Host, if it exists
// this function may be used simultaneously by several go routines.
func (t *TCPLayer) removeActiveConn(host TransportHost) (net.Conn, bool) {
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
func (t *TCPLayer) getActiveConn(host TransportHost) (net.Conn, bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	conn, ok := t.activeConnections[host]
	return conn, ok
}
