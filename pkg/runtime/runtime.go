package runtime

import (
	"bytes"
	"context"
	"encoding/binary"
	"log/slog"
	"sync"
	"time"

	rtconfig "github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/config"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

type Runtime struct {
	cancelFunc    func()
	msgChannel    chan Message
	timerChannel  chan Timer
	timerMutex    sync.Mutex
	wg            sync.WaitGroup
	ongoingTimers map[int]*time.Timer
	protocols     map[int]*ProtoProtocol
	networkLayer  net.TransportLayer
	sessionLayer  *net.SessionLayer

	logger *slog.Logger

	// High-level session callbacks are implemented optionally by protocols
	// via the interfaces below.
}

// SessionConnectedHandler can be implemented by a protocol that wants to be
// notified whenever a session is established with some Host.
type SessionConnectedHandler interface {
	OnSessionConnected(net.Host)
}

// SessionDisconnectedHandler can be implemented by a protocol that wants to be
// notified whenever a session is torn down with some Host.
type SessionDisconnectedHandler interface {
	OnSessionDisconnected(net.Host)
}

// Internal representation of session events delivered to each ProtoProtocol.
type sessionEventType int

const (
	sessionConnectedEvent sessionEventType = iota
	sessionDisconnectedEvent
)

type sessionEvent struct {
	kind sessionEventType
	host net.Host
}

var (
	instance *Runtime
	once     sync.Once
)

// GetRuntimeInstance returns the singleton Runtime for this process.
// There is intentionally only one Runtime per process; application code
// and protocol implementations normally do not need to call this directly
// and should instead work through higher-level APIs (e.g. ApplyConfig,
// ProtocolContext, and the helpers in this package).
func ApplyConfig(cfg *rtconfig.Config) *slog.Logger {
	if cfg == nil {
		logger := slog.Default()
		r := GetRuntimeInstance()
		r.SetLogger(logger)
		return logger
	}

	rtconfig.SetGlobalConfig(cfg)
	logger := NewLoggerFromConfig(cfg.Logging)
	slog.SetDefault(logger)

	r := GetRuntimeInstance()
	r.SetLogger(logger)
	return logger
}

func GetRuntimeInstance() *Runtime {
	once.Do(func() {
		base := slog.Default().With("component", "runtime")
		instance = &Runtime{
			msgChannel:    make(chan Message, rtconfig.RuntimeMsgTimerBuffer()),
			timerChannel:  make(chan Timer, rtconfig.RuntimeMsgTimerBuffer()),
			ongoingTimers: make(map[int]*time.Timer),
			protocols:     make(map[int]*ProtoProtocol),
			logger:        base,
		}
	})
	return instance
}

func (r *Runtime) SetLogger(logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	r.logger = logger.With("component", "runtime")
}

func (r *Runtime) Logger() *slog.Logger {
	if r.logger == nil {
		return slog.Default()
	}
	return r.logger
}

func (r *Runtime) Start() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	r.cancelFunc = cancel

	r.Logger().Info("runtime starting")

	r.startProtocols(ctx)
	r.initProtocols()
	r.startSessionEvents(ctx)

	r.wg.Add(1)
	go r.eventHandler(ctx)
}

func (r *Runtime) StartWithDuration(duration time.Duration) {
	r.Start()
	time.AfterFunc(duration, func() {
		r.Cancel()
	})
}

func (r *Runtime) RegisterNetworkLayer(networkLayer net.TransportLayer) {
	r.networkLayer = networkLayer
}

func (r *Runtime) RegisterSessionLayer(sessionLayer *net.SessionLayer) {
	r.sessionLayer = sessionLayer
}

func Connect(host net.Host) {
	runtime := GetRuntimeInstance()
	if runtime.sessionLayer == nil {
		panic("Session layer not registered")
	}
	runtime.sessionLayer.Connect(host)
}

func Disconnect(host net.Host) {
	runtime := GetRuntimeInstance()
	if runtime.sessionLayer == nil {
		panic("Session layer not registered")
	}
	runtime.sessionLayer.Disconnect(host)
}

func (r *Runtime) RegisterProtocol(protocol *ProtoProtocol) {
	r.protocols[protocol.ProtocolID()] = protocol
}

func (r *Runtime) GetProtocol(protocolID int) *ProtoProtocol {
	return r.protocols[protocolID]
}

func (r *Runtime) Cancel() {
	r.cancelFunc()
	if r.sessionLayer != nil {
		r.sessionLayer.Cancel()
	}
	if r.networkLayer != nil {
		r.networkLayer.Cancel()
	}
	// Stop and clear all timers to avoid callbacks after shutdown.
	r.timerMutex.Lock()
	for id, t := range r.ongoingTimers {
		if t != nil {
			t.Stop()
		}
		delete(r.ongoingTimers, id)
	}
	r.timerMutex.Unlock()

	r.wg.Wait()
	slog.Info("runtime stopped")
}

func (r *Runtime) eventHandler(ctx context.Context) {
	defer r.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-r.msgChannel:
			protocol := r.protocols[msg.ProtocolID()]
			protocol.MessageChannel() <- msg
		case timer := <-r.timerChannel:
			protocol := r.protocols[timer.ProtocolID()]
			protocol.TimerChannel() <- timer
		// Application-level messages from the session layer
		case sessionMsg := <-r.sessionLayer.OutMessages():
			processMessage(sessionMsg.Msg, sessionMsg.Host())
		}
	}
}

func (r *Runtime) startProtocols(ctx context.Context) {
	for _, protocol := range r.protocols {
		r.Logger().Info("starting protocol", "protocolID", protocol.ProtocolID())
		protocol.Start(ctx, &r.wg)
	}
}

func (r *Runtime) initProtocols() {
	for _, protocol := range r.protocols {
		r.Logger().Info("initializing protocol", "protocolID", protocol.ProtocolID())
		protocol.Init()
	}
}

func (r *Runtime) startSessionEvents(ctx context.Context) {
	if r.sessionLayer == nil {
		return
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-r.sessionLayer.OutChannelEvents():
				switch e := ev.(type) {
				case *net.SessionConnected:
					host := e.Host()
					for _, proto := range r.protocols {
						// Deliver the event into the protocol's own event loop
						select {
						case proto.sessionEvents <- sessionEvent{kind: sessionConnectedEvent, host: host}:
						case <-ctx.Done():
							return
						}
					}
				case *net.SessionDisconnected:
					host := e.Host()
					for _, proto := range r.protocols {
						select {
						case proto.sessionEvents <- sessionEvent{kind: sessionDisconnectedEvent, host: host}:
						case <-ctx.Done():
							return
						}
					}
				}
			}
		}
	}()
}

type senderSetter interface {
	SetSender(net.Host)
}

func processMessage(buffer bytes.Buffer, from net.Host) {
	var protocolID, messageID uint16
	if err := binary.Read(&buffer, binary.LittleEndian, &protocolID); err != nil {
		logger := GetRuntimeInstance().Logger()
		logger.Error("failed to read protocolID from message header",
			"from", from.ToString(),
			"err", err,
		)
		return
	}
	if err := binary.Read(&buffer, binary.LittleEndian, &messageID); err != nil {
		logger := GetRuntimeInstance().Logger()
		logger.Error("failed to read messageID from message header",
			"from", from.ToString(),
			"protocolID", protocolID,
			"err", err,
		)
		return
	}

	runtime := GetRuntimeInstance()
	logger := runtime.Logger()
	protocol, exists := runtime.protocols[int(protocolID)]
	if !exists {
		// Unknown protocol: log and discard the message, runtime keeps operating.
		logger.Warn("received message for unknown protocol",
			"from", from.ToString(),
			"protocolID", protocolID,
			"messageID", messageID,
		)
		return
	}

	serializer, ok := protocol.msgSerializers[int(messageID)]
	if !ok {
		// Unknown message ID for a known protocol: log and discard.
		logger.Warn("received message for unknown messageID",
			"from", from.ToString(),
			"protocolID", protocolID,
			"messageID", messageID,
		)
		return
	}

	// Remaining bytes belong to the message-specific payload.
	message, err := serializer.Deserialize(buffer.Bytes())
	if err != nil {
		logger.Error("failed to deserialize message",
			"from", from.ToString(),
			"protocolID", protocolID,
			"messageID", messageID,
			"err", err,
		)
		return
	}

	// If the concrete message type supports setting the sender,
	// inform it of the remote host that sent this message.
	if m, ok := message.(senderSetter); ok {
		m.SetSender(from)
	}

	// push the message to that protocol's channel
	logger.Debug("dispatching message",
		"from", from.ToString(),
		"protocolID", protocolID,
		"messageID", messageID,
	)
	protocol.messageChannel <- message
}
