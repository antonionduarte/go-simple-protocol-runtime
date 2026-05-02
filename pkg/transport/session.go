package transport

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// SessionLayer sits between the runtime and a concrete Layer.
// It is responsible for:
//   - Performing a simple handshake to associate ephemeral transport
//     connections with stable logical Hosts.
//   - Emitting session-level events (connected / disconnected / failed).
//   - Framing application payloads with a LayerIdentifier so receivers can
//     distinguish handshake vs. application messages.
type (
	SessionLayer struct {
		self             Host                // Our own higher-level Host identity
		connectChan      chan Host           // Requests to connect come in as net.Host
		disconnectChan   chan Host           // Requests to disconnect come in as net.Host
		sendChan         chan SessionMessage // Requests to send a SessionMessage
		outChannelEvents chan SessionEvent   // Outgoing events (connected, disconnected, etc.)
		outMessages      chan SessionMessage // Outgoing messages at the session/application level

		// sessions holds per-peer session state machines, keyed by transport host.
		sessions map[Host]*sessionConn

		// transport-level details and host mappings.
		transport          Layer         // Underlying transport layer
		transportToLogical map[Host]Host // Map from transport host to logical host
		logicalToTransport map[Host]Host // Map from logical host to transport host

		ctx        context.Context
		cancelFunc context.CancelFunc

		logger *slog.Logger

		mu sync.Mutex // guards session maps and host mappings
	}

	// SessionState represents the lifecycle state of a single session with a peer.
	SessionState int

	// HandshakeType identifies the type of a session-layer handshake message.
	HandshakeType byte

	// sessionConn models a single logical session with a peer and owns the
	// handshake state machine for that peer.
	sessionConn struct {
		logicalHost   Host
		transportHost Host
		state         SessionState

		lastErr error

		layer *SessionLayer
	}

	SessionMessage struct {
		host  Host            // The real Host identity of the remote node
		layer LayerIdentifier // Session vs Application
		Msg   bytes.Buffer    // The raw data or serialized content
	}

	// SessionEvent signals connection success/failure or disconnection at the session level.
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

	// LayerIdentifier differentiates between application-level payloads and
	// session/handshake payloads carried over the same transport.
	LayerIdentifier int
)

// LayerIdentifier constants. These values are written on the wire as the first
// byte of the session payload coming from the transport layer.
const (
	Application LayerIdentifier = iota
	Session
)

// SessionState values.
const (
	SessionStateIdle SessionState = iota
	SessionStateHandshakingClient
	SessionStateHandshakingServer
	SessionStateEstablished
	SessionStateFailed
	SessionStateClosing
)

// HandshakeType values. Session-layer payloads always start with a single
// byte HandshakeType followed by type-specific data (if any).
const (
	HandshakeHello HandshakeType = iota + 1
	HandshakeAck
)

// Accessor methods to implement the SessionEvent interface:
func (s *SessionConnected) Host() Host    { return s.host }
func (s *SessionDisconnected) Host() Host { return s.host }
func (s *SessionFailed) Host() Host       { return s.host }

func NewSessionLayer(transport Layer, self Host, ctx context.Context, eventsBuf, msgsBuf int) *SessionLayer {
	ctx, cancel := context.WithCancel(ctx)
	logger := slog.Default().With("component", "session")
	if eventsBuf <= 0 {
		eventsBuf = defaultSessionEventsBuffer
	}
	if msgsBuf <= 0 {
		msgsBuf = defaultSessionMessagesBuffer
	}
	session := &SessionLayer{
		self:               self,
		connectChan:        make(chan Host),
		disconnectChan:     make(chan Host),
		sendChan:           make(chan SessionMessage),
		outChannelEvents:   make(chan SessionEvent, eventsBuf),
		outMessages:        make(chan SessionMessage, msgsBuf),
		sessions:           make(map[Host]*sessionConn),
		transport:          transport,
		transportToLogical: make(map[Host]Host),
		logicalToTransport: make(map[Host]Host),
		ctx:                ctx,
		cancelFunc:         cancel,
		logger:             logger,
	}
	go session.handler(ctx)
	return session
}

// --- sessionConn state machine methods ---

// handleClientConnectRequested is called when the local runtime requests that
// we establish a session to logicalHost. The SessionLayer is expected to have
// already set logicalHost and transportHost appropriately when constructing
// the sessionConn.
func (s *sessionConn) handleClientConnectRequested() {
	switch s.state {
	case SessionStateIdle, SessionStateFailed, SessionStateClosing:
		s.state = SessionStateHandshakingClient
		s.layer.logger.Debug("session FSM: client connect requested",
			"logical", s.logicalHost.String(),
			"transport", s.transportHost.String())
		// Initiate underlying transport connection.
		s.layer.transport.Connect(s.transportHost)
	default:
		// Connect requested in an unexpected state; log and ignore.
		s.layer.logger.Warn("session FSM: connect requested in non-idle state",
			"state", s.state,
			"logical", s.logicalHost.String(),
			"transport", s.transportHost.String())
	}
}

// handleConnected is invoked when the underlying transport connects
// for this session's transportHost.
func (s *sessionConn) handleConnected() {
	switch s.state {
	case SessionStateHandshakingClient:
		// Client side: send Hello with our logical host and consider the
		// session established from the client's perspective.
		s.layer.logger.Debug("session FSM: transport connected (client), sending Hello and marking established",
			"logical", s.logicalHost.String(),
			"transport", s.transportHost.String())
		helloPayload, err := encodeHello(s.layer.self)
		if err != nil {
			s.layer.logger.Error("session FSM: encodeHello failed",
				"transport", s.transportHost.String(),
				"err", err)
			s.lastErr = err
			s.state = SessionStateFailed
			s.layer.emitEvent(&SessionFailed{host: s.logicalHost})
			return
		}
		sessionMsg := SessionMessage{
			host:  s.transportHost,
			layer: Session,
			Msg:   helloPayload,
		}
		msg := serializeMessage(sessionMsg)
		s.layer.logger.Debug("session FSM: client sending Hello on transport",
			"transport", s.transportHost.String())
		s.layer.transport.Send(msg, s.transportHost)
		s.layer.logger.Debug("session FSM: client Hello sent",
			"transport", s.transportHost.String())

		// Mark as established on the client side and emit SessionConnected.
		s.state = SessionStateEstablished
		s.layer.setServerMapping(s.transportHost, s.logicalHost)
		s.layer.logger.Info("session FSM: client emitting SessionConnected",
			"peer", s.logicalHost.String(),
			"transport", s.transportHost.String())
		s.layer.emitEvent(&SessionConnected{host: s.logicalHost})

	case SessionStateHandshakingServer:
		// Server side: just record that the transport is up; we wait for Hello.
		s.layer.logger.Debug("session FSM: transport connected (server)",
			"transport", s.transportHost.String())

	default:
		// In other states, just log; this likely indicates a reconnect or race.
		s.layer.logger.Warn("session FSM: transport connected in unexpected state",
			"state", s.state,
			"transport", s.transportHost.String())
	}
}

// handleHandshakeMessage processes a session-layer handshake message (Hello or Ack).
func (s *sessionConn) handleHandshakeMessage(msg SessionMessage) {
	// Work on a copy so we don't mutate shared buffers.
	buf := bytes.NewBuffer(msg.Msg.Bytes())
	ht, host, err := parseHandshakePayload(buf)
	if err != nil {
		s.lastErr = err
		s.layer.logger.Error("session FSM: failed to parse handshake payload",
			"transport", s.transportHost.String(),
			"err", err)
		return
	}

	switch s.state {
	case SessionStateHandshakingServer:
		s.handleServerHandshake(ht, host)
	case SessionStateHandshakingClient:
		s.handleClientHandshake(ht, host)
	default:
		s.layer.logger.Warn("session FSM: handshake message in unexpected state",
			"state", s.state,
			"type", ht,
			"transport", s.transportHost.String())
	}
}

func (s *sessionConn) handleServerHandshake(ht HandshakeType, remote Host) {
	switch ht {
	case HandshakeHello:
		// Server received client's logical host; record mapping and send Ack.
		s.logicalHost = remote
		s.layer.logger.Debug("session FSM: server received Hello",
			"client", remote.String(),
			"transport", s.transportHost.String())

		ackPayload := encodeAck()
		sessionMsg := SessionMessage{
			host:  s.transportHost,
			layer: Session,
			Msg:   ackPayload,
		}
		msg := serializeMessage(sessionMsg)
		s.layer.logger.Debug("session FSM: server sending Ack on transport",
			"transport", s.transportHost.String())
		s.layer.transport.Send(msg, s.transportHost)
		s.layer.logger.Debug("session FSM: server Ack sent",
			"transport", s.transportHost.String())

		// Mark as established and emit event.
		s.state = SessionStateEstablished
		s.layer.setServerMapping(s.transportHost, s.logicalHost)
		s.layer.logger.Debug("session FSM: server handshake complete",
			"client", s.logicalHost.String(),
			"transport", s.transportHost.String())
		s.layer.logger.Info("session FSM: server emitting SessionConnected",
			"peer", s.logicalHost.String(),
			"transport", s.transportHost.String())
		s.layer.emitEvent(&SessionConnected{host: s.logicalHost})

	case HandshakeAck:
		// Server receiving Ack is unexpected.
		s.layer.logger.Warn("session FSM: server received unexpected Ack",
			"transport", s.transportHost.String())
	}
}

func (s *sessionConn) handleClientHandshake(ht HandshakeType, _ Host) {
	switch ht {
	case HandshakeAck:
		// Client receiving Ack is optional confirmation; we already treat the
		// session as established on transport connect. Just log it.
		s.layer.logger.Debug("session FSM: client received Ack",
			"server", s.logicalHost.String(),
			"transport", s.transportHost.String())

	case HandshakeHello:
		// Client receiving Hello is unexpected in the current simple protocol.
		s.layer.logger.Warn("session FSM: client received unexpected Hello",
			"transport", s.transportHost.String())
	}
}

// handleDisconnected handles a disconnect at the transport level.
func (s *sessionConn) handleDisconnected() {
	s.layer.logger.Debug("session FSM: transport disconnected",
		"state", s.state,
		"logical", s.logicalHost.String(),
		"transport", s.transportHost.String())

	switch s.state {
	case SessionStateEstablished, SessionStateHandshakingClient, SessionStateHandshakingServer:
		s.state = SessionStateClosing
		logical := s.layer.mapToHost(s.transportHost)
		s.layer.cleanupServerMapping(s.transportHost)
		s.layer.logger.Info("session FSM: emitting SessionDisconnected",
			"peer", logical.String(),
			"transport", s.transportHost.String())
		s.layer.emitEvent(&SessionDisconnected{host: logical})
	default:
		// In other states, simply mark as failed.
		s.state = SessionStateFailed
	}
}

// handleFailed handles a failure at the transport level.
func (s *sessionConn) handleFailed() {
	s.layer.logger.Warn("session FSM: transport failed",
		"state", s.state,
		"logical", s.logicalHost.String(),
		"transport", s.transportHost.String())

	logical := s.layer.mapToHost(s.transportHost)
	s.layer.cleanupServerMapping(s.transportHost)
	s.state = SessionStateFailed
	s.layer.logger.Info("session FSM: emitting SessionFailed",
		"peer", logical.String(),
		"transport", s.transportHost.String())
	s.layer.emitEvent(&SessionFailed{host: logical})
}

// encodeHello builds a session-layer handshake payload announcing the
// sender's logical Host. Layout:
//
//	[HandshakeType(1 byte, HandshakeHello) || WriteHost(self)]
//
// Returns an error if the host cannot be serialized.
func encodeHello(h Host) (bytes.Buffer, error) {
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(byte(HandshakeHello))
	if err := WriteHost(buf, h); err != nil {
		return bytes.Buffer{}, fmt.Errorf("encodeHello: %w", err)
	}
	return *buf, nil
}

// encodeAck builds a session-layer handshake ACK payload with no extra
// data: [HandshakeType(1 byte, HandshakeAck)].
func encodeAck() bytes.Buffer {
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(byte(HandshakeAck))
	return *buf
}

// parseHandshakePayload interprets a session-layer handshake payload into
// a HandshakeType and optional Host (for Hello messages). Layout:
//
//	[HandshakeType(1 byte) || Data...]
//
// where Data is wire.WriteHost(host) for HandshakeHello and empty for HandshakeAck.
func parseHandshakePayload(buf *bytes.Buffer) (HandshakeType, Host, error) {
	var zeroHost Host
	if buf.Len() == 0 {
		return 0, zeroHost, fmt.Errorf("handshake payload empty")
	}

	msgType, err := buf.ReadByte()
	if err != nil {
		return 0, zeroHost, fmt.Errorf("failed to read handshake type: %w", err)
	}

	switch HandshakeType(msgType) {
	case HandshakeHello:
		host, err := ReadHost(buf)
		if err != nil {
			return 0, zeroHost, fmt.Errorf("HandshakeHello: %w", err)
		}
		return HandshakeHello, host, nil
	case HandshakeAck:
		return HandshakeAck, zeroHost, nil
	default:
		return 0, zeroHost, fmt.Errorf("unknown handshake type: %d", msgType)
	}
}

// Cancel stops the internal goroutine(s) by cancelling their context.
func (s *SessionLayer) Cancel() {
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
}

// emitEvent pushes a session event onto outChannelEvents, but bails out if the
// session context has been cancelled. This prevents the FSM goroutine from
// blocking forever during shutdown when no consumer is reading.
func (s *SessionLayer) emitEvent(ev SessionEvent) {
	select {
	case s.outChannelEvents <- ev:
	case <-s.ctx.Done():
	}
}

func (s *SessionLayer) Connect(host Host) {
	s.logger.Debug("session connect requested", "host", host.String())
	select {
	case s.connectChan <- host:
	case <-s.ctx.Done():
	}
}

func (s *SessionLayer) Disconnect(host Host) {
	s.logger.Debug("session disconnect requested", "host", host.String())
	select {
	case s.disconnectChan <- host:
	case <-s.ctx.Done():
	}
}

func (s *SessionLayer) Send(msg bytes.Buffer, sendTo Host) {
	s.logger.Debug("session send requested", "to", sendTo.String(), "bytes", msg.Len())
	select {
	case s.sendChan <- SessionMessage{Msg: msg, host: sendTo, layer: Application}:
	case <-s.ctx.Done():
	}
}

func (s *SessionLayer) OutChannelEvents() chan SessionEvent {
	return s.outChannelEvents
}

func (s *SessionLayer) OutMessages() chan SessionMessage {
	return s.outMessages
}

// Host returns the logical host associated with this session message.
func (m SessionMessage) Host() Host {
	return m.host
}

func (s *SessionLayer) handler(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case msg := <-s.transport.OutChannel():
			s.transportMessageHandler(msg)

		case event := <-s.transport.OutEvents():
			s.transportEventHandler(event)

		case host := <-s.connectChan:
			s.connectClient(host)

		case host := <-s.disconnectChan:
			s.disconnect(host)

		case msg := <-s.sendChan:
			s.send(msg.Msg, msg.host)
		}
	}
}

func (s *SessionLayer) transportMessageHandler(msg Message) {
	sessionMsg := deserializeMessage(msg)
	switch sessionMsg.layer {
	case Application:
		s.logger.Debug("session application message received",
			"from", sessionMsg.host.String(),
			"bytes", sessionMsg.Msg.Len())
		select {
		case s.outMessages <- sessionMsg:
		case <-s.ctx.Done():
		}

	case Session:
		s.logger.Debug("session handshake message received", "from", msg.Host.String(), "bytes", sessionMsg.Msg.Len())
		s.dispatchHandshakeMessage(msg.Host, sessionMsg)

	default:
		s.logger.Warn("session message with unknown layer identifier dropped",
			"from", msg.Host.String(),
			"layer", sessionMsg.layer,
			"bytes", sessionMsg.Msg.Len(),
		)
	}
}

func (s *SessionLayer) transportEventHandler(event Event) {
	switch e := event.(type) {
	case *Connected:
		s.logger.Info("session inbound transport connected", "host", e.host.String())
		s.dispatchConnected(e.host)

	case *Disconnected:
		s.dispatchDisconnected(e.host)

	case *Failed:
		s.dispatchFailed(e.Host())
	}
}

func (s *SessionLayer) connectClient(h Host) {
	s.logger.Info("session initiating client handshake", "to", h.String())
	s.withSession(h, h, func(sc *sessionConn) {
		sc.handleClientConnectRequested()
	})
}

func (s *SessionLayer) disconnect(h Host) {
	s.transport.Disconnect(h)
	s.cleanupServerMapping(h)
}

func (s *SessionLayer) send(msg bytes.Buffer, sendTo Host) {
	sessionMsg := SessionMessage{
		host:  sendTo,
		layer: Application,
		Msg:   msg,
	}

	underlyingHost := s.resolveUnderlyingHost(sendTo)

	transportMsg := serializeMessage(sessionMsg)
	s.transport.Send(transportMsg, underlyingHost)
}

// withSession looks up or creates a sessionConn for the given transport host.
// If logicalHost is non-zero (Port != 0), it is recorded as the desired
// logical host for this session.
func (s *SessionLayer) withSession(transportHost Host, logicalHost Host, fn func(sc *sessionConn)) {
	s.mu.Lock()
	sc, ok := s.sessions[transportHost]
	if !ok {
		sc = &sessionConn{
			logicalHost:   logicalHost,
			transportHost: transportHost,
			state:         SessionStateIdle,
			layer:         s,
		}
		s.sessions[transportHost] = sc
	} else if logicalHost.Port != 0 {
		// Update logical host if provided.
		sc.logicalHost = logicalHost
	}
	s.mu.Unlock()

	fn(sc)
}

// dispatchHandshakeMessage routes a session-layer handshake message into the
// appropriate sessionConn state machine.
func (s *SessionLayer) dispatchHandshakeMessage(transportHost Host, msg SessionMessage) {
	s.withSession(transportHost, Host{}, func(sc *sessionConn) {
		// If this is an inbound connection on the server side and we haven't
		// yet entered a state, mark as server handshaking.
		if sc.state == SessionStateIdle {
			sc.state = SessionStateHandshakingServer
		}
		sc.handleHandshakeMessage(msg)
	})
}

func (s *SessionLayer) dispatchConnected(transportHost Host) {
	s.withSession(transportHost, Host{}, func(sc *sessionConn) {
		// If this session was created by an inbound connection and hasn't
		// been marked yet, treat it as server-handshaking.
		if sc.state == SessionStateIdle {
			sc.state = SessionStateHandshakingServer
		}
		sc.handleConnected()
	})
}

func (s *SessionLayer) dispatchDisconnected(transportHost Host) {
	s.withSession(transportHost, Host{}, func(sc *sessionConn) {
		sc.handleDisconnected()
	})
}

func (s *SessionLayer) dispatchFailed(transportHost Host) {
	s.withSession(transportHost, Host{}, func(sc *sessionConn) {
		sc.handleFailed()
	})
}

func (s *SessionLayer) setServerMapping(transport Host, logical Host) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transportToLogical[transport] = logical
	s.logicalToTransport[logical] = transport
}

func (s *SessionLayer) cleanupServerMapping(h Host) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if logical, ok := s.transportToLogical[h]; ok {
		delete(s.transportToLogical, h)
		delete(s.logicalToTransport, logical)
	}
}

// resolveUnderlyingHost returns the transport host associated with the given
// logical host, falling back to the logical host itself if no mapping exists.
func (s *SessionLayer) resolveUnderlyingHost(sendTo Host) Host {
	s.mu.Lock()
	defer s.mu.Unlock()

	if tHost, ok := s.logicalToTransport[sendTo]; ok {
		return tHost
	}
	return sendTo
}

// mapToHost returns the logical host associated with a transport host if
// present, or a fallback based on legacy mappings.
func (s *SessionLayer) mapToHost(h Host) Host {
	s.mu.Lock()
	defer s.mu.Unlock()

	if logical, ok := s.transportToLogical[h]; ok {
		return logical
	}
	return h
}

func deserializeMessage(msg Message) SessionMessage {
	buffer := msg.Msg

	if buffer.Len() == 0 {
		return SessionMessage{
			host:  msg.Host,
			layer: Application,
			Msg:   buffer,
		}
	}

	header := buffer.Next(1)
	layer := LayerIdentifier(header[0])

	return SessionMessage{
		host:  msg.Host,
		layer: layer,
		Msg:   buffer, // remaining bytes: for Application, [ProtocolID || MessageID || Contents]
	}
}

func serializeMessage(msg SessionMessage) Message {
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(byte(msg.layer))
	buf.Write(msg.Msg.Bytes())

	return NewMessage(*buf, msg.host)
}
