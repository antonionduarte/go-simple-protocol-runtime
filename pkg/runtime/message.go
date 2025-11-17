package runtime

import (
	"bytes"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

// Message is an interface all messages must implement
type (
	Message interface {
		MessageID() int
		ProtocolID() int
		Serializer() Serializer
		Sender() net.Host
	}

	// Serializer is responsible for turning concrete messages into a byte
	// slice and back. The runtime takes care of framing (LayerID,
	// ProtocolID, MessageID), so serializers only deal with the message
	// payload.
	Serializer interface {
		Serialize() ([]byte, error)
		Deserialize(data []byte) (Message, error)
	}
)

// Helper to send a message up the Transport Layer
func SendMessage(msg Message, sendTo net.Host) {
	// 1) Serialize the message
	payload, err := msg.Serializer().Serialize()
	if err != nil {
		// log or handle error
		return
	}

	// 2) Create a buffer that includes [protocolID, messageID] + the msg bytes
	buffer := new(bytes.Buffer)

	protocolID := uint16(msg.ProtocolID())
	messageID := uint16(msg.MessageID())

	// Write the protocolID, messageID
	_ = writeUint16(buffer, protocolID)
	_ = writeUint16(buffer, messageID)

	// Append the actual message payload
	buffer.Write(payload)

	// 3) Send the message via the session layer, marking it as an
	// Application-level message (LayerID is added by the session layer).
	runtime := GetRuntimeInstance()
	runtime.sessionLayer.Send(*buffer, sendTo)
}

// Utility for writing uint16 in little endian
func writeUint16(buf *bytes.Buffer, val uint16) error {
	tmp := make([]byte, 2)
	tmp[0] = byte(val)      // low byte
	tmp[1] = byte(val >> 8) // high byte
	_, err := buf.Write(tmp)
	return err
}
