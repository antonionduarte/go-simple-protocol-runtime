package net

import (
	"bytes"
	"net"
)

type (
	TCPLayer struct {
		outChannel        chan *NetworkMessage
		outChannelEvents  chan *ConnEvents
		activeConnections map[*Host]net.Conn
	}
)

func NewTCPLayer(self *Host) *TCPLayer {
	tcpLayer := &TCPLayer{
		outChannel:        make(chan *NetworkMessage, 10), // These will be sent to the TCP layer, so it sends.
		outChannelEvents:  make(chan *ConnEvents, 1),      // These will be sent to the upper layer (probably the protocol)
		activeConnections: make(map[*Host]net.Conn),       // These are the active connections.
	}

	tcpLayer.start(self)

	return tcpLayer
}

func (t *TCPLayer) Send(networkMessage *NetworkMessage) {
	conn := t.activeConnections[networkMessage.Host]
	msgBytes := networkMessage.Msg.Bytes()
	_, err := conn.Write(msgBytes) // TODO: Replace with actual message.

	if err != nil {
		print(err)
		// TODO: Replace with decent logger event.
		// TODO: Probably means conn died, should disconnect host(?) and send event to upper layer(?)
	}
}

func (t *TCPLayer) Connect(host *Host) {
	conn, err := net.Dial("tcp", host.ToString())
	if err != nil {
		print(err)
		// TODO: Replace with decent logger event.
	}
	t.activeConnections[host] = conn
}

func (t *TCPLayer) Disconnect(host *Host) {
	conn := t.activeConnections[host]
	err := conn.Close()
	if err != nil {
		// TODO: Replace with decent logger event.
		print(err)
	}
	delete(t.activeConnections, host)
}

func (t *TCPLayer) OutChannel() chan *NetworkMessage {
	return t.outChannel
}

func (t *TCPLayer) OutChannelEvents() chan *ConnEvents {
	return t.outChannelEvents
}

func (t *TCPLayer) start(self *Host) {
	listener, err := net.Listen("tcp", self.ToString())

	if err != nil {
		// TODO: Replace with decent logger event.
		print(err)
	}

	go t.eventHandler()

	for {
		conn, err := listener.Accept()
		if err != nil {
			// TODO: Replace with decent logger event.
			print(err)
		}

		host := NewHost(0, conn.RemoteAddr().String())
		t.activeConnections[host] = conn
		go t.handleConnection(conn, host)
	}
}

func (t *TCPLayer) handleConnection(conn net.Conn, host *Host) {
	for {
		buf := make([]byte, 1024)
		_, err := conn.Read(buf)
		if err != nil {
			// TODO: Replace with decent logger event.
			print(err)
		}
		var byteBuffer bytes.Buffer
		byteBuffer.Write(buf)
		t.outChannel <- &NetworkMessage{host, &byteBuffer}
	}
}

func (t *TCPLayer) eventHandler() {
	for {
		select {
		case msg := <-t.outChannel:
			print(msg)
		case event := <-t.outChannelEvents:
			print(event)
		}
	}
}
