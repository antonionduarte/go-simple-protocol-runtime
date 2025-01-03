package net

import (
	"bytes"
	"context"
)

// SessionLayer is intended to abstract connection handshakes at the "session" level.
type (
	SessionLayer struct {
		self             Host                // Our own higher-level Host identity
		connectChan      chan Host           // Requests to connect come in as net.Host
		disconnectChan   chan Host           // Requests to disconnect come in as net.Host
		sendChan         chan SessionMessage // Requests to send a SessionMessage
		outChannelEvents chan SessionEvent   // Outgoing events (connected, disconnected, etc.)
		outMessages      chan SessionMessage // Outgoing messages at the session/application level

		ongoingHandshakes map[TransportHost]chan HandshakeMessage // Handshake channels per connection
		transport         TransportLayer                          // Underlying transport layer
		serverMappings    map[TransportHost]Host                  // Map from the client-facing TransportHost to the real Host
	}

	// HandshakeMessage is used internally to pass around events or messages during handshake.
	HandshakeMessage struct {
		event      TransportEvent
		sessionMsg SessionMessage
	}

	// SessionMessage is the “payload” at the session layer.
	// It can be either `LayerIdentifier = Session` (for handshake) or
	// `LayerIdentifier = Application` for application-level data.
	SessionMessage struct {
		host  Host // The real Host identity of the remote node
		layer LayerIdentifier
		msg   bytes.Buffer // The raw data or serialized content
	}

	// SessionEvent signals connection success/failure or disconnection at the session level.
	SessionEvent interface {
		Host() Host
	}

	// Below are basic event types for session:
	SessionDisconnected struct {
		host Host
	}
	SessionFailed struct {
		host Host
	}
	SessionConnected struct {
		host Host
	}

	// LayerIdentifier helps differentiate handshake (session) vs. application messages
	LayerIdentifier int
)

// Some constants to identify the type of session message.
const (
	Application LayerIdentifier = iota
	Session
)

// Accessor methods to implement the SessionEvent interface:
func (s *SessionConnected) Host() Host    { return s.host }
func (s *SessionDisconnected) Host() Host { return s.host }
func (s *SessionFailed) Host() Host       { return s.host }

// NewSessionLayer constructs and starts the session layer.
func NewSessionLayer(transport TransportLayer, self Host, ctx context.Context) *SessionLayer {
	session := &SessionLayer{
		self:              self,
		connectChan:       make(chan Host),
		disconnectChan:    make(chan Host),
		sendChan:          make(chan SessionMessage),
		outChannelEvents:  make(chan SessionEvent),
		outMessages:       make(chan SessionMessage),
		serverMappings:    make(map[TransportHost]Host),
		ongoingHandshakes: make(map[TransportHost]chan HandshakeMessage),
		transport:         transport,
	}
	go session.handler(ctx)
	return session
}

/* -----------------------------------------------------------------------
   Public Methods
   ----------------------------------------------------------------------- */

// Connect asks the session layer to establish a session with Host.
func (s *SessionLayer) Connect(host Host) {
	s.connectChan <- host
}

// Disconnect asks the session layer to tear down a session with Host.
func (s *SessionLayer) Disconnect(host Host) {
	s.disconnectChan <- host
}

// Send asks the session layer to deliver a message buffer to Host.
func (s *SessionLayer) Send(msg bytes.Buffer, sendTo Host) {
	s.sendChan <- SessionMessage{msg: msg, host: sendTo, layer: Application}
}

// OutChannelEvents returns a channel of session-level events (Connected, Disconnected, etc.)
func (s *SessionLayer) OutChannelEvents() chan SessionEvent {
	return s.outChannelEvents
}

// OutMessages returns a channel of session-level messages (after handshake).
func (s *SessionLayer) OutMessages() chan SessionMessage {
	return s.outMessages
}

/* -----------------------------------------------------------------------
   Internal Handler (select loop)
   ----------------------------------------------------------------------- */

func (s *SessionLayer) handler(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		// Transport -> incoming messages
		case msg := <-s.transport.OutChannel():
			s.transportMessageHandler(msg)

		// Transport -> connection events
		case event := <-s.transport.OutTransportEvents():
			s.transportEventHandler(event)

		// Connect requests
		case host := <-s.connectChan:
			s.connectClient(host)

		// Disconnect requests
		case host := <-s.disconnectChan:
			s.disconnect(host)

		// Send requests
		case msg := <-s.sendChan:
			s.send(msg.msg, msg.host)
		}
	}
}

/* -----------------------------------------------------------------------
   Transport Message / Event Handlers
   ----------------------------------------------------------------------- */

// transportMessageHandler is called when a raw TransportMessage arrives from the network.
func (s *SessionLayer) transportMessageHandler(msg TransportMessage) {
	// Attempt to parse it as a SessionMessage
	sessionMsg := deserializeTransportMessage(msg)
	switch sessionMsg.layer {
	case Application:
		// This is an application-level message from a remote node
		s.outMessages <- sessionMsg

	case Session:
		// This is a handshake-level message for an ongoing handshake
		ch, ok := s.ongoingHandshakes[msg.Host]
		if ok {
			ch <- HandshakeMessage{sessionMsg: sessionMsg}
		}
	}
}

// transportEventHandler is called when a transport-level event occurs: connected, disconnected, etc.
func (s *SessionLayer) transportEventHandler(event TransportEvent) {
	switch e := event.(type) {
	case *TransportConnected:
		// Possibly we are a 'server' side if we didn't initiate the connect.
		// We'll set up a handshake channel if needed.
		s.connectServer(e.host)

	case *TransportDisconnected:
		// Clean up handshake channels or serverMappings as needed
		// E.g.:
		_ = s.cleanupOngoingHandshake(e.host)
		_ = s.cleanupServerMapping(e.host)
		s.outChannelEvents <- &SessionDisconnected{host: s.mapToHost(e.host)}

	case *TransportFailed:
		// If there's a handshake in progress, we notify it
		if ch, ok := s.ongoingHandshakes[e.Host()]; ok {
			ch <- HandshakeMessage{event: e}
		}
		s.outChannelEvents <- &SessionFailed{host: s.mapToHost(e.Host())}
	}
}

/* -----------------------------------------------------------------------
   Connect / Disconnect / Send
   ----------------------------------------------------------------------- */

// connectClient is called when we want to initiate a session to a remote Host
func (s *SessionLayer) connectClient(h Host) {
	// Convert net.Host -> net.TransportHost
	tHost := NewTransportHost(h.Port, h.IP)

	// Create a channel for handshake steps
	handshakeChannel := make(chan HandshakeMessage)
	s.ongoingHandshakes[tHost] = handshakeChannel

	// Initiate the actual transport connection
	s.transport.Connect(tHost)

	// Launch the handshake logic in a goroutine
	go s.handshakeClient(handshakeChannel, tHost)
}

// connectServer is called when we see that the transport has accepted a new connection
// from a remote node (acting as "server" side of handshake).
func (s *SessionLayer) connectServer(tHost TransportHost) {
	handshakeChannel := make(chan HandshakeMessage)
	s.ongoingHandshakes[tHost] = handshakeChannel
	go s.handshakeServer(handshakeChannel, tHost)
}

// disconnect is called when the user wants to tear down a session with a given Host
func (s *SessionLayer) disconnect(h Host) {
	tHost := NewTransportHost(h.Port, h.IP)
	s.transport.Disconnect(tHost)

	// optionally clean up handshake or server mapping
	_ = s.cleanupOngoingHandshake(tHost)
	_ = s.cleanupServerMapping(tHost)
}

// send is called when the user wants to send an application-level message to a Host
func (s *SessionLayer) send(msg bytes.Buffer, sendTo Host) {
	tHost := NewTransportHost(sendTo.Port, sendTo.IP)
	// At session level, we might just wrap msg in a transport message and send.

	// For example:
	transportMsg := NewTransportMessage(msg, tHost)
	s.transport.Send(transportMsg, tHost)
}

/* -----------------------------------------------------------------------
   Handshake Logic
   ----------------------------------------------------------------------- */

// handshakeServer is the "server" side of the handshake protocol
func (s *SessionLayer) handshakeServer(inChan chan HandshakeMessage, tHost TransportHost) {
	// Wait for the remote side to send us their 'Host'
	serverHostMsg := <-inChan // blocking until a message arrives
	sessionHost := DeserializeHost(serverHostMsg.sessionMsg.msg)

	// For example, we send an ACK
	ackBuffer := bytes.NewBuffer([]byte{0x01})
	msg := NewTransportMessage(*ackBuffer, tHost)
	s.transport.Send(msg, tHost)

	// You might store that in serverMappings to note we've established a session
	s.serverMappings[tHost] = sessionHost

	// Notify upper layer
	s.outChannelEvents <- &SessionConnected{host: sessionHost}
}

// handshakeClient is the "client" side. We connect and do the "initiate handshake" steps.
func (s *SessionLayer) handshakeClient(inChan chan HandshakeMessage, tHost TransportHost) {
	// 1) Wait for TransportConnected or TransportFailed
	reply := <-inChan
	switch reply.event.(type) {
	case *TransportConnected:
		// 2) Send our Host
		msg := serializeTransportMessage(SessionMessage{
			host:  s.self,  // we are the local host
			layer: Session, // handshake
			msg:   SerializeHost(s.self),
		})
		s.transport.Send(msg, tHost)

		// 3) Wait for server's ACK
		reply2 := <-inChan
		if reply2.sessionMsg.msg.Len() > 0 && reply2.sessionMsg.msg.Bytes()[0] == 0x01 {
			// We got an ACK, meaning handshake success
			sessionHost := Host{IP: tHost.IP, Port: tHost.Port}
			s.serverMappings[tHost] = sessionHost
			s.outChannelEvents <- &SessionConnected{host: sessionHost}
		}

	case *TransportFailed:
		return
	case *TransportDisconnected:
		return
	}
}

/* -----------------------------------------------------------------------
   Helpers: cleanup, mapping, serialization
   ----------------------------------------------------------------------- */

// cleanupOngoingHandshake removes the handshake channel for tHost
func (s *SessionLayer) cleanupOngoingHandshake(tHost TransportHost) bool {
	if _, ok := s.ongoingHandshakes[tHost]; ok {
		delete(s.ongoingHandshakes, tHost)
		return true
	}
	return false
}

// cleanupServerMapping removes the serverMapping for tHost
func (s *SessionLayer) cleanupServerMapping(tHost TransportHost) bool {
	if _, ok := s.serverMappings[tHost]; ok {
		delete(s.serverMappings, tHost)
		return true
	}
	return false
}

// mapToHost returns the Host from serverMappings if present, or a fallback
func (s *SessionLayer) mapToHost(tHost TransportHost) Host {
	if h, ok := s.serverMappings[tHost]; ok {
		return h
	}
	// fallback: same IP/Port as tHost
	return Host{Port: tHost.Port, IP: tHost.IP}
}

/* -----------------------------------------------------------------------
   Serialization / Deserialization of SessionMessages
   ----------------------------------------------------------------------- */

// deserializeTransportMessage transforms a raw TransportMessage into a SessionMessage.
// For now, we just do something trivial.
func deserializeTransportMessage(msg TransportMessage) SessionMessage {
	// This is user-defined. Here's a minimal example:
	sessionMsg := SessionMessage{
		// We might store 'host' if needed, or treat it as separate
		host:  Host{IP: msg.Host.IP, Port: msg.Host.Port},
		layer: Session, // might guess it's a session message
		msg:   msg.Msg, // the raw payload
	}
	return sessionMsg
}

// serializeTransportMessage wraps a SessionMessage into a TransportMessage
func serializeTransportMessage(msg SessionMessage) TransportMessage {
	// You might want to prefix with some metadata, but here's a direct approach:
	return NewTransportMessage(msg.msg, NewTransportHost(msg.host.Port, msg.host.IP))
}
