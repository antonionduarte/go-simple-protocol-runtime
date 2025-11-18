package runtime

import (
	"context"
	"log/slog"
	"os"

	rtconfig "github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/config"
)

type componentFilterHandler struct {
	next      slog.Handler
	allowed   map[string]struct{}
	component string // logger-level component, if set via WithAttrs
}

func NewComponentFilterHandler(next slog.Handler, allowedComponents []string) slog.Handler {
	allowed := make(map[string]struct{}, len(allowedComponents))
	for _, c := range allowedComponents {
		allowed[c] = struct{}{}
	}
	return &componentFilterHandler{
		next:    next,
		allowed: allowed,
	}
}

func (h *componentFilterHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *componentFilterHandler) Handle(ctx context.Context, r slog.Record) error {
	// If this logger has a fixed component (set via WithAttrs), decide based
	// solely based on that value.
	if h.component != "" {
		if _, ok := h.allowed[h.component]; !ok {
			return nil
		}
		return h.next.Handle(ctx, r)
	}

	// Otherwise, fall back to inspecting record-level attributes. This allows
	// callers to pass "component" explicitly in individual log calls.
	hasComponent := false
	allowed := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "component" {
			if a.Value.Kind() == slog.KindString {
				hasComponent = true
				if _, ok := h.allowed[a.Value.String()]; ok {
					allowed = true
				}
			}
			return false
		}
		return true
	})
	if hasComponent && !allowed {
		return nil
	}
	return h.next.Handle(ctx, r)
}

func (h *componentFilterHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Detect if this logger is being enriched with a logger-level component
	// attribute. If so, capture it so Handle can filter on it even though
	// it is not present in individual Records.
	component := h.component
	for _, a := range attrs {
		if a.Key == "component" && a.Value.Kind() == slog.KindString {
			component = a.Value.String()
			break
		}
	}
	return &componentFilterHandler{
		next:      h.next.WithAttrs(attrs),
		allowed:   h.allowed,
		component: component,
	}
}

func (h *componentFilterHandler) WithGroup(name string) slog.Handler {
	return &componentFilterHandler{
		next:      h.next.WithGroup(name),
		allowed:   h.allowed,
		component: h.component,
	}
}

func ParseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info":
		fallthrough
	default:
		return slog.LevelInfo
	}
}

func NewLoggerFromConfig(cfg rtconfig.LoggingConfig) *slog.Logger {
	level := ParseLogLevel(cfg.Level)

	var handler slog.Handler
	switch cfg.Format {
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		})
	case "text":
		fallthrough
	default:
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		})
	}

	if len(cfg.Components) > 0 {
		handler = NewComponentFilterHandler(handler, cfg.Components)
	}

	return slog.New(handler)
}
