package net

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
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

		ongoingHandshakes map[Host]chan HandshakeMessage // Handshake channels per connection
		transport         TransportLayer                 // Underlying transport layer
		serverMappings    map[Host]Host                  // Map from the client-facing Host to the real Host

		ctx        context.Context
		cancelFunc context.CancelFunc

		mu sync.Mutex // guards ongoingHandshakes and serverMappings
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
		host  Host            // The real Host identity of the remote node
		layer LayerIdentifier // Session vs Application
		Msg   bytes.Buffer    // The raw data or serialized content
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
	ctx, cancel := context.WithCancel(ctx)
	session := &SessionLayer{
		self:              self,
		connectChan:       make(chan Host),
		disconnectChan:    make(chan Host),
		sendChan:          make(chan SessionMessage),
		outChannelEvents:  make(chan SessionEvent),
		outMessages:       make(chan SessionMessage),
		serverMappings:    make(map[Host]Host),
		ongoingHandshakes: make(map[Host]chan HandshakeMessage),
		transport:         transport,
		ctx:               ctx,
		cancelFunc:        cancel,
	}
	go session.handler(ctx)
	return session
}

// Cancel stops the internal goroutine(s) by cancelling their context.
func (s *SessionLayer) Cancel() {
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
}

/* -----------------------------------------------------------------------
   Public Methods
   ----------------------------------------------------------------------- */

// Connect asks the session layer to establish a session with Host.
func (s *SessionLayer) Connect(host Host) {
	slog.Debug("session connect requested", "host", host.ToString())
	s.connectChan <- host
}

// Disconnect asks the session layer to tear down a session with Host.
func (s *SessionLayer) Disconnect(host Host) {
	slog.Debug("session disconnect requested", "host", host.ToString())
	s.disconnectChan <- host
}

// Send asks the session layer to deliver a message buffer to Host.
func (s *SessionLayer) Send(msg bytes.Buffer, sendTo Host) {
	slog.Debug("session send requested", "to", sendTo.ToString(), "bytes", msg.Len())
	s.sendChan <- SessionMessage{Msg: msg, host: sendTo, layer: Application}
}

// OutChannelEvents returns a channel of session-level events (Connected, Disconnected, etc.)
func (s *SessionLayer) OutChannelEvents() chan SessionEvent {
	return s.outChannelEvents
}

// OutMessages returns a channel of session-level messages (after handshake).
func (s *SessionLayer) OutMessages() chan SessionMessage {
	return s.outMessages
}

// Host returns the logical host associated with this session message.
func (m SessionMessage) Host() Host {
	return m.host
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
			s.send(msg.Msg, msg.host)
		}
	}
}

/* -----------------------------------------------------------------------
   Transport Message / Event Handlers
   ----------------------------------------------------------------------- */

// transportMessageHandler is called when a raw TransportMessage arrives from the network.
func (s *SessionLayer) transportMessageHandler(msg TransportMessage) {
	// Attempt to parse it as a SessionMessage, based on the LayerID header
	sessionMsg := deserializeTransportMessage(msg)
	switch sessionMsg.layer {
	case Application:
		// This is an application-level message from a remote node
		slog.Debug("session application message received", "from", sessionMsg.host.ToString(), "bytes", sessionMsg.Msg.Len())
		s.outMessages <- sessionMsg

	case Session:
		// This is a handshake-level message for an ongoing handshake
		slog.Debug("session handshake message received", "from", msg.Host.ToString(), "bytes", sessionMsg.Msg.Len())
		s.mu.Lock()
		ch, ok := s.ongoingHandshakes[msg.Host]
		s.mu.Unlock()
		if ok {
			ch <- HandshakeMessage{sessionMsg: sessionMsg}
		}
	}
}

// transportEventHandler is called when a transport-level event occurs: connected, disconnected, etc.
func (s *SessionLayer) transportEventHandler(event TransportEvent) {
	switch e := event.(type) {
	case *TransportConnected:
		// If we already have a handshake channel for this host, we must have
		// initiated the connection -> notify the client-side handshake.
		if ch, ok := s.ongoingHandshakes[e.host]; ok {
			ch <- HandshakeMessage{event: e}
		} else {
			// Otherwise, this is an incoming connection -> act as server side.
			slog.Info("session inbound transport connected", "host", e.host.ToString())
			s.connectServer(e.host)
		}

	case *TransportDisconnected:
		// Map the underlying transport host to the logical Host *before*
		// cleaning up the serverMappings; otherwise we might lose the
		// logical identity and only see the ephemeral transport endpoint.
		h := s.mapToHost(e.host)
		// Clean up handshake channels or serverMappings as needed
		_ = s.cleanupOngoingHandshake(e.host)
		_ = s.cleanupServerMapping(e.host)
		slog.Info("session disconnected", "host", h.ToString())
		s.outChannelEvents <- &SessionDisconnected{host: h}

	case *TransportFailed:
		// If there's a handshake in progress, we notify it
		s.mu.Lock()
		if ch, ok := s.ongoingHandshakes[e.Host()]; ok {
			// copy pointer under lock but send outside the lock
			go func(ch chan HandshakeMessage, ev TransportEvent) {
				ch <- HandshakeMessage{event: ev}
			}(ch, e)
		}
		s.mu.Unlock()
		h := s.mapToHost(e.Host())
		slog.Warn("session transport failed", "host", h.ToString())
		s.outChannelEvents <- &SessionFailed{host: h}
	}
}

/* -----------------------------------------------------------------------
   Connect / Disconnect / Send
   ----------------------------------------------------------------------- */

// connectClient is called when we want to initiate a session to a remote Host
func (s *SessionLayer) connectClient(h Host) {
	// Create a channel for handshake steps
	handshakeChannel := make(chan HandshakeMessage)
	s.mu.Lock()
	s.ongoingHandshakes[h] = handshakeChannel
	s.mu.Unlock()

	// Initiate the actual transport connection
	slog.Info("session initiating client handshake", "to", h.ToString())
	s.transport.Connect(h)

	// Launch the handshake logic in a goroutine
	go s.handshakeClient(handshakeChannel, h)
}

// connectServer is called when we see that the transport has accepted a new connection
// from a remote node (acting as "server" side of handshake).
func (s *SessionLayer) connectServer(h Host) {
	handshakeChannel := make(chan HandshakeMessage)
	s.mu.Lock()
	s.ongoingHandshakes[h] = handshakeChannel
	s.mu.Unlock()
	slog.Info("session initiating server handshake", "remote", h.ToString())
	go s.handshakeServer(handshakeChannel, h)
}

// disconnect is called when the user wants to tear down a session with a given Host
func (s *SessionLayer) disconnect(h Host) {
	s.transport.Disconnect(h)

	// optionally clean up handshake or server mapping
	s.mu.Lock()
	_ = s.cleanupOngoingHandshake(h)
	_ = s.cleanupServerMapping(h)
	s.mu.Unlock()
}

// send is called when the user wants to send an application-level message to a Host
func (s *SessionLayer) send(msg bytes.Buffer, sendTo Host) {
	// Wrap the payload as an Application-level SessionMessage so the
	// receiver can distinguish it from handshake messages.
	sessionMsg := SessionMessage{
		host:  sendTo,
		layer: Application,
		Msg:   msg,
	}

	// Find the underlying Transport host corresponding to this logical Host.
	// On the client side, sendTo will typically be the same as the
	// transport host. On the server side, the client may be behind an
	// ephemeral TCP port, so we look up the mapping learned during
	// handshake (transportHost -> logicalHost).
	underlyingHost := sendTo
	for tHost, logical := range s.serverMappings {
		if CompareHost(logical, sendTo) {
			underlyingHost = tHost
			break
		}
	}

	transportMsg := serializeTransportMessage(sessionMsg)
	s.transport.Send(transportMsg, underlyingHost)
}

/* -----------------------------------------------------------------------
   Handshake Logic
   ----------------------------------------------------------------------- */

// handshakeServer is the "server" side of the handshake protocol
func (s *SessionLayer) handshakeServer(inChan chan HandshakeMessage, h Host) {
	// Wait for the remote side to send us their 'Host'
	var serverHostMsg HandshakeMessage
	select {
	case <-s.ctx.Done():
		return
	case serverHostMsg = <-inChan: // blocking until a message arrives
	}
	sessionHost := DeserializeHost(serverHostMsg.sessionMsg.Msg)
	slog.Info("session server received client host", "client", sessionHost.ToString())

	// Send an ACK as a Session-layer message so the client can
	// recognize it and mark the handshake as successful.
	ackPayload := *bytes.NewBuffer([]byte{0x01})
	ackSession := SessionMessage{
		host:  h,
		layer: Session,
		Msg:   ackPayload,
	}
	msg := serializeTransportMessage(ackSession)
	s.transport.Send(msg, h)

	// You might store that in serverMappings to note we've established a session
	s.serverMappings[h] = sessionHost

	// Notify upper layer
	slog.Info("session server handshake complete", "client", sessionHost.ToString())
	s.outChannelEvents <- &SessionConnected{host: sessionHost}

	// We are done with this handshake channel.
	_ = s.cleanupOngoingHandshake(h)
}

// handshakeClient is the "client" side. We connect and do the "initiate handshake" steps.
func (s *SessionLayer) handshakeClient(inChan chan HandshakeMessage, h Host) {
	// 1) Wait for TransportConnected or TransportFailed
	var reply HandshakeMessage
	select {
	case <-s.ctx.Done():
		return
	case reply = <-inChan:
	}
	switch reply.event.(type) {
	case *TransportConnected:
		// 2) Send our Host
		slog.Info("session client transport connected, sending host", "remote", h.ToString(), "self", s.self.ToString())
		msg := serializeTransportMessage(SessionMessage{
			host:  h,       // remote host we're establishing a session with
			layer: Session, // handshake
			Msg:   SerializeHost(s.self),
		})
		s.transport.Send(msg, h)

		// 3) Wait for server's ACK
		var reply2 HandshakeMessage
		select {
		case <-s.ctx.Done():
			return
		case reply2 = <-inChan:
		}
		if reply2.sessionMsg.Msg.Len() > 0 && reply2.sessionMsg.Msg.Bytes()[0] == 0x01 {
			// We got an ACK, meaning handshake success
			sessionHost := h
			s.serverMappings[h] = sessionHost
			slog.Info("session client handshake complete", "server", sessionHost.ToString())
			s.outChannelEvents <- &SessionConnected{host: sessionHost}
		}

	case *TransportFailed, *TransportDisconnected:
		// ensure we clean up the handshake channel on errors
		_ = s.cleanupOngoingHandshake(h)
		return
	}
}

/* -----------------------------------------------------------------------
   Helpers: cleanup, mapping, serialization
   ----------------------------------------------------------------------- */

// cleanupOngoingHandshake removes the handshake channel for tHost
func (s *SessionLayer) cleanupOngoingHandshake(h Host) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.ongoingHandshakes[h]; ok {
		delete(s.ongoingHandshakes, h)
		return true
	}
	return false
}

// cleanupServerMapping removes the serverMapping for tHost
func (s *SessionLayer) cleanupServerMapping(h Host) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.serverMappings[h]; ok {
		delete(s.serverMappings, h)
		return true
	}
	return false
}

// mapToHost returns the Host from serverMappings if present, or a fallback
func (s *SessionLayer) mapToHost(h Host) Host {
	s.mu.Lock()
	defer s.mu.Unlock()
	if mapped, ok := s.serverMappings[h]; ok {
		return mapped
	}
	// fallback: same IP/Port as input
	return h
}

/* -----------------------------------------------------------------------
   Serialization / Deserialization of SessionMessages
   ----------------------------------------------------------------------- */

// deserializeTransportMessage transforms a raw TransportMessage into a SessionMessage.
func deserializeTransportMessage(msg TransportMessage) SessionMessage {
	buffer := msg.Msg

	// Default to Application with empty payload if buffer is too small.
	if buffer.Len() == 0 {
		return SessionMessage{
			host:  msg.Host,
			layer: Application,
			Msg:   buffer,
		}
	}

	// First byte encodes the LayerIdentifier
	header := buffer.Next(1)
	layer := LayerIdentifier(header[0])

	return SessionMessage{
		host:  msg.Host,
		layer: layer,
		Msg:   buffer, // remaining bytes: for Application, [ProtocolID || MessageID || Contents]
	}
}

// serializeTransportMessage wraps a SessionMessage into a TransportMessage
func serializeTransportMessage(msg SessionMessage) TransportMessage {
	// Prefix the payload with the LayerIdentifier so the receiver can
	// distinguish Session vs Application messages.
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(byte(msg.layer))
	buf.Write(msg.Msg.Bytes())

	return NewTransportMessage(*buf, msg.host)
}
