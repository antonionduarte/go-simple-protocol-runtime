package runtime

// Message is an interface that all messages must implement.
// It is used to identify to which protocol the message belongs.
type Message interface {
	HandlerID() MessageID
	ProtocolID() ProtocolID
	Serialize() ([]byte, error)
	Deserialize() (Message, error)
}

type MessageID int
