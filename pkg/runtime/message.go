package runtime

import (
	"bytes"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

type (
	Message interface {
		MessageID() int
		ProtocolID() int
		Serializer() Serializer
		Sender() net.Host
	}

	Serializer interface {
		Serialize() ([]byte, error)
		Deserialize(data []byte) (Message, error)
	}
)

func SendMessage(msg Message, sendTo net.Host) {
	payload, err := msg.Serializer().Serialize()
	if err != nil {
		return
	}

	buffer := new(bytes.Buffer)

	protocolID := uint16(msg.ProtocolID())
	messageID := uint16(msg.MessageID())

	_ = writeUint16(buffer, protocolID)
	_ = writeUint16(buffer, messageID)

	buffer.Write(payload)

	runtime := GetRuntimeInstance()
	runtime.sessionLayer.Send(*buffer, sendTo)
}

func writeUint16(buf *bytes.Buffer, val uint16) error {
	tmp := make([]byte, 2)
	tmp[0] = byte(val)      // low byte
	tmp[1] = byte(val >> 8) // high byte
	_, err := buf.Write(tmp)
	return err
}
