package protocol

import (
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

const (
	PingMessageID = 1
	PongMessageID = 2
)

type (
	PingMessage struct {
		messageID  int
		protocolID int
		sender     net.Host
		serializer runtime.Serializer
	}

	PongMessage struct {
		messageID  int
		protocolID int
		sender     net.Host
		serializer runtime.Serializer
	}

	PingSerializer struct{}
	PongSerializer struct{}
)

func NewPingMessage(sender net.Host) *PingMessage {
	return &PingMessage{
		messageID:  PingMessageID,
		protocolID: PingPongProtocolId,
		sender:     sender,
		serializer: &PingSerializer{},
	}
}

func NewPongMessage(sender net.Host) *PongMessage {
	return &PongMessage{
		messageID:  PongMessageID,
		protocolID: PingPongProtocolId,
		sender:     sender,
		serializer: &PongSerializer{},
	}
}

func (p *PingMessage) MessageID() int {
	return p.messageID
}

func (p *PingMessage) ProtocolID() int {
	return p.protocolID
}

func (p *PingMessage) Sender() net.Host {
	return p.sender
}

func (p *PingMessage) Serializer() runtime.Serializer {
	return p.serializer
}

func (p *PingMessage) SetSender(h net.Host) {
	p.sender = h
}

func (p *PongMessage) MessageID() int {
	return p.messageID
}

func (p *PongMessage) ProtocolID() int {
	return p.protocolID
}

func (p *PongMessage) Sender() net.Host {
	return p.sender
}

func (p *PongMessage) Serializer() runtime.Serializer {
	return p.serializer
}

func (p *PongMessage) SetSender(h net.Host) {
	p.sender = h
}

func (p *PingSerializer) Serialize() ([]byte, error) {
	return nil, nil
}

func (p *PingSerializer) Deserialize(data []byte) (runtime.Message, error) {
	return &PingMessage{
		messageID:  PingMessageID,
		protocolID: PingPongProtocolId,
	}, nil
}

func (p *PongSerializer) Serialize() ([]byte, error) {
	return nil, nil
}

func (p *PongSerializer) Deserialize(data []byte) (runtime.Message, error) {
	return &PongMessage{
		messageID:  PongMessageID,
		protocolID: PingPongProtocolId,
	}, nil
}
