package runtime

import (
	"bytes"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

type (
	// Message is the minimal interface that all protocol messages must
	// implement to be sent through the runtime. It is part of the public API
	// exposed to protocol implementations.
	Message interface {
		MessageID() int
		ProtocolID() int
		Serializer() Serializer
		Sender() net.Host
	}

	// Serializer is responsible for encoding and decoding concrete Message
	// types. Implementations must obey the following contract:
	//   - Serialize MUST NOT mutate the Message instance.
	//   - Deserialize MUST return a new Message instance of the correct
	//     concrete type for the given protocol/message ID.
	Serializer interface {
		Serialize() ([]byte, error)
		Deserialize(data []byte) (Message, error)
	}
)

func SendMessage(msg Message, sendTo net.Host) error {
	runtime := GetRuntimeInstance()
	logger := runtime.Logger()

	payload, err := msg.Serializer().Serialize()
	if err != nil {
		logger.Error("failed to serialize message",
			"protocolID", msg.ProtocolID(),
			"messageID", msg.MessageID(),
			"to", sendTo.ToString(),
			"err", err,
		)
		return err
	}

	// Application-level header + payload format:
	//   [ProtocolID(uint16 LE) || MessageID(uint16 LE) || Payload...]
	buffer := new(bytes.Buffer)

	protocolID := uint16(msg.ProtocolID())
	messageID := uint16(msg.MessageID())

	if err := writeUint16(buffer, protocolID); err != nil {
		logger.Error("failed to encode protocolID header",
			"protocolID", msg.ProtocolID(),
			"messageID", msg.MessageID(),
			"to", sendTo.ToString(),
			"err", err,
		)
		return err
	}
	if err := writeUint16(buffer, messageID); err != nil {
		logger.Error("failed to encode messageID header",
			"protocolID", msg.ProtocolID(),
			"messageID", msg.MessageID(),
			"to", sendTo.ToString(),
			"err", err,
		)
		return err
	}

	buffer.Write(payload)

	runtime.sessionLayer.Send(*buffer, sendTo)
	return nil
}

func writeUint16(buf *bytes.Buffer, val uint16) error {
	tmp := make([]byte, 2)
	tmp[0] = byte(val)      // low byte
	tmp[1] = byte(val >> 8) // high byte
	_, err := buf.Write(tmp)
	return err
}
