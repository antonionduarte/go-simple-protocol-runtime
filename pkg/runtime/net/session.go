package net

import (
	"bytes"
	"context"
)

type (
	SessionLayer struct {
		self              Host
		connectChan       chan Host
		disconnectChan    chan Host
		sendChan          chan SessionMessage
		outChannelEvents  chan SessionEvent
		outMessages       chan SessionMessage
		ongoingHandshakes map[TransportHost]chan HandshakeMessage
		transport         TransportLayer
		serverMappings    map[TransportHost]Host // mapping of Client Addr to Server Addr.
	}

	HandshakeMessage struct {
		event      TransportEvent // optional field
		sessionMsg SessionMessage // optional field
	}

	SessionMessage struct {
		host  Host // server host of each peer instead of the client host that the transport layer receives
		layer LayerIdentifier
		msg   bytes.Buffer
	}

	SessionEvent interface {
		Host() Host
	}

	SessionDisconnected struct {
		host Host
	}

	SessionFailed struct {
		host Host
	}

	SessionConnected struct {
		host Host
	}

	LayerIdentifier int
)

const (
	Application LayerIdentifier = iota
	Session
)

func (s *SessionConnected) Host() Host {
	return s.host
}

func (s *SessionDisconnected) Host() Host {
	return s.host
}

func (s *SessionFailed) Host() Host {
	return s.host
}

func NewSessionLayer(transport TransportLayer, self Host, ctx context.Context) *SessionLayer {
	session := &SessionLayer{
		self:              self,
		connectChan:       make(chan Host),
		disconnectChan:    make(chan Host),
		sendChan:          make(chan SessionMessage),
		outChannelEvents:  make(chan SessionEvent),
		outMessages:       make(chan SessionMessage),
		serverMappings:    make(map[TransportHost]Host),                  // TODO: Access to this is not controlled via Mutex
		ongoingHandshakes: make(map[TransportHost]chan HandshakeMessage), // TODO: Access to this is not controlled via Mutex
		transport:         transport,
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
		case msg := <-s.transport.OutChannel():
			s.transportMessageHandler(msg)
		case event := <-s.transport.OutTransportEvents():
			s.transportEventHandler(event)
		case host := <-s.connectChan:
			s.connectClient(host)
		case host := <-s.disconnectChan:
			s.disconnect(host)
		case msg := <-s.sendChan:
			s.send(msg.msg, msg.host)
		}
	}
}

func (s *SessionLayer) transportMessageHandler(msg TransportMessage) {
	sessionMsg := deserializeTransportMessage(msg)
	switch sessionMsg.layer {
	case Application:
		s.outMessages <- sessionMsg
	case Session:
		s.ongoingHandshakes[msg.Host] <- HandshakeMessage{sessionMsg: sessionMsg}
	}
}

func (s *SessionLayer) transportEventHandler(event TransportEvent) {
	switch e := event.(type) {
	case *TransportConnected:
		handshakeChan, ok := s.ongoingHandshakes[e.Host()]
		if ok {
			handshakeChan <- HandshakeMessage{event: e}
		}
		s.connectServer(e.host)
	case *TransportDisconnected:
		// TODO: Clean ongoing Handshakes and ServerMappings
	case *TransportFailed:
		s.ongoingHandshakes[e.Host()] <- HandshakeMessage{event: e}
		// TODO: Clean ongoing Handshakes
	}
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

func (s *SessionLayer) connectClient(host TransportHost) {
	handshakeChannel := make(chan HandshakeMessage)
	// s.ongoingHandshakes
	go s.handshakeClient(handshakeChannel, host)
}

func (s *SessionLayer) connectServer(host TransportHost) {
	handshakeChannel := make(chan HandshakeMessage)
	s.ongoingHandshakes[host] = handshakeChannel
	go s.handshakeServer(handshakeChannel, host)
}

// handshakeServer
// protocol:
// - await for session message with ServerHost from client
// - send ACK
// assume connection is established and can start sending messages
func (s *SessionLayer) handshakeServer(inChan chan HandshakeMessage, host TransportHost) {
	serverHostMsg := <-inChan // Await for ServerHost to be received from other peer
	sessionHost := DeserializeHost(serverHostMsg.sessionMsg.msg)
	ackBuffer := bytes.NewBuffer([]byte{0x01})                // Create a buffer with a single byte for ACK
	s.transport.Send(TransportMessage{Msg: *ackBuffer}, host) // Send ACK
}

// handshakeClient
// protocol:
// - connectClient via Transport to remote host
//   - await for ConnectEvent or ConnectFailed for that Host
//   - send session message with my ServerHost
//   - recv ACK
//   - assume connection is established and can start sending messages
func (s *SessionLayer) handshakeClient(inChan chan HandshakeMessage, host TransportHost) {
	sessionHost := Host{IP: host.IP, Port: host.Port}
	s.transport.Connect(host)
	reply := <-inChan           // await for reply
	switch reply.event.(type) { // proceed in case Connection was successful - otherwise return
	case *TransportConnected:
		replyMsg := serializeTransportMessage(
			SessionMessage{
				msg:   SerializeHost(s.self),
				layer: Session,
			},
		)
		s.transport.Send(replyMsg, host)
		<-inChan // await for reply
		s.serverMappings[host] = sessionHost
		s.outChannelEvents <- &SessionConnected{host: sessionHost}
	case *TransportFailed:
		return
	case *TransportDisconnected:
		return
	}
}

func deserializeTransportMessage(msg TransportMessage) SessionMessage {
	return SessionMessage{}
}

func serializeTransportMessage(msg SessionMessage) TransportMessage {
	return TransportMessage{}
}
