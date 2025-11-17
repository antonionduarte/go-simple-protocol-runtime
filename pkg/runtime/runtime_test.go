package runtime

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

/* ------------------------------------------------------------------
   Mock Protocol
------------------------------------------------------------------ */

type MockProtocol struct {
	StartCalled bool
	InitCalled  bool
	ProtoID     int
	MockSelf    net.Host

	// For message dispatch tests
	HandledMessages []Message
}

func (m *MockProtocol) Start(ctx ProtocolContext) {
	m.StartCalled = true
}
func (m *MockProtocol) Init(ctx ProtocolContext) {
	m.InitCalled = true
}
func (m *MockProtocol) ProtocolID() int {
	return m.ProtoID
}
func (m *MockProtocol) Self() net.Host {
	return m.MockSelf
}

// RecordMessageHandler is a helper handler that appends messages to the protocol's slice.
func (m *MockProtocol) RecordMessageHandler(msg Message) {
	m.HandledMessages = append(m.HandledMessages, msg)
}

/* ------------------------------------------------------------------
   Mock Network Layer
   This now matches the new TransportLayer interface:
   interface {
       Connect(host TransportHost)
       Disconnect(host TransportHost)
       Send(msg TransportMessage, sendTo TransportHost)
       OutChannel() chan TransportMessage
       OutTransportEvents() chan TransportEvent
       Cancel()
   }
------------------------------------------------------------------ */

type MockNetworkLayer struct {
	ConnectCalled    bool
	DisconnectCalled bool
	SendCalled       bool
	CancelCalled     bool

	outChannel         chan net.TransportMessage
	outTransportEvents chan net.TransportEvent
}

// Factory function to create a mock with buffered channels
func NewMockNetworkLayer() *MockNetworkLayer {
	return &MockNetworkLayer{
		outChannel:         make(chan net.TransportMessage, 1),
		outTransportEvents: make(chan net.TransportEvent, 1),
	}
}

// Implement TransportLayer interface
func (m *MockNetworkLayer) Connect(host net.Host) {
	m.ConnectCalled = true
}
func (m *MockNetworkLayer) Disconnect(host net.Host) {
	m.DisconnectCalled = true
}
func (m *MockNetworkLayer) Send(msg net.TransportMessage, sendTo net.Host) {
	m.SendCalled = true
}
func (m *MockNetworkLayer) OutChannel() chan net.TransportMessage {
	return m.outChannel
}
func (m *MockNetworkLayer) OutTransportEvents() chan net.TransportEvent {
	return m.outTransportEvents
}
func (m *MockNetworkLayer) Cancel() {
	m.CancelCalled = true
}

/* ------------------------------------------------------------------
   Tests
------------------------------------------------------------------ */

func TestGetRuntimeInstance(t *testing.T) {
	instance1 := GetRuntimeInstance()
	instance2 := GetRuntimeInstance()

	if instance1 != instance2 {
		t.Errorf("GetRuntimeInstance should return the same instance, but got two different pointers")
	}
}

func TestRegisterProtocol(t *testing.T) {
	runtime := GetRuntimeInstance()

	// We create a mock protocol
	testHost := net.NewHost(8080, "127.0.0.1") // net.Host (value)
	mockProtocol := &MockProtocol{ProtoID: 123, MockSelf: testHost}

	// Wrap in a ProtoProtocol
	protoProtocol := NewProtoProtocol(mockProtocol, testHost)

	runtime.RegisterProtocol(protoProtocol)

	if _, exists := runtime.protocols[mockProtocol.ProtocolID()]; !exists {
		t.Errorf("Protocol was not registered correctly")
	}
}

func TestStartAndCancel(t *testing.T) {
	runtime := GetRuntimeInstance()

	// We create and register a MockNetworkLayer that matches the new TransportLayer interface
	mockNetworkLayer := NewMockNetworkLayer()
	runtime.RegisterNetworkLayer(mockNetworkLayer)
	// And a SessionLayer that wraps it (no real traffic in this test)
	self := net.NewHost(0, "127.0.0.1")
	session := net.NewSessionLayer(mockNetworkLayer, self, context.Background())
	runtime.RegisterSessionLayer(session)

	// Start the runtime
	runtime.Start()

	// Cancel the runtime
	runtime.Cancel()

	// Ensure the mock network layer's Cancel() was called
	if !mockNetworkLayer.CancelCalled {
		t.Errorf("Expected Cancel to be called on network layer, but it wasn't")
	}
}

// -------------------------------------------------------------------
// processMessage tests
// -------------------------------------------------------------------

// localMessage is a simple Message implementation used in processMessage tests.
type localMessage struct {
	id, pid int
	sender  net.Host
}

func (m *localMessage) MessageID() int         { return m.id }
func (m *localMessage) ProtocolID() int        { return m.pid }
func (m *localMessage) Serializer() Serializer { return &localSerializer{} }
func (m *localMessage) Sender() net.Host       { return m.sender }

// Ensure localMessage implements Message.
var _ Message = (*localMessage)(nil)

// localSerializer is a no-op serializer (we only care about Deserialize in tests).
type localSerializer struct{}

func (s *localSerializer) Serialize() (bytes.Buffer, error) {
	return *bytes.NewBuffer(nil), nil
}

func (s *localSerializer) Deserialize(buf bytes.Buffer) (Message, error) {
	return &localMessage{}, nil
}

// testSerializer lets us inject a specific Message or error from Deserialize.
type testSerializer struct {
	msg Message
	err error
}

func (s *testSerializer) Serialize() (bytes.Buffer, error) {
	return *bytes.NewBuffer(nil), nil
}

func (s *testSerializer) Deserialize(buf bytes.Buffer) (Message, error) {
	return s.msg, s.err
}

// TestProcessMessage_DispatchesMessage verifies that a well-formed buffer
// is deserialized and pushed into the correct protocol's messageChannel.
func TestProcessMessage_DispatchesToHandler(t *testing.T) {
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

// assertError is a simple error type used for testing.
type assertError struct{}

func (assertError) Error() string { return "expected deserialize error" }

// -------------------------------------------------------------------
// Integration-style end-to-end test (runtime + session + TCP)
// -------------------------------------------------------------------

// IntegrationProtocol is a minimal protocol used to test end-to-end
// behaviour: when a session is established with a peer, it sends a
// single localMessage to that peer.
type IntegrationProtocol struct {
	ProtoID  int
	SelfHost net.Host
	Peer     net.Host
}

func (p *IntegrationProtocol) Start(ctx ProtocolContext) {
	// No handlers needed for this test; we only test send path.
}

func (p *IntegrationProtocol) Init(ctx ProtocolContext) {
	// Initiate a session to the peer.
	ctx.Connect(p.Peer)
}

func (p *IntegrationProtocol) ProtocolID() int { return p.ProtoID }
func (p *IntegrationProtocol) Self() net.Host  { return p.SelfHost }

// OnSessionConnected sends a single localMessage to the peer when the
// session is established.
func (p *IntegrationProtocol) OnSessionConnected(h net.Host) {
	if !net.CompareHost(h, p.Peer) {
		return
	}
	msg := &localMessage{
		id:     1,
		pid:    p.ProtoID,
		sender: p.SelfHost,
	}
	SendMessage(msg, p.Peer)
}

func (p *IntegrationProtocol) OnSessionDisconnected(h net.Host) {}

// waitSessionEventRuntime is a helper to wait for a net.SessionEvent with a timeout.
func waitSessionEventRuntime(t *testing.T, ch chan net.SessionEvent, timeout time.Duration) net.SessionEvent {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for SessionEvent")
		return nil
	}
}

// TestRuntime_EndToEndIntegration sets up a runtime with real TCPLayer and
// SessionLayer, plus a remote SessionLayer, and verifies that when the
// protocol sends a message on OnSessionConnected, the remote side receives
// an application-level SessionMessage with the expected protocol/message IDs.
func TestRuntime_EndToEndIntegration(t *testing.T) {
	runtime := GetRuntimeInstance()

	hostA := net.NewHost(7301, "127.0.0.1")
	hostB := net.NewHost(7302, "127.0.0.1")

	// Contexts for both sides
	ctxA, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()

	// Transport + session for A (owned by runtime)
	tcpA := net.NewTCPLayer(hostA, ctxA)
	sessionA := net.NewSessionLayer(tcpA, hostA, ctxA)
	runtime.RegisterNetworkLayer(tcpA)
	runtime.RegisterSessionLayer(sessionA)

	// Transport + session for B (no runtime)
	tcpB := net.NewTCPLayer(hostB, ctxB)
	sessionB := net.NewSessionLayer(tcpB, hostB, ctxB)

	defer tcpB.Cancel()

	// Register integration protocol
	proto := NewProtoProtocol(&IntegrationProtocol{
		ProtoID:  1001,
		SelfHost: hostA,
		Peer:     hostB,
	}, hostA)
	runtime.RegisterProtocol(proto)

	// Start the runtime
	runtime.Start()
	defer runtime.Cancel()

	// Wait for B to see SessionConnected
	evB := waitSessionEventRuntime(t, sessionB.OutChannelEvents(), 5*time.Second)
	if _, ok := evB.(*net.SessionConnected); !ok {
		t.Fatalf("expected SessionConnected on B, got %T", evB)
	}

	// Now wait for a session-level application message on B
	select {
	case sm := <-sessionB.OutMessages():
		// sm.Msg should contain [LayerID(Application) handled already] and then
		// [ProtocolID || MessageID || Payload]. We only check ProtocolID/MessageID.
		buf := sm.Msg
		if buf.Len() < 4 {
			t.Fatalf("expected at least 4 bytes for protocolID/messageID, got %d", buf.Len())
		}
		var protoID, msgID uint16
		if err := binary.Read(&buf, binary.LittleEndian, &protoID); err != nil {
			t.Fatalf("failed to read protocolID: %v", err)
		}
		if err := binary.Read(&buf, binary.LittleEndian, &msgID); err != nil {
			t.Fatalf("failed to read messageID: %v", err)
		}
		if protoID != 1001 || msgID != 1 {
			t.Fatalf("unexpected protocolID/messageID: got (%d,%d), want (1001,1)", protoID, msgID)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for SessionMessage on B")
	}
}
