package net

import (
	"bytes"
	"context"
)

type (
	SessionLayer struct {
		connectChan      chan Host
		disconnectChan   chan Host
		sendChan         chan SessionMessage
		transport        TransportLayer
		serverMappings   map[TransportHost]Host // mapping of Client Addr to Server Addr.
		outChannelEvents chan TransportConnEvents
		outMessages      chan SessionMessage
	}

	SessionMessage struct {
		host Host // server host of each peer instead of the client host that the transport layer receives
		msg  bytes.Buffer
	}

	ConnEvents int
)

const (
	ConnConnected ConnEvents = iota
	ConnDisconnected
	ConnFailed
)

func NewSessionLayer(transport TransportLayer, ctx context.Context) *SessionLayer {
	session := &SessionLayer{
		connectChan:      make(chan Host),
		disconnectChan:   make(chan Host),
		sendChan:         make(chan SessionMessage),
		outChannelEvents: make(chan TransportConnEvents),
		outMessages:      make(chan SessionMessage),
		serverMappings:   make(map[TransportHost]Host),
		transport:        transport,
	}
	go session.handler(ctx) // TODO: Rethink this
	return session
}

func (s *SessionLayer) Connect(host Host) {
	s.connectChan <- host
}

func (s *SessionLayer) Disconnect(host Host) {
	s.disconnectChan <- host
}

func (s *SessionLayer) Send(msg bytes.Buffer, sendTo Host) {
	s.sendChan <- SessionMessage{msg: msg, host: sendTo}
}

func (s *SessionLayer) handler(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case host := <-s.connectChan:
			s.connect(host)
		case host := <-s.disconnectChan:
			s.disconnect(host)
		case msg := <-s.sendChan:
			s.send(msg.msg, msg.host)
		}
	}
}

/*
Problems (off the top of my head):
	-	When someone connects to us instead of the opposite,
		we also need to propagate that info, with the respective ServerHost
		to this layer.

	-	All the ConnEvents (both the ones we propagate up and the ones we receive)
		from the lower layer, need to be richer - contain the TransportHost that they involve.
*/

func (s *SessionLayer) connect(host Host) {
	// this requires an entire protocol...
}

func (s *SessionLayer) handshake() {
	// protocol:
	// - connect via TCP to remote host
	//	- send session message asking for ServerHost
	//	- recv session message with reply about ServerHost
	//	- save ServerHost in TransportHost->ServerHost and ServerHost->TransportHost map
	//	- send ConnConnected event to the up layers
}

func (s *SessionLayer) disconnect(host Host) {
	// s.transport.Disconnect() TODO: Gotta think about which host to use here
	// - attempt to get TransportHost from ServerHost->TransportHost map
	// - disconnect from that TransportHost via the TCPLayer
}

func (s *SessionLayer) send(msg bytes.Buffer, sendTo Host) {
	// s.transport.Send() TODO: Gotta think about which host to use here
	// - attempt to get TransportHost from ServerHost->TransportHost map
	// - send message to that TransportHost via the TCPLayer
}
