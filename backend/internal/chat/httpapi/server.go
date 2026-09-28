// Package httpapi is the chat service's HTTP surface.
package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/config"
	"urara-vision/backend/internal/chat/llm"
	"urara-vision/backend/internal/model"
)

// Backend is the part of apiclient.Client the handlers call.
type Backend interface {
	Health(ctx context.Context) bool
	Features(ctx context.Context) (apiclient.Features, error)
	CreateConversation(ctx context.Context, snapshotID, title string) (model.Conversation, error)
	ListConversations(ctx context.Context, snapshotID string, limit int) ([]model.Conversation, error)
	GetConversation(ctx context.Context, cid string) (model.Conversation, error)
	DeleteConversation(ctx context.Context, cid string) error
}

type Deps struct {
	Settings     *config.Settings
	Log          *slog.Logger
	Backend      Backend
	Model        llm.Model
	Tools        ToolBackend
	Agent        Agent
	ProbeTimeout time.Duration    // /debug/llm; 0: 15s
	Clock        func() time.Time // nil: time.Now
}

type Server struct {
	settings     *config.Settings
	log          *slog.Logger
	backend      Backend
	model        llm.Model
	tools        ToolBackend
	agent        Agent
	probeTimeout time.Duration
	gate         *FeatureGate // nil without a backend, so tests of the shell run ungated
	limiter      *TurnLimiter
}

func New(deps Deps) *Server {
	s := &Server{
		settings: deps.Settings, log: deps.Log, backend: deps.Backend,
		model: deps.Model, tools: deps.Tools, agent: deps.Agent, probeTimeout: deps.ProbeTimeout,
		limiter: NewTurnLimiter(deps.Settings.MaxConcurrentTurns, deps.Settings.TurnAdmissionWait),
	}
	if s.probeTimeout == 0 {
		s.probeTimeout = defaultProbeTimeout
	}
	if deps.Backend != nil {
		s.gate = NewFeatureGate(deps.Backend, deps.Settings.FeaturesCache, deps.Clock, deps.Log)
	}
	return s
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	// FastAPI's own bodies for these.
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusNotFound, map[string]any{"detail": "Not Found"})
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusMethodNotAllowed, map[string]any{"detail": "Method Not Allowed"})
	})

	// Probes and /debug/* are outside identity and the gate.
	r.Get("/healthz", s.healthz)
	r.Get("/readyz", s.readyz)
	r.Get("/debug/llm", s.debugLLM)
	r.Get("/debug/tools", s.debugTools)
	r.Post("/debug/tool", s.debugTool)
	r.Post("/debug/answer", s.debugAnswer)

	// Identity first, so an anonymous caller cannot learn whether chat is on.
	r.Route("/api/chat", func(r chi.Router) {
		r.Use(s.identity, s.requireChat)
		r.Post("/conversations", s.createConversation)
		r.Get("/conversations", s.listConversations)
		r.Get("/conversations/{cid}", s.getConversation)
		r.Delete("/conversations/{cid}", s.deleteConversation)
		r.Post("/answer", s.chatAnswer)
	})
	return s.Wrap(r)
}

// Wrap applies the middleware every route runs under. Request ID is outermost,
// so a 413 or a recovered panic still carries it.
func (s *Server) Wrap(h http.Handler) http.Handler {
	return s.requestID(s.limitBody(s.recoverer(h)))
}

func (s *Server) writeTooLarge(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, http.StatusRequestEntityTooLarge, map[string]any{
		"detail": fmt.Sprintf("request body is larger than the %d byte limit", s.settings.MaxRequestBytes),
	})
}

// limitBody refuses a declared oversized body before reading it, and bounds
// the read for a missing or lying Content-Length.
func (s *Server) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > s.settings.MaxRequestBytes {
			s.writeTooLarge(w, r)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, s.settings.MaxRequestBytes)
		next.ServeHTTP(w, r)
	})
}
