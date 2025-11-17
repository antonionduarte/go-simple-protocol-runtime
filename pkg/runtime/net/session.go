package net

import (
	"bytes"
	"context"
	"log/slog"
	"sync"

	rtconfig "github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/config"
)

// SessionLayer sits between the runtime and a concrete TransportLayer.
// It is responsible for:
//   - Performing connection handshakes to associate ephemeral transport
//     connections with stable logical Hosts.
//   - Emitting session-level events (connected / disconnected / failed).
//   - Framing application payloads with a LayerIdentifier so receivers can
//     distinguish handshake vs. application messages.
//
// Concurrency notes:
//   - Internal goroutines (handler, handshakeClient, handshakeServer) all
//     interact via channels.
//   - Shared maps (ongoingHandshakes, serverMappings) are ONLY accessed
//     through helper methods that acquire the mutex internally so callers
//     never need to lock around them.
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

		logger *slog.Logger

		mu sync.Mutex // guards ongoingHandshakes and serverMappings
	}

	// HandshakeMessage is used internally to pass around events or messages during handshake.
	HandshakeMessage struct {
		event      TransportEvent
		sessionMsg SessionMessage
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

func NewSessionLayer(transport TransportLayer, self Host, ctx context.Context) *SessionLayer {
	ctx, cancel := context.WithCancel(ctx)
	logger := slog.Default().With("component", "session")
	eventsBuf := rtconfig.SessionEventsBuffer()
	msgsBuf := rtconfig.SessionMessagesBuffer()
	session := &SessionLayer{
		self:              self,
		connectChan:       make(chan Host),
		disconnectChan:    make(chan Host),
		sendChan:          make(chan SessionMessage),
		outChannelEvents:  make(chan SessionEvent, eventsBuf),
		outMessages:       make(chan SessionMessage, msgsBuf),
		serverMappings:    make(map[Host]Host),
		ongoingHandshakes: make(map[Host]chan HandshakeMessage),
		transport:         transport,
		ctx:               ctx,
		cancelFunc:        cancel,
		logger:            logger,
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

func (s *SessionLayer) Connect(host Host) {
	s.logger.Debug("session connect requested", "host", host.ToString())
	s.connectChan <- host
}

func (s *SessionLayer) Disconnect(host Host) {
	s.logger.Debug("session disconnect requested", "host", host.ToString())
	s.disconnectChan <- host
}

func (s *SessionLayer) Send(msg bytes.Buffer, sendTo Host) {
	s.logger.Debug("session send requested", "to", sendTo.ToString(), "bytes", msg.Len())
	s.sendChan <- SessionMessage{Msg: msg, host: sendTo, layer: Application}
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

		case event := <-s.transport.OutTransportEvents():
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

func (s *SessionLayer) transportMessageHandler(msg TransportMessage) {
	sessionMsg := deserializeTransportMessage(msg)
	switch sessionMsg.layer {
	case Application:
		s.logger.Debug("session application message received", "from", sessionMsg.host.ToString(), "bytes", sessionMsg.Msg.Len())
		s.outMessages <- sessionMsg

	case Session:
		s.logger.Debug("session handshake message received", "from", msg.Host.ToString(), "bytes", sessionMsg.Msg.Len())
		ch, ok := s.getHandshakeChannel(msg.Host)
		if ok {
			ch <- HandshakeMessage{sessionMsg: sessionMsg}
		}
	}
}

func (s *SessionLayer) transportEventHandler(event TransportEvent) {
	switch e := event.(type) {
	case *TransportConnected:
		if ch, ok := s.getHandshakeChannel(e.host); ok {
			ch <- HandshakeMessage{event: e}
		} else {
			s.logger.Info("session inbound transport connected", "host", e.host.ToString())
			s.connectServer(e.host)
		}

	case *TransportDisconnected:
		h := s.mapToHost(e.host)
		_ = s.cleanupOngoingHandshake(e.host)
		_ = s.cleanupServerMapping(e.host)
		s.logger.Info("session disconnected", "host", h.ToString())
		s.outChannelEvents <- &SessionDisconnected{host: h}

	case *TransportFailed:
		if ch, ok := s.getHandshakeChannel(e.Host()); ok {
			go func(ch chan HandshakeMessage, ev TransportEvent) {
				ch <- HandshakeMessage{event: ev}
			}(ch, e)
		}
		h := s.mapToHost(e.Host())
		s.logger.Warn("session transport failed", "host", h.ToString())
		s.outChannelEvents <- &SessionFailed{host: h}
	}
}

func (s *SessionLayer) connectClient(h Host) {
	handshakeChannel := make(chan HandshakeMessage)
	s.setHandshakeChannel(h, handshakeChannel)

	s.logger.Info("session initiating client handshake", "to", h.ToString())
	s.transport.Connect(h)

	go s.handshakeClient(handshakeChannel, h)
}

func (s *SessionLayer) connectServer(h Host) {
	handshakeChannel := make(chan HandshakeMessage)
	s.setHandshakeChannel(h, handshakeChannel)
	s.logger.Info("session initiating server handshake", "remote", h.ToString())
	go s.handshakeServer(handshakeChannel, h)
}

func (s *SessionLayer) disconnect(h Host) {
	s.transport.Disconnect(h)

	_ = s.cleanupOngoingHandshake(h)
	_ = s.cleanupServerMapping(h)
}

func (s *SessionLayer) send(msg bytes.Buffer, sendTo Host) {
	sessionMsg := SessionMessage{
		host:  sendTo,
		layer: Application,
		Msg:   msg,
	}

	underlyingHost := s.resolveUnderlyingHost(sendTo)

	transportMsg := serializeTransportMessage(sessionMsg)
	s.transport.Send(transportMsg, underlyingHost)
}

func (s *SessionLayer) handshakeServer(inChan chan HandshakeMessage, h Host) {
	var serverHostMsg HandshakeMessage
	select {
	case <-s.ctx.Done():
		return
	case serverHostMsg = <-inChan:
	}
	sessionHost := DeserializeHost(serverHostMsg.sessionMsg.Msg)
	s.logger.Info("session server received client host", "client", sessionHost.ToString())

	ackPayload := *bytes.NewBuffer([]byte{0x01})
	ackSession := SessionMessage{
		host:  h,
		layer: Session,
		Msg:   ackPayload,
	}
	msg := serializeTransportMessage(ackSession)
	s.transport.Send(msg, h)

	s.setServerMapping(h, sessionHost)

	s.logger.Info("session server handshake complete", "client", sessionHost.ToString())
	s.outChannelEvents <- &SessionConnected{host: sessionHost}

	_ = s.cleanupOngoingHandshake(h)
}

func (s *SessionLayer) handshakeClient(inChan chan HandshakeMessage, h Host) {
	var reply HandshakeMessage
	select {
	case <-s.ctx.Done():
		return
	case reply = <-inChan:
	}
	switch reply.event.(type) {
	case *TransportConnected:
		s.logger.Info("session client transport connected, sending host", "remote", h.ToString(), "self", s.self.ToString())
		msg := serializeTransportMessage(SessionMessage{
			host:  h,
			layer: Session,
			Msg:   SerializeHost(s.self),
		})
		s.transport.Send(msg, h)

		var reply2 HandshakeMessage
		select {
		case <-s.ctx.Done():
			return
		case reply2 = <-inChan:
		}
		if reply2.sessionMsg.Msg.Len() > 0 && reply2.sessionMsg.Msg.Bytes()[0] == 0x01 {
			// We got an ACK, meaning handshake success
			sessionHost := h
			s.setServerMapping(h, sessionHost)
			s.logger.Info("session client handshake complete", "server", sessionHost.ToString())
			s.outChannelEvents <- &SessionConnected{host: sessionHost}
		}

	case *TransportFailed, *TransportDisconnected:
		_ = s.cleanupOngoingHandshake(h)
		return
	}
}

func (s *SessionLayer) setHandshakeChannel(h Host, ch chan HandshakeMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ongoingHandshakes[h] = ch
}

func (s *SessionLayer) getHandshakeChannel(h Host) (chan HandshakeMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.ongoingHandshakes[h]
	return ch, ok
}

func (s *SessionLayer) cleanupOngoingHandshake(h Host) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.ongoingHandshakes[h]; ok {
		delete(s.ongoingHandshakes, h)
		return true
	}
	return false
}

func (s *SessionLayer) setServerMapping(transport Host, logical Host) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serverMappings[transport] = logical
}

func (s *SessionLayer) cleanupServerMapping(h Host) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.serverMappings[h]; ok {
		delete(s.serverMappings, h)
		return true
	}
	return false
}

func (s *SessionLayer) resolveUnderlyingHost(sendTo Host) Host {
	s.mu.Lock()
	defer s.mu.Unlock()

	underlyingHost := sendTo
	for tHost, logical := range s.serverMappings {
		if CompareHost(logical, sendTo) {
			underlyingHost = tHost
			break
		}
	}
	return underlyingHost
}

// mapToHost returns the Host from serverMappings if present, or a fallback
func (s *SessionLayer) mapToHost(h Host) Host {
	s.mu.Lock()
	defer s.mu.Unlock()
	if mapped, ok := s.serverMappings[h]; ok {
		return mapped
	}
	return h
}

func deserializeTransportMessage(msg TransportMessage) SessionMessage {
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

func serializeTransportMessage(msg SessionMessage) TransportMessage {
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(byte(msg.layer))
	buf.Write(msg.Msg.Bytes())

	return NewTransportMessage(*buf, msg.host)
}
