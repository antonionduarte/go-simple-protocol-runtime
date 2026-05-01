package runtime

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
)

// Codec is the public typed codec interface a protocol author implements
// (or supplies via a generic helper like BinaryCodec[M]) to encode and
// decode a concrete Message type.
//
// Implementations must obey the following contract:
//   - Encode MUST NOT mutate the Message instance it receives.
//   - Decode MUST return a new Message instance of the correct concrete
//     type for the given wire identifier.
type Codec[M Message] interface {
	Encode(m M) ([]byte, error)
	Decode(data []byte) (M, error)
}

// codec is the type-erased internal form. RegisterCodec[M] adapts a
// Codec[M] into a codec for storage in proto.codecs.
type codec interface {
	encode(Message) ([]byte, error)
	decode([]byte) (Message, error)
}

// codecAdapter[M] bridges a typed Codec[M] to the runtime's untyped
// internal codec interface.
type codecAdapter[M Message] struct {
	c Codec[M]
}

func (a codecAdapter[M]) encode(m Message) ([]byte, error) {
	typed, ok := m.(M)
	if !ok {
		return nil, fmt.Errorf("runtime: codec mismatch: expected %T, got %T", *new(M), m)
	}
	return a.c.Encode(typed)
}

func (a codecAdapter[M]) decode(data []byte) (Message, error) {
	return a.c.Decode(data)
}

// RegisterCodec registers a Codec[M] under M's wire identifier on the
// supplied ProtocolContext. The wire identifier is derived from M's Go
// type name (or M.WireName() if implemented).
func RegisterCodec[M Message](ctx ProtocolContext, c Codec[M]) {
	ctx.registerCodec(WireID[M](), codecAdapter[M]{c: c})
}

// RegisterHandler registers fn as the handler for messages of type M on
// the supplied ProtocolContext. The wire identifier is derived from M's
// Go type name (or M.WireName() if implemented). The framework performs
// the type assertion before invoking fn.
func RegisterHandler[M Message](ctx ProtocolContext, fn func(M)) {
	ctx.registerHandler(WireID[M](), func(raw Message) {
		fn(raw.(M))
	})
}

// BinaryCodec is a Codec[M] that encodes M with encoding/binary using
// little-endian byte order. It works for any M whose fields are
// fixed-size types (boolN/intN/uintN/floatN/complex/fixed arrays/structs
// of same). Variable-length fields (string, slice, int/uint, map,
// interface, pointer) are not supported by encoding/binary and require a
// custom Codec built on the wire helpers.
//
// M must be a pointer to a struct: encoding/binary needs a non-nil
// pointer for both Read and Write. BinaryCodec.Decode allocates a fresh
// value via reflect.New(t.Elem()).
type BinaryCodec[M Message] struct{}

func (BinaryCodec[M]) Encode(m M) ([]byte, error) {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, m); err != nil {
		return nil, fmt.Errorf("BinaryCodec.Encode: %w", err)
	}
	return buf.Bytes(), nil
}

func (BinaryCodec[M]) Decode(data []byte) (M, error) {
	var zero M
	t := reflect.TypeOf(zero)
	if t == nil || t.Kind() != reflect.Pointer {
		return zero, fmt.Errorf("BinaryCodec[M]: M must be a pointer type, got %T", zero)
	}
	ptr := reflect.New(t.Elem()).Interface().(M)
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, ptr); err != nil {
		return zero, fmt.Errorf("BinaryCodec.Decode: %w", err)
	}
	return ptr, nil
}
