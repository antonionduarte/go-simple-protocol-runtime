package runtime

import (
	"bytes"
	"testing"
	"time"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

// TestProcessMessage_DispatchesMessage verifies that a well-formed buffer
// is deserialized and pushed into the correct protocol's messageChannel.
func TestProcessMessage_DispatchesToHandler(t *testing.T) {
	resetRuntimeForTests()
	runtime := GetRuntimeInstance()

	// Prepare a mock protocol and register it.
	testHost := net.NewHost(9001, "127.0.0.1")
	mockProtocol := &MockProtocol{ProtoID: 42, MockSelf: testHost}
	proto := NewProtoProtocol(mockProtocol, testHost)
	runtime.RegisterProtocol(proto)

	// Register a serializer for messageID = 1 that always returns our test message.
	const messageID = 1
	testMsg := &localMessage{id: messageID, pid: mockProtocol.ProtoID, sender: testHost}

	ts := &testSerializer{
		msg: testMsg,
	}

	proto.RegisterMessageSerializer(messageID, ts)

	// Build a buffer: [ProtocolID(uint16) || MessageID(uint16)]
	var buf bytes.Buffer
	_ = writeUint16(&buf, uint16(mockProtocol.ProtoID))
	_ = writeUint16(&buf, uint16(messageID))

	// Call processMessage with some sender host.
	from := net.NewHost(9999, "127.0.0.1")
	processMessage(buf, from)

	// Because processMessage pushes onto proto.messageChannel, ensure we receive it.
	select {
	case m := <-proto.MessageChannel():
		if m.MessageID() != messageID || m.ProtocolID() != mockProtocol.ProtoID {
			t.Fatalf("unexpected message dispatched: %+v", m)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected a message to be dispatched to proto.messageChannel")
	}
}

// TestProcessMessage_UnknownProtocolID ensures that an unknown protocol ID
// does not panic and does not dispatch a message.
func TestProcessMessage_UnknownProtocolID(t *testing.T) {
	// Build a buffer with a protocol ID that is not registered.
	const unknownProtoID = 9999
	const messageID = 1

	var buf bytes.Buffer
	_ = writeUint16(&buf, uint16(unknownProtoID))
	_ = writeUint16(&buf, uint16(messageID))

	from := net.NewHost(8888, "127.0.0.1")
	// Simply call processMessage and ensure it returns without panic.
	processMessage(buf, from)
}

// TestProcessMessage_UnknownMessageID ensures that a registered protocol but
// unregistered message ID results in no dispatch.
func TestProcessMessage_UnknownMessageID(t *testing.T) {
	resetRuntimeForTests()
	runtime := GetRuntimeInstance()

	testHost := net.NewHost(9101, "127.0.0.1")
	mockProtocol := &MockProtocol{ProtoID: 43, MockSelf: testHost}
	proto := NewProtoProtocol(mockProtocol, testHost)
	runtime.RegisterProtocol(proto)

	const unknownMessageID = 99

	var buf bytes.Buffer
	_ = writeUint16(&buf, uint16(mockProtocol.ProtoID))
	_ = writeUint16(&buf, uint16(unknownMessageID))

	from := net.NewHost(9999, "127.0.0.1")
	processMessage(buf, from)

	// There should be no message dispatched to this proto.
	select {
	case m := <-proto.MessageChannel():
		t.Fatalf("did not expect a message to be dispatched, got %+v", m)
	default:
		// ok
	}
}

// TestProcessMessage_DeserializeError ensures that if the serializer returns
// an error, the message is not dispatched.
func TestProcessMessage_DeserializeError(t *testing.T) {
	resetRuntimeForTests()
	runtime := GetRuntimeInstance()

	testHost := net.NewHost(9201, "127.0.0.1")
	mockProtocol := &MockProtocol{ProtoID: 44, MockSelf: testHost}
	proto := NewProtoProtocol(mockProtocol, testHost)
	runtime.RegisterProtocol(proto)

	const messageID = 1

	ts := &testSerializer{
		msg: nil,
		err: assertError{},
	}

	proto.RegisterMessageSerializer(messageID, ts)

	var buf bytes.Buffer
	_ = writeUint16(&buf, uint16(mockProtocol.ProtoID))
	_ = writeUint16(&buf, uint16(messageID))

	from := net.NewHost(9999, "127.0.0.1")
	processMessage(buf, from)

	// There should be no message dispatched to this proto.
	select {
	case m := <-proto.MessageChannel():
		t.Fatalf("did not expect a message to be dispatched when Deserialize fails, got %+v", m)
	default:
		// ok
	}
}

// TestProcessMessage_TruncatedHeader ensures that a buffer that doesn't contain
// enough bytes for ProtocolID/MessageID is handled gracefully.
func TestProcessMessage_TruncatedHeader(t *testing.T) {
	resetRuntimeForTests()
	_ = GetRuntimeInstance()

	// Only 1 byte, but we need at least 4 for protocolID+messageID.
	var buf bytes.Buffer
	buf.WriteByte(0x01)

	from := net.NewHost(9999, "127.0.0.1")
	processMessage(buf, from)
	// No panic and no message dispatched: there is no protocol registered in this test.
}
