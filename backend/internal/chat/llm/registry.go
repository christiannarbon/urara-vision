package llm

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"urara-vision/backend/internal/chat/config"
)

type Factory func(ctx context.Context, s config.Settings, log *slog.Logger) (Model, error)

// Filled by cmd/chat via Register, so every adapter dependency is visible there. No init().
var factories = map[string]Factory{}

// Register panics on a duplicate name: that is a wiring bug.
func Register(name string, f Factory) {
	if _, ok := factories[name]; ok {
		panic("llm: provider registered twice: " + name)
	}
	factories[name] = f
}

// Names lists the registered providers, sorted.
func Names() []string {
	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func New(ctx context.Context, s config.Settings, log *slog.Logger) (Model, error) {
	f, ok := factories[s.LLMProvider]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q; expected one of %v", s.LLMProvider, Names())
	}
	return f(ctx, s, log)
}

// Describe names the configured model for /readyz and message meta, without calling it.
func Describe(s config.Settings) map[string]string {
	d := map[string]string{"provider": s.LLMProvider, "model": s.LLMModel}
	if strings.HasPrefix(s.LLMProvider, "vertex") {
		d["location"] = s.VertexLocation
	}
	return d
}
