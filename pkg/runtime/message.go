package runtime

// Message is an interface that all messages must implement.
// It is used to identify to which protocol the message belongs.
type Message interface {
	HandlerID() MessageID
	ProtocolID() ProtocolID
	Serialize() ([]byte, error)
	Deserialize([]byte) (Message, error) // TODO: does this even make sense? maybe.
}

type MessageID int
