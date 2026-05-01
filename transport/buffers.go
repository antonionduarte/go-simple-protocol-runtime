package transport

// Per-layer default channel buffer sizes used when callers pass a
// non-positive value to the constructors. Kept private to this package;
// the runtime mirrors the same defaults under exported names in
// pkg/runtime/buffers.go for users who want to read or override them.
const (
	defaultTransportOutBuffer    = 16
	defaultSessionEventsBuffer   = 16
	defaultSessionMessagesBuffer = 16
)
