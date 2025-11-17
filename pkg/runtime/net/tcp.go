package net

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
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

// maxFrameSize is a safety limit for a single TCP frame payload.
// Frames with a declared length larger than this are treated as protocol errors.
const maxFrameSize uint32 = 16 * 1024 * 1024 // 16MiB

// encodeFrame wraps a payload with a 4-byte big-endian length prefix.
// Frame format: [length(uint32 BE) || payload].
func encodeFrame(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		// Allow empty payloads, but still send a 0-length frame.
	}
	if len(payload) > int(^uint32(0)) {
		return nil, fmt.Errorf("payload too large: %d bytes", len(payload))
	}

	buf := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(payload)))
	copy(buf[4:], payload)
	return buf, nil
}

// decodeFrames consumes as many complete frames as possible from buf and returns
// a slice of payload byte slices. The consumed bytes are removed from buf.
// If an invalid length is found (e.g. exceeds maxFrameSize), an error is returned.
func decodeFrames(buf *bytes.Buffer) ([][]byte, error) {
	var frames [][]byte
	for {
		// Need at least 4 bytes for the length prefix.
		if buf.Len() < 4 {
			return frames, nil
		}

		header := buf.Bytes()
		length := binary.BigEndian.Uint32(header[:4])
		if length > maxFrameSize {
			return nil, fmt.Errorf("frame length %d exceeds maxFrameSize %d", length, maxFrameSize)
		}

		// Wait until the full frame is available.
		if buf.Len() < int(4+length) {
			return frames, nil
		}

		// Consume header.
		_ = buf.Next(4)

		// Consume payload.
		payload := buf.Next(int(length))
		// Make a copy to decouple from the underlying buffer.
		cpy := make([]byte, len(payload))
		copy(cpy, payload)
		frames = append(frames, cpy)
	}
}

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
	frame, err := encodeFrame(networkMessage.Msg.Bytes())
	if err != nil {
		t.logger.Error("tcp encode frame failed", "host", sendTo.ToString(), "err", err)
		t.outTransportEvents <- &TransportFailed{host: sendTo}
		t.Disconnect(sendTo)
		return
	}

	if _, err := conn.Write(frame); err != nil {
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
	readBuf := make([]byte, 4096)
	recvBuf := bytes.NewBuffer(nil)
	for {
		select {
		case <-t.ctx.Done():
			_ = conn.Close()
			return
		default:
			_ = conn.SetReadDeadline(time.Now().Add(time.Second))

			n, err := conn.Read(readBuf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					continue
				}
				t.logger.Error("tcp read error, disconnecting", "host", host.ToString(), "err", err)
				t.disconnect(host)
				return
			}
			if n == 0 {
				continue
			}

			if _, err := recvBuf.Write(readBuf[:n]); err != nil {
				t.logger.Error("tcp recv buffer write failed", "host", host.ToString(), "err", err)
				t.disconnect(host)
				return
			}

			frames, err := decodeFrames(recvBuf)
			if err != nil {
				t.logger.Error("tcp frame decode failed, disconnecting", "host", host.ToString(), "err", err)
				t.outTransportEvents <- &TransportFailed{host: host}
				t.disconnect(host)
				return
			}

			for _, frame := range frames {
				// Each frame is the payload that upper layers already understand:
				// [LayerID || ProtocolID || MessageID || Contents]
				data := make([]byte, len(frame))
				copy(data, frame)
				t.outChannel <- TransportMessage{
					Host: host,
					Msg:  *bytes.NewBuffer(data),
				}
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
