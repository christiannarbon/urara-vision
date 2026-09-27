// Package httpapi is the chat service's HTTP surface.
package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"urara-vision/backend/internal/chat/config"
)

type Deps struct {
	Settings *config.Settings
	Log      *slog.Logger
}

type Server struct {
	settings *config.Settings
	log      *slog.Logger
}

func New(deps Deps) *Server {
	return &Server{settings: deps.Settings, log: deps.Log}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	// FastAPI's own bodies for these.
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusNotFound, map[string]any{"detail": "Not Found"})
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusMethodNotAllowed, map[string]any{"detail": "Method Not Allowed"})
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
