package runtime

import (
	"sync"
	"testing"
	"time"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

func resetRuntimeForTests() {
	instance = nil
	once = sync.Once{}
}

type MockProtocol struct {
	StartCalled bool
	InitCalled  bool
	ProtoID     int
	MockSelf    net.Host

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

func (m *MockProtocol) RecordMessageHandler(msg Message) {
	m.HandledMessages = append(m.HandledMessages, msg)
}

type MockNetworkLayer struct {
	ConnectCalled    bool
	DisconnectCalled bool
	SendCalled       bool
	CancelCalled     bool

	outChannel         chan net.TransportMessage
	outTransportEvents chan net.TransportEvent
}

func NewMockNetworkLayer() *MockNetworkLayer {
	return &MockNetworkLayer{
		outChannel:         make(chan net.TransportMessage, 1),
		outTransportEvents: make(chan net.TransportEvent, 1),
	}
}

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

type localMessage struct {
	id, pid int
	sender  net.Host
}

func (m *localMessage) MessageID() int         { return m.id }
func (m *localMessage) ProtocolID() int        { return m.pid }
func (m *localMessage) Serializer() Serializer { return &localSerializer{} }
func (m *localMessage) Sender() net.Host       { return m.sender }

var _ Message = (*localMessage)(nil)

type localSerializer struct{}

func (s *localSerializer) Serialize() ([]byte, error) {
	return nil, nil
}

func (s *localSerializer) Deserialize(data []byte) (Message, error) {
	return &localMessage{}, nil
}

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

type assertError struct{}

func (assertError) Error() string { return "expected deserialize error" }

type IntegrationProtocol struct {
	ProtoID  int
	SelfHost net.Host
	Peer     net.Host
}

func (p *IntegrationProtocol) Start(ctx ProtocolContext) {
}

func (p *IntegrationProtocol) Init(ctx ProtocolContext) {
	ctx.Connect(p.Peer)
}

func (p *IntegrationProtocol) ProtocolID() int { return p.ProtoID }
func (p *IntegrationProtocol) Self() net.Host  { return p.SelfHost }

func (p *IntegrationProtocol) OnSessionConnected(h net.Host) {
	if !net.CompareHost(h, p.Peer) {
		return
	}
	msg := &localMessage{
		id:     1,
		pid:    p.ProtoID,
		sender: p.SelfHost,
	}
	_ = SendMessage(msg, p.Peer)
}

func (p *IntegrationProtocol) OnSessionDisconnected(h net.Host) {}

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
