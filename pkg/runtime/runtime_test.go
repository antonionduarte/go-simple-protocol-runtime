package runtime

import (
	"context"
	"testing"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

/* ------------------------------------------------------------------
   Mock Protocol
------------------------------------------------------------------ */

type MockProtocol struct {
	StartCalled bool
	InitCalled  bool
	ProtoID     int
	MockSelf    *net.Host
}

func (m *MockProtocol) Start() {
	m.StartCalled = true
}
func (m *MockProtocol) Init() {
	m.InitCalled = true
}
func (m *MockProtocol) ProtocolID() int {
	return m.ProtoID
}
func (m *MockProtocol) Self() *net.Host {
	return m.MockSelf
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
	mockProtocol := &MockProtocol{ProtoID: 123}
	testHost := net.NewHost(8080, "127.0.0.1") // net.Host (value)

	// Wrap in a ProtoProtocol
	// pass &testHost instead of testHost
	protoProtocol := NewProtoProtocol(mockProtocol, &testHost)

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
