package net

import (
	"bytes"
	"context"
	"net"
	"sync"
)

type (
	TCPLayer struct {
		cancelFunc        func() // TODO: Should I do this? Is this idiomatic?
		outChannel        chan *NetworkMessage
		outChannelEvents  chan ConnEvents
		activeConnections map[*Host]net.Conn
		mutex             sync.Mutex // Mutex to protect concurrent access to activeConnections
	}
)

// NewTCPLayer creates a new TCPLayer and starts the listener
func NewTCPLayer(self *Host) *TCPLayer {
	tcpLayer := &TCPLayer{
		outChannel:        make(chan *NetworkMessage, 10),
		outChannelEvents:  make(chan ConnEvents, 1),
		activeConnections: make(map[*Host]net.Conn),
	}

	go tcpLayer.start(self) // Starting the listener in a goroutine

	return tcpLayer
}

func (t *TCPLayer) Cancel() {
	t.cancelFunc()
}

// Send sends a message to the specified host
func (t *TCPLayer) Send(networkMessage *NetworkMessage) {
	t.mutex.Lock()
	conn, ok := t.activeConnections[networkMessage.Host]
	t.mutex.Unlock()

	if !ok {
		t.outChannelEvents <- ConnFailed
		return
	}

	_, err := conn.Write(networkMessage.Msg.Bytes())
	if err != nil {
		t.outChannelEvents <- ConnFailed
		t.Disconnect(networkMessage.Host) // Disconnect if there's an error
	}
}

// Connect connects to the specified host
func (t *TCPLayer) Connect(host *Host) {
	conn, err := net.Dial("tcp", host.ToString())
	if err != nil {
		t.outChannelEvents <- ConnFailed
		return
	}

	t.mutex.Lock()
	t.activeConnections[host] = conn
	t.mutex.Unlock()

	t.outChannelEvents <- ConnConnected
}

// Disconnect disconnects from the specified host
func (t *TCPLayer) Disconnect(host *Host) {
	t.mutex.Lock()
	conn, ok := t.activeConnections[host]
	if ok {
		delete(t.activeConnections, host)
	}
	t.mutex.Unlock()

	if ok {
		err := conn.Close()
		if err != nil {
			// TODO: Additional logging can be done here if necessary
		}
		t.outChannelEvents <- ConnDisconnected
	}
}

// OutChannel returns the channel for outgoing messages
func (t *TCPLayer) OutChannel() chan *NetworkMessage {
	return t.outChannel
}

// OutChannelEvents returns the channel for outgoing events
func (t *TCPLayer) OutChannelEvents() chan ConnEvents {
	return t.outChannelEvents
}

// start starts the listener
func (t *TCPLayer) start(self *Host) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	t.cancelFunc = cancel

	listener, err := net.Listen("tcp", self.ToString())
	if err != nil {
		// TODO: Handle error
		return
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			// TODO: Handle error
			continue
		}

		// Parsing the actual port when creating a new Host
		addr := conn.RemoteAddr().(*net.TCPAddr)
		host := NewHost(addr.Port, addr.IP.String())

		t.mutex.Lock()
		t.activeConnections[host] = conn
		t.mutex.Unlock()

		t.outChannelEvents <- ConnConnected // Connection established

		go t.handleConnection(ctx, conn, host)
	}
}

// handleConnection handles a single tcp connection
func (t *TCPLayer) handleConnection(ctx context.Context, conn net.Conn, host *Host) {
	buf := make([]byte, 1024)

	for {
		//  TODO: I'm not sure if this works test later.
		select {
		case <-ctx.Done():
			return
		default:
			_, err := conn.Read(buf)
			if err != nil {
				t.Disconnect(host) // Disconnect on error and handle event
				return
			}

			var byteBuffer bytes.Buffer
			byteBuffer.Write(buf)
			t.outChannel <- &NetworkMessage{host, &byteBuffer}
		}
	}
}
