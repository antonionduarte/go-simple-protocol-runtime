package runtime

import (
	"testing"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

type failingSerializer struct{}

func (f *failingSerializer) Serialize() ([]byte, error) {
	return nil, assertError{}
}

func (f *failingSerializer) Deserialize(data []byte) (Message, error) {
	return nil, assertError{}
}

type failingMessage struct {
	id        int
	pid       int
	serializer Serializer
	sender    net.Host
}

func (m *failingMessage) MessageID() int         { return m.id }
func (m *failingMessage) ProtocolID() int        { return m.pid }
func (m *failingMessage) Serializer() Serializer { return m.serializer }
func (m *failingMessage) Sender() net.Host       { return m.sender }

func TestSendMessage_SerializeError(t *testing.T) {
	resetRuntimeForTests()
	_ = GetRuntimeInstance() // ensure runtime is initialized

	msg := &failingMessage{
		id:        1,
		pid:       123,
		serializer: &failingSerializer{},
		sender:    net.NewHost(0, "127.0.0.1"),
	}
	to := net.NewHost(8000, "127.0.0.1")

	if err := SendMessage(msg, to); err == nil {
		t.Fatalf("expected SendMessage to return error when Serialize fails")
	}
}


