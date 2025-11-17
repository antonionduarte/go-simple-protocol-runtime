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

var global *Config

func SetGlobalConfig(cfg *Config) {
	global = cfg
}

func GlobalConfig() *Config {
	return global
}

const (
	defaultRuntimeMsgTimerBuffer = 1
	defaultTransportOutBuffer    = 10
	defaultSessionEventsBuffer   = 0
	defaultSessionMessagesBuffer = 0
)

// RuntimeMsgTimerBuffer returns the buffer size for the runtime's internal
// msg/timer channels, falling back to a sensible default if unset.
func RuntimeMsgTimerBuffer() int {
	if global != nil && global.Runtime.ChannelBuffer > 0 {
		return global.Runtime.ChannelBuffer
	}
	return defaultRuntimeMsgTimerBuffer
}

// TransportOutBuffer returns the buffer size for transport-layer outward
// channels (outChannel, outTransportEvents), falling back to defaults.
func TransportOutBuffer() int {
	if global != nil && global.Runtime.ChannelBuffer > 0 {
		return global.Runtime.ChannelBuffer
	}
	return defaultTransportOutBuffer
}

// SessionEventsBuffer returns the buffer size for session-layer event
// channels, falling back to defaults.
func SessionEventsBuffer() int {
	if global != nil && global.Runtime.ChannelBuffer > 0 {
		return global.Runtime.ChannelBuffer
	}
	return defaultSessionEventsBuffer
}

// SessionMessagesBuffer returns the buffer size for session-layer message
// channels, falling back to defaults.
func SessionMessagesBuffer() int {
	if global != nil && global.Runtime.ChannelBuffer > 0 {
		return global.Runtime.ChannelBuffer
	}
	return defaultSessionMessagesBuffer
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
