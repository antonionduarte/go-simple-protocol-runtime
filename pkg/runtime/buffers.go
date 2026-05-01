package runtime

// Default channel buffer sizes for the runtime, transport, and session
// layers. Callers that don't have a specific value in mind can either use
// these constants directly or pass a caller-provided value through the
// matching *BufferOr helper, which falls back to the default when given
// a non-positive value.
const (
	DefaultTransportOutBuffer    = 16
	DefaultSessionEventsBuffer   = 16
	DefaultSessionMessagesBuffer = 16
)

// TransportOutBufferOr returns the transport out-channel buffer size,
// preferring the caller-provided value when positive and otherwise
// falling back to DefaultTransportOutBuffer.
func TransportOutBufferOr(n int) int {
	if n > 0 {
		return n
	}
	return DefaultTransportOutBuffer
}

// SessionEventsBufferOr returns the session events-channel buffer size,
// preferring the caller-provided value when positive.
func SessionEventsBufferOr(n int) int {
	if n > 0 {
		return n
	}
	return DefaultSessionEventsBuffer
}

// SessionMessagesBufferOr returns the session messages-channel buffer
// size, preferring the caller-provided value when positive.
func SessionMessagesBufferOr(n int) int {
	if n > 0 {
		return n
	}
	return DefaultSessionMessagesBuffer
}
