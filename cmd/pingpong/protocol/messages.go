package protocol

import "github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime"

// PingMessage and PongMessage are pure fixed-size payloads. Because
// runtime.BaseMessage is a zero-byte marker, encoding/binary can size
// these structs and runtime.BinaryCodec[*M] handles encode/decode for
// us — no manual codec needed.
type (
	PingMessage struct {
		runtime.BaseMessage
		Seq uint64
	}

	PongMessage struct {
		runtime.BaseMessage
		Seq uint64
	}
)

func NewPingMessage(seq uint64) *PingMessage { return &PingMessage{Seq: seq} }
func NewPongMessage(seq uint64) *PongMessage { return &PongMessage{Seq: seq} }
