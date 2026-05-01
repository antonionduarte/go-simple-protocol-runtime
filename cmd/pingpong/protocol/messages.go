package protocol

import (
	"encoding/binary"
	"fmt"

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
		seq        uint64
		serializer runtime.Serializer
	}

	PongMessage struct {
		messageID  int
		protocolID int
		sender     net.Host
		seq        uint64
		serializer runtime.Serializer
	}

	PingSerializer struct{}
	PongSerializer struct{}
)

func NewPingMessage(sender net.Host, seq uint64) *PingMessage {
	return &PingMessage{
		messageID:  PingMessageID,
		protocolID: PingPongProtocolId,
		sender:     sender,
		seq:        seq,
		serializer: &PingSerializer{},
	}
}

func NewPongMessage(sender net.Host, seq uint64) *PongMessage {
	return &PongMessage{
		messageID:  PongMessageID,
		protocolID: PingPongProtocolId,
		sender:     sender,
		seq:        seq,
		serializer: &PongSerializer{},
	}
}

func (p *PingMessage) MessageID() int                 { return p.messageID }
func (p *PingMessage) ProtocolID() int                { return p.protocolID }
func (p *PingMessage) Sender() net.Host               { return p.sender }
func (p *PingMessage) Seq() uint64                    { return p.seq }
func (p *PingMessage) Serializer() runtime.Serializer { return p.serializer }
func (p *PingMessage) SetSender(h net.Host)           { p.sender = h }

func (p *PongMessage) MessageID() int                 { return p.messageID }
func (p *PongMessage) ProtocolID() int                { return p.protocolID }
func (p *PongMessage) Sender() net.Host               { return p.sender }
func (p *PongMessage) Seq() uint64                    { return p.seq }
func (p *PongMessage) Serializer() runtime.Serializer { return p.serializer }
func (p *PongMessage) SetSender(h net.Host)           { p.sender = h }

// PingSerializer encodes the ping sequence number as a fixed 8-byte
// little-endian payload.
func (p *PingSerializer) Serialize(msg runtime.Message) ([]byte, error) {
	ping, ok := msg.(*PingMessage)
	if !ok {
		return nil, fmt.Errorf("PingSerializer.Serialize: expected *PingMessage, got %T", msg)
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, ping.seq)
	return buf, nil
}

func (p *PingSerializer) Deserialize(data []byte) (runtime.Message, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("PingSerializer.Deserialize: payload too short (%d bytes, need 8)", len(data))
	}
	return &PingMessage{
		messageID:  PingMessageID,
		protocolID: PingPongProtocolId,
		seq:        binary.LittleEndian.Uint64(data[:8]),
		serializer: &PingSerializer{},
	}, nil
}

// PongSerializer encodes the pong sequence number as a fixed 8-byte
// little-endian payload.
func (p *PongSerializer) Serialize(msg runtime.Message) ([]byte, error) {
	pong, ok := msg.(*PongMessage)
	if !ok {
		return nil, fmt.Errorf("PongSerializer.Serialize: expected *PongMessage, got %T", msg)
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, pong.seq)
	return buf, nil
}

func (p *PongSerializer) Deserialize(data []byte) (runtime.Message, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("PongSerializer.Deserialize: payload too short (%d bytes, need 8)", len(data))
	}
	return &PongMessage{
		messageID:  PongMessageID,
		protocolID: PingPongProtocolId,
		seq:        binary.LittleEndian.Uint64(data[:8]),
		serializer: &PongSerializer{},
	}, nil
}
