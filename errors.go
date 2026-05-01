package protorun

import "errors"

// Public sentinel errors. The runtime returns these (or wraps them with
// fmt.Errorf("...: %w", Err...)) so callers can use errors.Is to test
// for specific failure conditions instead of string-matching.
var (
	// ErrNoSessionLayer is returned by Connect/Disconnect/ConnectWithRetry
	// when no SessionLayer has been registered (typically because
	// WithTCPTransport was not supplied to runtime.New).
	ErrNoSessionLayer = errors.New("protorun: session layer not registered")

	// ErrNoNetworkLayer is returned by Run when no transport.Layer has
	// been registered.
	ErrNoNetworkLayer = errors.New("protorun: network layer not registered")

	// ErrAlreadyCancelled is returned by Run when Cancel has already
	// been called on the runtime instance.
	ErrAlreadyCancelled = errors.New("protorun: cannot start a runtime that has been cancelled")

	// ErrNoCodec is returned by Send when no Codec has been registered
	// for the message type's wire identifier.
	ErrNoCodec = errors.New("protorun: no codec registered for message type")
)
