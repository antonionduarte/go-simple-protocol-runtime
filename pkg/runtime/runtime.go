package runtime

import (
	"encoding/binary"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
	"sync"
)

type Runtime struct {
	msgChannel   chan Message
	timerChannel chan Timer
	protocols    map[int]ProtoProtocol
	networkLayer net.NetworkLayer
}

var instance *Runtime
var once sync.Once

// GetRuntimeInstance creates a new instance.
func GetRuntimeInstance() *Runtime {
	once.Do(func() {
		instance = &Runtime{
			msgChannel:   make(chan Message, 1),
			timerChannel: make(chan Timer, 1),
			protocols:    make(map[int]ProtoProtocol, 1),
		}
	})
	return instance
}

// Start starts the instance, and runs the start and init function for all the protocols.
func (r *Runtime) Start() {
	if r.networkLayer == nil {
		// TODO: Replace with decent logger event.
		panic("Network layer not registered")
	}

	r.startProtocols()
	r.initProtocols()

	for {
		select {
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

// RegisterProtocol registers a protocol to the instance.
// It must take in a ProtoProtocol, which should encapsulate the protocol that you yourself develop.
func (r *Runtime) RegisterProtocol(protocol ProtoProtocol) {
	r.protocols[protocol.ProtocolID()] = protocol
}

func (r *Runtime) RegisterNetworkLayer(networkLayer net.NetworkLayer) {
	r.networkLayer = networkLayer
}

// receiveMessage receives a message from the Network Layer.
func receiveMessage(networkMessage *net.NetworkMessage) {
	buffer := networkMessage.Msg

	var protocolID, messageID uint16
	if err := binary.Read(buffer, binary.LittleEndian, &protocolID); err != nil {
		// TODO: Handle the error
		return
	}
	if err := binary.Read(buffer, binary.LittleEndian, &messageID); err != nil {
		// TODO: Handle the error
		return
	}

	runtimeInstance := GetRuntimeInstance()
	protocol, exists := runtimeInstance.protocols[int(protocolID)]
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

func (r *Runtime) startProtocols() {
	for _, protocol := range r.protocols {
		protocol.Start()
	}
}

func (r *Runtime) initProtocols() {
	for _, protocol := range r.protocols {
		protocol.Init()
	}
}
