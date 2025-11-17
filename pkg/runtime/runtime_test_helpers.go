package runtime

import (
	"sync"
	"testing"
	"time"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

/* ------------------------------------------------------------------
   Shared test helpers: protocols, messages, serializers, network mocks
   ------------------------------------------------------------------ */

// resetRuntimeForTests clears the global Runtime singleton so tests that need
// a fresh instance can start from a clean slate. It must only be called from
// test code.
func resetRuntimeForTests() {
	instance = nil
	once = sync.Once{}
}

// MockProtocol is a simple Protocol used across tests to assert that
// Start/Init were called and that messages were dispatched correctly.
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

// MockNetworkLayer is a fake TransportLayer used by core runtime tests.
type MockNetworkLayer struct {
	ConnectCalled    bool
	DisconnectCalled bool
	SendCalled       bool
	CancelCalled     bool

	outChannel         chan net.TransportMessage
	outTransportEvents chan net.TransportEvent
}

// NewMockNetworkLayer creates a mock with buffered channels.
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

func (s *localSerializer) Serialize() ([]byte, error) {
	return nil, nil
}

func (s *localSerializer) Deserialize(data []byte) (Message, error) {
	return &localMessage{}, nil
}

// testSerializer lets us inject a specific Message or error from Deserialize.
type testSerializer struct {
	msg Message
	err error
}

func (s *testSerializer) Serialize() ([]byte, error) {
	return nil, nil
}

func (s *testSerializer) Deserialize(data []byte) (Message, error) {
	return s.msg, s.err
}

// assertError is a simple error type used for testing.
type assertError struct{}

func (assertError) Error() string { return "expected deserialize error" }

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
