package httpapi

import (
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"urara-vision/backend/internal/chat/reqctx"
)

const (
	requestIDHeader = "X-Request-Id"
	// Fits a UUID or chi's "host/random-000001", and no more.
	maxRequestIDLength = 64
)

// SanitiseRequestID makes an inbound ID safe to log, or mints one. Newlines
// matter most: they would let a caller forge log lines.
func SanitiseRequestID(raw string) string {
	runes := []rune(raw)
	if len(runes) > maxRequestIDLength {
		runes = runes[:maxRequestIDLength]
	}
	var b strings.Builder
	for _, r := range runes {
		if r < 128 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.:/", r)) {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	return b.String()
}

// statusWriter records the status and adds the request ID header unless a
// handler already set it.
type statusWriter struct {
	http.ResponseWriter
	id     string
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		if w.Header().Get(requestIDHeader) == "" {
			w.Header().Set(requestIDHeader, w.id)
		}
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Probes hit these on a timer; a success is logged at debug only.
var quietPaths = map[string]bool{"/healthz": true, "/readyz": true}

func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := SanitiseRequestID(r.Header.Get(requestIDHeader))
		sw := &statusWriter{ResponseWriter: w, id: id}
		started := time.Now()

		defer func() {
			status := sw.status
			if status == 0 {
				status = http.StatusOK
			}
			level := slog.LevelInfo
			if quietPaths[r.URL.Path] && status < 400 {
				level = slog.LevelDebug
			}
			ms := math.Round(float64(time.Since(started).Microseconds())/10) / 100
			s.log.Log(r.Context(), level, "request",
				"method", r.Method, "path", r.URL.Path, "status", status,
				"duration_ms", ms, "request_id", id)
		}()

		next.ServeHTTP(sw, r.WithContext(reqctx.WithRequestID(r.Context(), id)))
	})
}
