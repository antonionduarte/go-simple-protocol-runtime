package runtime

import (
	"context"
	"log/slog"
)

// componentFilterHandler wraps another slog.Handler and only forwards records
// whose "component" attribute is in the allowed set.
type componentFilterHandler struct {
	next    slog.Handler
	allowed map[string]struct{}
}

// NewComponentFilterHandler creates a handler that drops all log records whose
// "component" attr is not one of the provided values. Records without a
// "component" attr are also dropped.
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
	keep := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "component" {
			if a.Value.Kind() == slog.KindString {
				if _, ok := h.allowed[a.Value.String()]; ok {
					keep = true
				}
			}
			// stop iteration once we inspect the component attr
			return false
		}
		return true
	})
	if !keep {
		return nil
	}
	return h.next.Handle(ctx, r)
}

func (h *componentFilterHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &componentFilterHandler{
		next:    h.next.WithAttrs(attrs),
		allowed: h.allowed,
	}
}

func (h *componentFilterHandler) WithGroup(name string) slog.Handler {
	return &componentFilterHandler{
		next:    h.next.WithGroup(name),
		allowed: h.allowed,
	}
}

// ParseLogLevel converts a string into a slog.Level. Unknown values default to info.
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
