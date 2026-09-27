package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"urara-vision/backend/internal/chat/reqctx"
)

var ErrChatDisabled = errors.New("chat is turned off")

// FeatureGate caches the backend's chat switch for everyone.
type FeatureGate struct {
	backend Backend
	ttl     time.Duration
	now     func() time.Time
	log     *slog.Logger

	mu      sync.RWMutex
	enabled *bool // nil until the first successful read
	expires time.Time
}

func NewFeatureGate(backend Backend, ttl time.Duration, now func() time.Time, log *slog.Logger) *FeatureGate {
	if now == nil {
		now = time.Now
	}
	return &FeatureGate{backend: backend, ttl: ttl, now: now, log: log}
}

// Require returns ErrChatDisabled when chat is switched off.
func (g *FeatureGate) Require(ctx context.Context) error {
	g.mu.RLock()
	fresh, enabled := g.now().Before(g.expires), g.enabled
	g.mu.RUnlock()

	if !fresh {
		g.mu.Lock()
		if !g.now().Before(g.expires) {
			g.refresh(ctx)
		}
		enabled = g.enabled
		g.mu.Unlock()
	}
	if enabled != nil && !*enabled {
		return ErrChatDisabled
	}
	return nil
}

// refresh runs with g.mu held, so one caller reads the backend at a time.
func (g *FeatureGate) refresh(ctx context.Context) {
	// As the service: the answer is shared. Uncancelled, so one caller leaving
	// does not fail the read for everyone waiting on it.
	ctx = reqctx.WithUserID(context.WithoutCancel(ctx), "")
	features, err := g.backend.Features(ctx)
	if err != nil {
		// Keep the last answer; with none yet, chat stays allowed.
		g.log.Warn("could not read backend features", "error", err.Error())
	} else {
		on := features.Chat.Enabled
		g.enabled = &on
	}
	g.expires = g.now().Add(g.ttl)
}

func (s *Server) requireChat(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.gate != nil && s.gate.Require(r.Context()) != nil {
			WriteError(w, r, http.StatusServiceUnavailable, map[string]any{"error": MsgChatTurnedOff})
			return
		}
		next.ServeHTTP(w, r)
	})
}
