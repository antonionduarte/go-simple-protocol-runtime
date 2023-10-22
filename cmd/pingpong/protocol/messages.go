package protocol

import (
	"bytes"
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
		sender     *net.Host
		serializer runtime.Serializer
	}

	PongMessage struct {
		messageID  int
		protocolID int
		sender     *net.Host
		serializer runtime.Serializer
	}

	PingSerializer struct{}
	PongSerializer struct{}
)

func NewPingMessage(sender *net.Host) *PingMessage {
	return &PingMessage{
		messageID:  PingMessageID,
		protocolID: PingPongProtocolId,
		sender:     sender,
		serializer: &PingSerializer{},
	}
}

func NewPongMessage(sender *net.Host) *PongMessage {
	return &PongMessage{
		messageID:  PongMessageID,
		protocolID: PingPongProtocolId,
		sender:     sender,
		serializer: &PongSerializer{},
	}
}

/*----------- Mandatory Methods ----------- */

func (p *PingMessage) MessageID() int {
	return p.messageID
}

func (p *PingMessage) ProtocolID() int {
	return p.protocolID
}

func (p *PingMessage) Sender() *net.Host {
	return p.sender
}

func (p *PingMessage) Serializer() runtime.Serializer {
	return p.serializer
}

func (p *PongMessage) MessageID() int {
	return p.messageID
}

func (p *PongMessage) ProtocolID() int {
	return p.protocolID
}

func (p *PongMessage) Sender() *net.Host {
	return p.sender
}

func (p *PongMessage) Serializer() runtime.Serializer {
	return p.serializer
}

/*----------- Serializers ----------- */

func (p *PingSerializer) Serialize() (*bytes.Buffer, error) {
	return nil, nil
}

func (p *PingSerializer) Deserialize(buffer *bytes.Buffer) (runtime.Message, error) {
	return nil, nil
}

func (p *PongSerializer) Serialize() (*bytes.Buffer, error) {
	return nil, nil
}

func (p *PongSerializer) Deserialize(buffer *bytes.Buffer) (runtime.Message, error) {
	return nil, nil
}
