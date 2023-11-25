package runtime

import (
	"testing"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

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

type MockNetworkLayer struct {
	ConnectCalled    bool
	DisconnectCalled bool
	SendCalled       bool
	CancelCalled     bool
	StartCalled      bool
}

func (m *MockNetworkLayer) Connect(host net.Host) {
	m.ConnectCalled = true
}

func (m *MockNetworkLayer) Disconnect(host net.Host) {
	m.DisconnectCalled = true
}

func (m *MockNetworkLayer) Send(msg net.TransportMessage) {
	m.SendCalled = true
}

func (m *MockNetworkLayer) OutChannel() chan net.TransportMessage {
	return make(chan net.TransportMessage, 1)
}

func (m *MockNetworkLayer) OutChannelEvents() chan net.TransportConnEvents {
	return make(chan net.TransportConnEvents, 1)
}

func (m *MockNetworkLayer) Cancel() {
	m.CancelCalled = true
}

func TestGetRuntimeInstance(t *testing.T) {
	instance1 := GetRuntimeInstance()
	instance2 := GetRuntimeInstance()

	if instance1 != instance2 {
		t.Errorf("GetRuntimeInstance should return the same instance")
	}
}

func TestRegisterProtocol(t *testing.T) {
	runtime := GetRuntimeInstance()
	mockProtocol := &MockProtocol{ProtoID: 123}
	testHost := net.NewHost(8080, "127.0.0.1") // Create a Host instance for testing

	protoProtocol := NewProtoProtocol(mockProtocol, testHost)

	runtime.RegisterProtocol(protoProtocol)

	if _, exists := runtime.protocols[mockProtocol.ProtocolID()]; !exists {
		t.Errorf("Protocol was not registered correctly")
	}
}

func TestStartAndCancel(t *testing.T) {
	runtime := GetRuntimeInstance()
	mockNetworkLayer := new(MockNetworkLayer)
	runtime.RegisterNetworkLayer(mockNetworkLayer)

	runtime.Start()
	runtime.Cancel()
	if !mockNetworkLayer.CancelCalled {
		t.Errorf("Expected Cancel to be called on network layer, but it wasn't")
	}
}
