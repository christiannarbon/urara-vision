package httpapi

import (
	"net/http"
	"strings"

	"urara-vision/backend/internal/chat/reqctx"
)

// nginx sets this after checking the session; no session or role checks here.
const userIDHeader = "X-User-Id"

func (s *Server) identity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get(userIDHeader))
		if id == "" {
			WriteError(w, r, http.StatusUnauthorized, map[string]any{"error": MsgNotSignedIn})
			return
		}
		next.ServeHTTP(w, r.WithContext(reqctx.WithUserID(r.Context(), id)))
	})
}
