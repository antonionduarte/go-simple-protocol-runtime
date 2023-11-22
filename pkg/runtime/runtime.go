package runtime

import (
	"context"
	"encoding/binary"
	"io"
	"os"
	"sync"
	"time"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"

	log "github.com/sirupsen/logrus"
)

type Runtime struct {
	cancelFunc    func() // TODO: Should I do this? Is this idiomatic?
	msgChannel    chan Message
	timerChannel  chan Timer
	timerMutex    sync.Mutex
	wg            sync.WaitGroup
	ongoingTimers map[int]*time.Timer
	protocols     map[int]*ProtoProtocol
	networkLayer  net.TransportLayer
}

var (
	instance *Runtime
	once     sync.Once
)

// GetRuntimeInstance creates a new instance.
func GetRuntimeInstance() *Runtime {
	once.Do(func() {
		instance = &Runtime{
			msgChannel:   make(chan Message, 1),
			timerChannel: make(chan Timer, 1),
			protocols:    make(map[int]*ProtoProtocol, 1),
		}
	})
	return instance
}

// RegisterMessageHandler registers a message handler to the instance.
func RegisterMessageHandler(protocolID int, messageID int, handler func(Message)) {
	runtime := GetRuntimeInstance()
	runtime.protocols[protocolID].RegisterMessageHandler(messageID, handler)
}

// RegisterMessageSerializer registers a message serializer to the instance.
func RegisterMessageSerializer(protocolID int, messageID int, serializer Serializer) {
	runtime := GetRuntimeInstance()
	runtime.protocols[protocolID].RegisterMessageSerializer(messageID, serializer)
}

// Start starts the instance, and runs the start and init function for all the protocols.
func (r *Runtime) Start() {
	if r.networkLayer == nil {
		// TODO: Replace with decent logger event.
		panic("Network layer not registered")
	}

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	r.cancelFunc = cancel

	r.startProtocols(ctx) // TODO: pass it the waitgroup
	r.initProtocols()
	r.wg.Add(1)
	go r.eventHandler(ctx)
}

func (r *Runtime) StartWithDuration(duration time.Duration) {
	r.Start()
	time.AfterFunc(duration, func() {
		r.Cancel()
	})
}

// GetProtocol returns a protocol from the instance.
func (r *Runtime) GetProtocol(protocolID int) *ProtoProtocol {
	return r.protocols[protocolID]
}

// RegisterProtocol registers a protocol to the instance.
// It must take in a ProtoProtocol, which should encapsulate the protocol that you yourself develop.
func (r *Runtime) RegisterProtocol(protocol *ProtoProtocol) {
	r.protocols[protocol.ProtocolID()] = protocol
}

// RegisterNetworkLayer registers the network layer that this runtime will use.
// It takes in the TransportLayer that you're going to use. e.g. (pkg/runtime/net/tcp.go)
func (r *Runtime) RegisterNetworkLayer(networkLayer net.TransportLayer) {
	r.networkLayer = networkLayer
}

func (r *Runtime) Cancel() {
	r.cancelFunc()          // this finishes execution of all the protocols
	r.networkLayer.Cancel() // this finishes execution of the network layer
	r.wg.Wait()
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
		case networkMessage := <-r.networkLayer.OutChannel():
			receiveMessage(networkMessage)
		}
	}
}

func (r *Runtime) startProtocols(ctx context.Context) {
	for _, protocol := range r.protocols {
		protocol.Start(ctx, &r.wg)
	}
}

func (r *Runtime) initProtocols() {
	for _, protocol := range r.protocols {
		protocol.Init()
	}
}

func (r *Runtime) setupLogger() {
	currentDate := time.Now().Format("2006-01-02")
	fileName := currentDate + ".log"

	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	mw := io.MultiWriter(os.Stdout, file)
	log.SetOutput(mw)
	log.Info("Logger initialized")
}

// TODO: This function could honestly have a better name.
// processMessage would be better probably.
// receiveMessage receives a message from the Network Layer.
func receiveMessage(networkMessage net.TransportMessage) {
	buffer := networkMessage.Msg

	var protocolID, messageID uint16
	if err := binary.Read(&buffer, binary.LittleEndian, &protocolID); err != nil {
		// TODO: Handle the error
		return
	}
	if err := binary.Read(&buffer, binary.LittleEndian, &messageID); err != nil {
		// TODO: Handle the error
		return
	}

	protocol, exists := instance.protocols[int(protocolID)]
	if !exists {
		// TODO: Handle the error (unknown protocol)
		return
	}

	message, err := protocol.msgSerializers[int(messageID)].Deserialize(buffer)
	if err != nil {
		// TODO: Handle the error
		return
	}

	protocol.messageChannel <- message
}
