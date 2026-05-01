package runtime

import (
	"context"
	"testing"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

func TestRegisterProtocol(t *testing.T) {
	testHost := net.NewHost(8080, "127.0.0.1")
	rt := New(testHost)

	mockProtocol := &MockProtocol{ProtoID: 123, MockSelf: testHost}
	protoProtocol := NewProtoProtocol(mockProtocol, testHost)
	rt.RegisterProtocol(protoProtocol)

	if _, exists := rt.protocols[mockProtocol.ProtocolID()]; !exists {
		t.Errorf("Protocol was not registered correctly")
	}
}

func TestStartAndCancel(t *testing.T) {
	self := net.NewHost(0, "127.0.0.1")
	rt := New(self)

	mockNetworkLayer := NewMockNetworkLayer()
	rt.RegisterNetworkLayer(mockNetworkLayer)
	session := net.NewSessionLayer(mockNetworkLayer, self, context.Background(), 0, 0)
	rt.RegisterSessionLayer(session)

	if err := rt.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	rt.Cancel()

	if !mockNetworkLayer.CancelCalled {
		t.Errorf("Expected Cancel to be called on network layer, but it wasn't")
	}
}

func TestStart_FailsWithoutLayers(t *testing.T) {
	rt := New(net.NewHost(0, "127.0.0.1"))
	if err := rt.Start(); err == nil {
		t.Fatalf("expected Start to fail without network/session layer registered")
	}

	rt.RegisterNetworkLayer(NewMockNetworkLayer())
	if err := rt.Start(); err == nil {
		t.Fatalf("expected Start to fail without session layer registered")
	}
}
