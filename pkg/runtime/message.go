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

	Serializer interface {
		Serialize() (bytes.Buffer, error)
		Deserialize(buffer bytes.Buffer) (Message, error)
	}
)

// Helper to send a message up the Transport Layer
func SendMessage(msg Message, sendTo net.Host) {
	// 1) Serialize the message
	msgBuffer, err := msg.Serializer().Serialize()
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
	buffer.Write(msgBuffer.Bytes())

	// 3) Create a TransportMessage with the buffer
	transportMsg := net.NewTransportMessage(*buffer, net.NewTransportHost(sendTo.Port, sendTo.IP))

	// 4) Now call Send() with TWO arguments:
	//    - the TransportMessage
	//    - the TransportHost we want to send to
	GetRuntimeInstance().networkLayer.Send(
		transportMsg,
		net.NewTransportHost(sendTo.Port, sendTo.IP),
	)
}

// Utility for writing uint16 in little endian
func writeUint16(buf *bytes.Buffer, val uint16) error {
	tmp := make([]byte, 2)
	tmp[0] = byte(val)      // low byte
	tmp[1] = byte(val >> 8) // high byte
	_, err := buf.Write(tmp)
	return err
}
