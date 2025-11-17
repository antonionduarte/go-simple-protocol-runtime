package runtime

import (
	"context"
	"testing"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

func TestGetRuntimeInstance(t *testing.T) {
	resetRuntimeForTests()
	instance1 := GetRuntimeInstance()
	instance2 := GetRuntimeInstance()

	if instance1 != instance2 {
		t.Errorf("GetRuntimeInstance should return the same instance, but got two different pointers")
	}
}

func TestRegisterProtocol(t *testing.T) {
	resetRuntimeForTests()
	runtime := GetRuntimeInstance()

	testHost := net.NewHost(8080, "127.0.0.1") // net.Host (value)
	mockProtocol := &MockProtocol{ProtoID: 123, MockSelf: testHost}

	protoProtocol := NewProtoProtocol(mockProtocol, testHost)

	runtime.RegisterProtocol(protoProtocol)

	if _, exists := runtime.protocols[mockProtocol.ProtocolID()]; !exists {
		t.Errorf("Protocol was not registered correctly")
	}
}

func TestStartAndCancel(t *testing.T) {
	resetRuntimeForTests()
	runtime := GetRuntimeInstance()

	mockNetworkLayer := NewMockNetworkLayer()
	runtime.RegisterNetworkLayer(mockNetworkLayer)

	self := net.NewHost(0, "127.0.0.1")
	session := net.NewSessionLayer(mockNetworkLayer, self, context.Background())
	runtime.RegisterSessionLayer(session)

	runtime.Start()

	runtime.Cancel()

	if !mockNetworkLayer.CancelCalled {
		t.Errorf("Expected Cancel to be called on network layer, but it wasn't")
	}
}
