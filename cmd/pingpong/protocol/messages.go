package protocol

import (
	"encoding/binary"
	"fmt"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

type (
	PingMessage struct {
		runtime.BaseMessage
		Seq uint64
	}

	PongMessage struct {
		runtime.BaseMessage
		Seq uint64
	}

	// PingCodec / PongCodec are manual Codec[M] implementations: they
	// encode and decode the Seq field as 8 little-endian bytes. They are
	// kept manual (rather than using runtime.BinaryCodec[*M]) because the
	// embedded BaseMessage carries a string-typed Host that
	// encoding/binary can't size — typical of messages that want sender
	// info logged. See codec_test.go for a BinaryCodec round-trip on a
	// pure fixed-size message.
	PingCodec struct{}
	PongCodec struct{}
)

func NewPingMessage(sender net.Host, seq uint64) *PingMessage {
	return &PingMessage{BaseMessage: runtime.BaseMessage{From: sender}, Seq: seq}
}

func NewPongMessage(sender net.Host, seq uint64) *PongMessage {
	return &PongMessage{BaseMessage: runtime.BaseMessage{From: sender}, Seq: seq}
}

func (PingCodec) Encode(p *PingMessage) ([]byte, error) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, p.Seq)
	return b, nil
}

func (PingCodec) Decode(b []byte) (*PingMessage, error) {
	if len(b) < 8 {
		return nil, fmt.Errorf("PingCodec.Decode: payload too short (%d bytes, need 8)", len(b))
	}
	return &PingMessage{Seq: binary.LittleEndian.Uint64(b[:8])}, nil
}

func (PongCodec) Encode(p *PongMessage) ([]byte, error) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, p.Seq)
	return b, nil
}

func (PongCodec) Decode(b []byte) (*PongMessage, error) {
	if len(b) < 8 {
		return nil, fmt.Errorf("PongCodec.Decode: payload too short (%d bytes, need 8)", len(b))
	}
	return &PongMessage{Seq: binary.LittleEndian.Uint64(b[:8])}, nil
}
