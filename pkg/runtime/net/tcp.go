package net

import (
	"bytes"
	"net"
)

type (
	TCPLayer struct {
		ReceiveChannel    chan NetworkMessage
		ChannelEvents     chan ConnEvents
		ActiveConnections map[*Host]net.Conn
	}
)

func NewTCPLayer(self Host) *TCPLayer {
	tcpLayer := &TCPLayer{
		ReceiveChannel:    make(chan NetworkMessage), // These will be sent to the TCP layer, so it sends.
		ChannelEvents:     make(chan ConnEvents),     // These will be sent to the upper layer (probably the protocol)
		ActiveConnections: make(map[*Host]net.Conn),  // These are the active connections.
	}

	tcpLayer.start(self)

	return tcpLayer
}

func (t *TCPLayer) start(self Host) {
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
		t.ActiveConnections[host] = conn
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
		t.ReceiveChannel <- NetworkMessage{host, byteBuffer}
	}
}

func (t *TCPLayer) eventHandler() {
	for {
		select {
		case msg := <-t.ReceiveChannel:
			print(msg)
		case event := <-t.ChannelEvents:
			print(event)
		}
	}
}

func (t *TCPLayer) Send(msg bytes.Buffer, host *Host) {
	conn := t.ActiveConnections[host]

	_, err := conn.Write(msg.Bytes()) // TODO: Replace with actual message.

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
	t.ActiveConnections[host] = conn
}

func (t *TCPLayer) Disconnect(host *Host) {
	conn := t.ActiveConnections[host]
	err := conn.Close()
	if err != nil {
		// TODO: Replace with decent logger event.
		print(err)
	}
	delete(t.ActiveConnections, host)
}
