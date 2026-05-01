package config

import (
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// LoggingConfig controls how logs are emitted.
type LoggingConfig struct {
	// Level is one of: "debug", "info", "warn", "error".
	Level string `yaml:"level"`
	// Components is an optional list of components to include:
	// "runtime", "session", "transport", "protocol".
	// If empty, all components are logged.
	Components []string `yaml:"components"`
	// Format controls the handler type: "text" or "json".
	// Defaults to "text".
	Format string `yaml:"format"`
}

// HostConfig is a simple host (ip:port) specification used in configs.
type HostConfig struct {
	IP   string `yaml:"ip"`
	Port int    `yaml:"port"`
}

// RuntimeConfig holds basic runtime/network configuration for examples.
// ChannelBuffer, if > 0, is used as a default buffer size for selected
// channels (e.g. outgoing message/event channels) in the example.
type RuntimeConfig struct {
	Self          HostConfig `yaml:"self"`
	Peer          HostConfig `yaml:"peer"`
	ChannelBuffer int        `yaml:"channelBuffer"`
}

// Config is the top-level YAML configuration structure.
type Config struct {
	Logging LoggingConfig `yaml:"logging"`
	Runtime RuntimeConfig `yaml:"runtime"`
}

// Default channel buffer sizes. Callers that don't have a specific value in
// mind can use these directly, or pass a user-provided value through one of
// the *BufferOr helpers below to fall back to a default when the user value
// is zero.
const (
	DefaultRuntimeMsgTimerBuffer = 16
	DefaultTransportOutBuffer    = 16
	DefaultSessionEventsBuffer   = 16
	DefaultSessionMessagesBuffer = 16
)

// RuntimeMsgTimerBufferOr returns the runtime msg/timer channel buffer,
// preferring the caller-provided value when non-zero and otherwise falling
// back to DefaultRuntimeMsgTimerBuffer.
func RuntimeMsgTimerBufferOr(n int) int {
	if n > 0 {
		return n
	}
	return DefaultRuntimeMsgTimerBuffer
}

// TransportOutBufferOr returns the transport out-channel buffer, preferring
// the caller-provided value when non-zero.
func TransportOutBufferOr(n int) int {
	if n > 0 {
		return n
	}
	return DefaultTransportOutBuffer
}

// SessionEventsBufferOr returns the session events buffer, preferring the
// caller-provided value when non-zero.
func SessionEventsBufferOr(n int) int {
	if n > 0 {
		return n
	}
	return DefaultSessionEventsBuffer
}

// SessionMessagesBufferOr returns the session messages buffer, preferring
// the caller-provided value when non-zero.
func SessionMessagesBufferOr(n int) int {
	if n > 0 {
		return n
	}
	return DefaultSessionMessagesBuffer
}

func defaultLoggingConfig() LoggingConfig {
	return LoggingConfig{
		Level:      "info",
		Components: []string{"runtime", "session", "transport", "protocol"},
		Format:     "text",
	}
}

// applyDefaults fills in any zero-valued fields with sensible defaults.
func (c *Config) applyDefaults() {
	def := defaultLoggingConfig()
	if c.Logging.Level == "" {
		c.Logging.Level = def.Level
	}
	if len(c.Logging.Components) == 0 {
		c.Logging.Components = def.Components
	}
	if c.Logging.Format == "" {
		c.Logging.Format = def.Format
	}
}

// LoadConfig reads a YAML configuration file from path and returns the
// populated Config with defaults applied.
func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	cfg.applyDefaults()
	return &cfg, nil
}
