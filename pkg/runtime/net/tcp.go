package net

import (
	"bytes"
	"context"
	"net"
	"sync"
	"time"
)

type (
	TCPLayer struct {
		sendChan           chan TCPSendMessage
		connectChan        chan TransportHost
		disconnectChan     chan TransportHost
		outChannel         chan TransportMessage
		outTransportEvents chan TransportEvent
		activeConnections  map[TransportHost]net.Conn
		mutex              sync.Mutex
		self               TransportHost

		cancelFunc context.CancelFunc
		ctx        context.Context
	}

	TCPSendMessage struct {
		host TransportHost
		msg  TransportMessage
	}
)

// NewTCPLayer now expects a context, since it uses goroutines
func NewTCPLayer(self TransportHost, ctx context.Context) *TCPLayer {
	// derive a cancelable context for the TCP layer itself
	ctx, cancel := context.WithCancel(ctx)
	tcpLayer := &TCPLayer{
		outChannel:         make(chan TransportMessage, 10),
		outTransportEvents: make(chan TransportEvent, 10),
		activeConnections:  make(map[TransportHost]net.Conn),
		sendChan:           make(chan TCPSendMessage),
		connectChan:        make(chan TransportHost),
		disconnectChan:     make(chan TransportHost),
		self:               self,
		ctx:                ctx,
		cancelFunc:         cancel,
	}

	tcpLayer.listen()     // start listening
	go tcpLayer.handler() // start main event loop
	return tcpLayer
}

func (t *TCPLayer) Send(msg TransportMessage, sendTo TransportHost) {
	t.sendChan <- TCPSendMessage{sendTo, msg}
}

func (t *TCPLayer) Connect(host TransportHost) {
	t.connectChan <- host
}

func (t *TCPLayer) Disconnect(host TransportHost) {
	t.disconnectChan <- host
}

func (t *TCPLayer) OutChannel() chan TransportMessage {
	return t.outChannel
}

func (t *TCPLayer) OutTransportEvents() chan TransportEvent {
	return t.outTransportEvents
}

// Cancel implements TransportLayer.Cancel()
func (t *TCPLayer) Cancel() {
	// Cancel the context so goroutines can clean up
	t.cancelFunc()
	// Optionally close channels
	close(t.sendChan)
	close(t.connectChan)
	close(t.disconnectChan)
	// You might also want to forcibly close any open connections:
	t.mutex.Lock()
	for host, conn := range t.activeConnections {
		_ = conn.Close()
		delete(t.activeConnections, host)
	}
	t.mutex.Unlock()
}

func (t *TCPLayer) send(networkMessage TransportMessage, sendTo TransportHost) {
	conn, ok := t.getActiveConn(sendTo)
	if !ok {
		// no active connection
		return
	}
	if _, err := conn.Write(networkMessage.Msg.Bytes()); err != nil {
		t.outTransportEvents <- &TransportFailed{host: sendTo}
		t.Disconnect(sendTo)
	}
}

func (t *TCPLayer) disconnect(host TransportHost) {
	conn, ok := t.removeActiveConn(host)
	if ok {
		_ = conn.Close()
		t.outTransportEvents <- &TransportDisconnected{host: host}
	}
}

func (t *TCPLayer) connect(host TransportHost) {
	conn, err := net.Dial("tcp", host.ToString())
	if err != nil {
		t.outTransportEvents <- &TransportFailed{host: host}
		return
	}
	t.outTransportEvents <- &TransportConnected{host: host}
	t.addActiveConn(conn, host)

	go t.connectionHandler(conn, host)
}

func (t *TCPLayer) handler() {
	for {
		select {
		case <-t.ctx.Done():
			return
		case host := <-t.disconnectChan:
			t.disconnect(host)
		case host := <-t.connectChan:
			t.connect(host)
		case send := <-t.sendChan:
			t.send(send.msg, send.host)
		}
	}
}

func (t *TCPLayer) listen() {
	ln, err := net.Listen("tcp", t.self.ToString())
	if err != nil {
		// log or panic
		return
	}
	go t.listenerHandler(ln)
}

func (t *TCPLayer) listenerHandler(listener net.Listener) {
	defer func() { _ = listener.Close() }()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-t.ctx.Done():
				return
			default:
				// log error or continue
				continue
			}
		}
		remote := conn.RemoteAddr().(*net.TCPAddr)
		host := NewTransportHost(remote.Port, remote.IP.String())

		t.outTransportEvents <- &TransportConnected{host: host}
		t.addActiveConn(conn, host)

		go t.connectionHandler(conn, host)
	}
}

func (t *TCPLayer) connectionHandler(conn net.Conn, host TransportHost) {
	buf := make([]byte, 4096)
	for {
		select {
		case <-t.ctx.Done():
			_ = conn.Close()
			return
		default:
			// set read deadline to avoid blocking forever
			_ = conn.SetReadDeadline(time.Now().Add(time.Second))

			n, err := conn.Read(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					// just a timeout, continue
					continue
				}
				// real error -> disconnect
				t.disconnect(host)
				return
			}
			// push upward
			t.outChannel <- TransportMessage{
				Host: host,
				Msg:  *bytes.NewBuffer(buf[:n]),
			}
		}
	}
}

// concurrency-safe
func (t *TCPLayer) addActiveConn(conn net.Conn, host TransportHost) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.activeConnections[host] = conn
}

func (t *TCPLayer) removeActiveConn(host TransportHost) (net.Conn, bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	conn, ok := t.activeConnections[host]
	if ok {
		delete(t.activeConnections, host)
	}
	return conn, ok
}

func (t *TCPLayer) getActiveConn(host TransportHost) (net.Conn, bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	conn, ok := t.activeConnections[host]
	return conn, ok
}
