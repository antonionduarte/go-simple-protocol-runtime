package runtime

import (
	"bytes"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

// Message is an interface that all messages must implement.
// It is used to identify to which protocol the message belongs.

type (
	Message interface {
		MessageID() int
		ProtocolID() int
		Serializer() Serializer
		Sender() net.Host
	}

	Serializer interface {
		Serialize() (*bytes.Buffer, error)
		Deserialize(buffer *bytes.Buffer) (Message, error)
	}
)
