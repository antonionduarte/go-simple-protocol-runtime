package net

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"sync"
	"time"

	rtconfig "github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/config"
)

type (
	TCPLayer struct {
		sendChan           chan TCPSendMessage
		connectChan        chan Host
		disconnectChan     chan Host
		outChannel         chan TransportMessage
		outTransportEvents chan TransportEvent
		activeConnections  map[Host]net.Conn
		mutex              sync.Mutex
		self               Host

		cancelFunc context.CancelFunc
		ctx        context.Context

		logger *slog.Logger
	}

	TCPSendMessage struct {
		host Host
		msg  TransportMessage
	}
)

func NewTCPLayer(self Host, ctx context.Context) *TCPLayer {
	ctx, cancel := context.WithCancel(ctx)
	logger := slog.Default().With("component", "transport", "transport", "tcp")
	outBuf := rtconfig.TransportOutBuffer()

	tcpLayer := &TCPLayer{
		outChannel:         make(chan TransportMessage, outBuf),
		outTransportEvents: make(chan TransportEvent, outBuf),
		activeConnections:  make(map[Host]net.Conn),
		sendChan:           make(chan TCPSendMessage),
		connectChan:        make(chan Host),
		disconnectChan:     make(chan Host),
		self:               self,
		ctx:                ctx,
		cancelFunc:         cancel,
		logger:             logger,
	}

	logger.Info("tcp layer starting", "self", self.ToString())
	tcpLayer.listen()     // start listening
	go tcpLayer.handler() // start main event loop
	return tcpLayer
}

func (t *TCPLayer) Send(msg TransportMessage, sendTo Host) {
	t.logger.Debug("tcp send requested", "to", sendTo.ToString(), "bytes", msg.Msg.Len())
	t.sendChan <- TCPSendMessage{sendTo, msg}
}

func (t *TCPLayer) Connect(host Host) {
	t.logger.Info("tcp connect requested", "to", host.ToString())
	t.connectChan <- host
}

func (t *TCPLayer) Disconnect(host Host) {
	t.logger.Info("tcp disconnect requested", "host", host.ToString())
	t.disconnectChan <- host
}

func (t *TCPLayer) OutChannel() chan TransportMessage {
	return t.outChannel
}

func (t *TCPLayer) OutTransportEvents() chan TransportEvent {
	return t.outTransportEvents
}

func (t *TCPLayer) Cancel() {
	t.cancelFunc()
	close(t.sendChan)
	close(t.connectChan)
	close(t.disconnectChan)
	t.mutex.Lock()
	for host, conn := range t.activeConnections {
		_ = conn.Close()
		delete(t.activeConnections, host)
	}
	t.mutex.Unlock()
}

func (t *TCPLayer) send(networkMessage TransportMessage, sendTo Host) {
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

func (t *TCPLayer) disconnect(host Host) {
	conn, ok := t.removeActiveConn(host)
	if ok {
		_ = conn.Close()
		t.outTransportEvents <- &TransportDisconnected{host: host}
	}
}

func (t *TCPLayer) connect(host Host) {
	conn, err := net.Dial("tcp", (&host).ToString())
	if err != nil {
		t.logger.Error("tcp connect failed", "host", host.ToString(), "err", err)
		t.outTransportEvents <- &TransportFailed{host: host}
		return
	}
	t.logger.Info("tcp connect established", "host", host.ToString())
	t.addActiveConn(conn, host)
	t.outTransportEvents <- &TransportConnected{host: host}

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
		t.logger.Error("tcp listen failed", "self", t.self.ToString(), "err", err)
		return
	}
	t.logger.Info("tcp listening", "self", t.self.ToString())
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
		host := NewHost(remote.Port, remote.IP.String())

		slog.Info("tcp inbound connection accepted", "remote", host.ToString())

		t.addActiveConn(conn, host)
		t.outTransportEvents <- &TransportConnected{host: host}

		go t.connectionHandler(conn, host)
	}
}

func (t *TCPLayer) connectionHandler(conn net.Conn, host Host) {
	buf := make([]byte, 4096)
	for {
		select {
		case <-t.ctx.Done():
			_ = conn.Close()
			return
		default:
			_ = conn.SetReadDeadline(time.Now().Add(time.Second))

			n, err := conn.Read(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					continue
				}
				t.logger.Error("tcp read error, disconnecting", "host", host.ToString(), "err", err)
				t.disconnect(host)
				return
			}
			data := make([]byte, n)
			copy(data, buf[:n])
			t.outChannel <- TransportMessage{
				Host: host,
				Msg:  *bytes.NewBuffer(data),
			}
		}
	}
}

// concurrency-safe
func (t *TCPLayer) addActiveConn(conn net.Conn, host Host) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.activeConnections[host] = conn
}

func (t *TCPLayer) removeActiveConn(host Host) (net.Conn, bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	conn, ok := t.activeConnections[host]
	if ok {
		delete(t.activeConnections, host)
	}
	return conn, ok
}

func (t *TCPLayer) getActiveConn(host Host) (net.Conn, bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	conn, ok := t.activeConnections[host]
	return conn, ok
}

// activeConnectionCount returns the number of active connections in a
// concurrency-safe way. Intended for use in tests.
func (t *TCPLayer) activeConnectionCount() int {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return len(t.activeConnections)
}
