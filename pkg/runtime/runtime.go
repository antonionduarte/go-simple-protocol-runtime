package runtime

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"

	log "github.com/sirupsen/logrus"
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
}

var (
	instance *Runtime
	once     sync.Once
)

// GetRuntimeInstance creates or returns the singleton instance.
func GetRuntimeInstance() *Runtime {
	once.Do(func() {
		instance = &Runtime{
			msgChannel:   make(chan Message, 1),
			timerChannel: make(chan Timer, 1),
			protocols:    make(map[int]*ProtoProtocol),
		}
	})
	return instance
}

// Start starts the instance.
func (r *Runtime) Start() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	r.cancelFunc = cancel

	slog.Info("runtime starting")

	r.startProtocols(ctx)
	r.initProtocols()

	r.wg.Add(1)
	go r.eventHandler(ctx)
}

// StartWithDuration starts the runtime instance, and runs it until a specified amount of time has passed.
func (r *Runtime) StartWithDuration(duration time.Duration) {
	r.Start()
	time.AfterFunc(duration, func() {
		r.Cancel()
	})
}

// RegisterNetworkLayer registers the network layer that this runtime will use.
func (r *Runtime) RegisterNetworkLayer(networkLayer net.TransportLayer) {
	r.networkLayer = networkLayer
}

// RegisterSessionLayer registers the session layer that this runtime will use.
func (r *Runtime) RegisterSessionLayer(sessionLayer *net.SessionLayer) {
	r.sessionLayer = sessionLayer
}

// RegisterMessageHandler registers a message handler for a given protocol & message ID.
func RegisterMessageHandler(protocolID int, messageID int, handler func(Message)) {
	runtime := GetRuntimeInstance()
	proto := runtime.protocols[protocolID]
	if proto == nil {
		return // or panic/log an error
	}
	proto.RegisterMessageHandler(messageID, handler)
}

// RegisterMessageSerializer registers a message serializer for a given protocol & message ID.
func RegisterMessageSerializer(protocolID int, messageID int, serializer Serializer) {
	runtime := GetRuntimeInstance()
	proto := runtime.protocols[protocolID]
	if proto == nil {
		return // or panic/log an error
	}
	proto.RegisterMessageSerializer(messageID, serializer)
}

// RegisterProtocol registers a protocol to the runtime.
func (r *Runtime) RegisterProtocol(protocol *ProtoProtocol) {
	r.protocols[protocol.ProtocolID()] = protocol
}

// GetProtocol returns a protocol from the instance.
func (r *Runtime) GetProtocol(protocolID int) *ProtoProtocol {
	return r.protocols[protocolID]
}

// Cancel gracefully stops the runtime.
func (r *Runtime) Cancel() {
	// Cancel protocols
	r.cancelFunc()
	// Also cancel the network/session layers if present
	if r.sessionLayer != nil {
		r.sessionLayer.Cancel()
	}
	if r.networkLayer != nil {
		r.networkLayer.Cancel()
	}
	// Wait for all goroutines
	r.wg.Wait()
	slog.Info("runtime stopped")
}

// The central event loop
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
		slog.Info("starting protocol", "protocolID", protocol.ProtocolID())
		protocol.Start(ctx, &r.wg)
	}
}

func (r *Runtime) initProtocols() {
	for _, protocol := range r.protocols {
		slog.Info("initializing protocol", "protocolID", protocol.ProtocolID())
		protocol.Init()
	}
}

// Optionally called from main(), as you prefer
func (r *Runtime) setupLogger() {
	currentDate := time.Now().Format("2006-01-02")
	fileName := currentDate + ".log"

	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()

	mw := io.MultiWriter(os.Stdout, file)
	log.SetOutput(mw)
	log.Info("Logger initialized")
}

// senderSetter is implemented by messages that want to be informed
// about the remote Host that sent them.
type senderSetter interface {
	SetSender(net.Host)
}

// processMessage reads protocolID + messageID from the buffer and dispatches to the correct serializer.
// The buffer is expected to start with [ProtocolID(uint16) || MessageID(uint16) || Payload...].
func processMessage(buffer bytes.Buffer, from net.Host) {
	var protocolID, messageID uint16
	if err := binary.Read(&buffer, binary.LittleEndian, &protocolID); err != nil {
		// TODO: log error
		return
	}
	if err := binary.Read(&buffer, binary.LittleEndian, &messageID); err != nil {
		// TODO: log error
		return
	}

	runtime := GetRuntimeInstance()
	protocol, exists := runtime.protocols[int(protocolID)]
	if !exists {
		// TODO: unknown protocol
		slog.Warn("received message for unknown protocol", "protocolID", protocolID, "messageID", messageID)
		return
	}

	serializer, ok := protocol.msgSerializers[int(messageID)]
	if !ok {
		// TODO: unknown message
		slog.Warn("received message for unknown messageID", "protocolID", protocolID, "messageID", messageID)
		return
	}

	message, err := serializer.Deserialize(buffer)
	if err != nil {
		// TODO: log error
		slog.Error("failed to deserialize message", "protocolID", protocolID, "messageID", messageID, "err", err)
		return
	}

	// If the concrete message type supports setting the sender,
	// inform it of the remote host that sent this message.
	if m, ok := message.(senderSetter); ok {
		m.SetSender(from)
	}

	// push the message to that protocol's channel
	slog.Debug("dispatching message", "protocolID", protocolID, "messageID", messageID)
	protocol.messageChannel <- message
}
