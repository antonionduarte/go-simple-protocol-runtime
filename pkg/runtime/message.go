package runtime

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

// Message is the minimal interface that every protocol message must satisfy.
// The wire identifier is derived from the concrete type (see wireid.go);
// no manual ID is required.
type Message interface {
	Sender() net.Host
}

// BaseMessage is an optional embeddable struct that satisfies the Sender()
// requirement and exposes a SetSender method used by the runtime to inject
// the sender host on the receive side. Embed it to avoid writing the
// boilerplate yourself:
//
//	type Ping struct {
//	    runtime.BaseMessage
//	    Seq uint64
//	}
type BaseMessage struct {
	From net.Host
}

func (b BaseMessage) Sender() net.Host        { return b.From }
func (b *BaseMessage) SetSender(h net.Host)   { b.From = h }

// sendMessage encodes the message using its registered codec, prepends the
// 8-byte little-endian wire identifier, and hands the buffer to the session
// layer. Returns an error if no codec is registered for the message type.
func (r *Runtime) sendMessage(msg Message, sendTo net.Host) error {
	logger := r.Logger()

	wireID := wireIDOf(msg)
	proto, ok := r.codecLookup[wireID]
	if !ok {
		return fmt.Errorf("sendMessage: no codec registered for message type %T (wireID=%#x)", msg, wireID)
	}
	c, ok := proto.codecs[wireID]
	if !ok {
		return fmt.Errorf("sendMessage: codec lookup raced (wireID=%#x)", wireID)
	}

	payload, err := c.encode(msg)
	if err != nil {
		logger.Error("failed to encode message",
			"type", fmt.Sprintf("%T", msg),
			"to", sendTo.ToString(),
			"err", err,
		)
		return err
	}

	buffer := new(bytes.Buffer)
	if err := binary.Write(buffer, binary.LittleEndian, wireID); err != nil {
		logger.Error("failed to write wireID header",
			"type", fmt.Sprintf("%T", msg),
			"to", sendTo.ToString(),
			"err", err,
		)
		return err
	}
	buffer.Write(payload)

	r.sessionLayer.Send(*buffer, sendTo)
	return nil
}
