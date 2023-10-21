package runtime

import "bytes"

// Message is an interface that all messages must implement.
// It is used to identify to which protocol the message belongs.

type (
	Message interface {
		MessageID() int
		ProtocolID() int
		Serializer() Serializer
	}

	Serializer interface {
		Serialize() (bytes.Buffer, error)
		Deserialize(buffer bytes.Buffer) (Message, error)
	}
)
