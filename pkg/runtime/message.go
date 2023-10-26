package runtime

import (
	"bytes"
	"encoding/binary"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

// Message is an interface that all messages must implement.
// It is used to identify to which protocol the message belongs.

type (
	Message interface {
		MessageID() int
		ProtocolID() int
		Serializer() Serializer
		Sender() *net.Host
	}

	Serializer interface {
		Serialize() (*bytes.Buffer, error)
		Deserialize(buffer *bytes.Buffer) (Message, error)
	}
)

func SendMessage(msg Message, sendTo *net.Host) {
	msgBuffer, err := msg.Serializer().Serialize()
	if err != nil {
		// TODO: Replace with decent logger event.
	}

	// Create a buffer and write Sender's Host, ProtocolID, MessageID, and the message.
	buffer := new(bytes.Buffer)

	// Serialize the sender's Host
	senderHostBuffer, _ := net.SerializeHost(msg.Sender())
	buffer.Write(senderHostBuffer.Bytes())

	// Serialize ProtocolID and MessageID
	protocolID := uint16(msg.ProtocolID())
	err = binary.Write(buffer, binary.LittleEndian, protocolID)
	if err != nil {
		return
	}

	messageID := uint16(msg.MessageID())
	err = binary.Write(buffer, binary.LittleEndian, messageID)
	if err != nil {
		return
	}

	buffer.Write(msgBuffer.Bytes())

	networkMessage := net.NewNetworkMessage(buffer, sendTo)
	GetRuntimeInstance().networkLayer.Send(networkMessage)
}
