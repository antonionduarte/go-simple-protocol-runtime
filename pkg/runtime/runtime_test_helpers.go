package runtime

import (
	"sync/atomic"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

// MockProtocol is a stub Protocol used in tests. It does no work itself —
// tests mostly care that Start/Init were called and that the protocol can
// be wrapped, registered, and torn down through the runtime APIs.
type MockProtocol struct {
	StartCalled bool
	InitCalled  bool

	HandledMessages []Message
}

func (m *MockProtocol) Start(_ ProtocolContext) { m.StartCalled = true }
func (m *MockProtocol) Init(_ ProtocolContext)  { m.InitCalled = true }

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

func (m *MockNetworkLayer) Connect(_ net.Host)                              { m.ConnectCalled = true }
func (m *MockNetworkLayer) Disconnect(_ net.Host)                           { m.DisconnectCalled = true }
func (m *MockNetworkLayer) Send(_ net.TransportMessage, _ net.Host)         { m.SendCalled = true }
func (m *MockNetworkLayer) OutChannel() chan net.TransportMessage           { return m.outChannel }
func (m *MockNetworkLayer) OutTransportEvents() chan net.TransportEvent     { return m.outTransportEvents }
func (m *MockNetworkLayer) Cancel()                                         { m.CancelCalled = true }

// localMessage is the canonical test message: just a sender, no payload.
// It uses BaseMessage so Sender/SetSender are inherited.
type localMessage struct {
	BaseMessage
}

var _ Message = (*localMessage)(nil)

// localCodec is a no-payload Codec[*localMessage] used by tests that don't
// care about message contents, only routing.
type localCodec struct{}

func (localCodec) Encode(_ *localMessage) ([]byte, error)      { return nil, nil }
func (localCodec) Decode(_ []byte) (*localMessage, error)      { return &localMessage{}, nil }

// failingCodec returns the supplied error from Encode and Decode. Used to
// test that the runtime propagates codec errors instead of swallowing them.
type failingCodec struct{}

func (failingCodec) Encode(_ *failingMessageBM) ([]byte, error) { return nil, assertError{} }
func (failingCodec) Decode(_ []byte) (*failingMessageBM, error) { return nil, assertError{} }

type failingMessageBM struct{ BaseMessage }

// assertError is a sentinel error used by failing test codecs.
type assertError struct{}

func (assertError) Error() string { return "expected codec error" }

// twoSidedProtocol is used by the two-runtime integration test. It
// connects to its peer on Init, replies once on inbound, and records
// receipt for the test to observe.
type twoSidedProtocol struct {
	Peer net.Host

	ctx      ProtocolContext
	received atomic.Bool
}

func (p *twoSidedProtocol) Start(ctx ProtocolContext) {
	p.ctx = ctx
	RegisterCodec[*localMessage](ctx, localCodec{})
	RegisterHandler[*localMessage](ctx, p.handle)
}

func (p *twoSidedProtocol) Init(ctx ProtocolContext) {
	p.ctx = ctx
	ctx.Connect(p.Peer)
}

func (p *twoSidedProtocol) OnSessionConnected(h net.Host) {
	if !net.CompareHost(h, p.Peer) || p.ctx == nil {
		return
	}
	_ = p.ctx.Send(&localMessage{}, p.Peer)
}

func (p *twoSidedProtocol) OnSessionDisconnected(_ net.Host) {}

func (p *twoSidedProtocol) handle(_ *localMessage) {
	p.received.Store(true)
}
