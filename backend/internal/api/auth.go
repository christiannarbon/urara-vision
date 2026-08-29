// Bearer-token authentication for the API surface.
//
// The token is a single shared secret rather than per-user credentials: this is
// an internal tool with no user model, and a shared token is the smallest thing
// that stops an exposed ingress from being world-writable. Anything needing
// real identity belongs at the ingress, in front of this.
package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

// requireToken rejects requests that do not carry the configured bearer token.
//
// It is a no-op when APIToken is empty, which is what keeps `docker compose up`
// and the test suite working with no configuration. config.Load refuses a token
// short enough to guess, so an empty token is the only unauthenticated mode and
// it is one the operator has to choose explicitly.
func (s *Server) requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.APIToken == "" {
			next.ServeHTTP(w, r)
			return
		}
		// A CORS preflight never carries Authorization -- the browser sends it
		// before it knows whether the request is allowed -- so it has to pass
		// through to the CORS handler, which answers it without reaching a
		// route.
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if !tokenMatches(bearerToken(r), s.cfg.APIToken) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="urara-vision"`)
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "unauthorized: supply the API token as 'Authorization: Bearer <token>'",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// bearerToken pulls the credential out of an Authorization header. Only the
// header is accepted; a query parameter would end up in access logs and browser
// history.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) < len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

// tokenMatches compares in constant time. Both sides are hashed first so the
// comparison is over two fixed-length values and the timing carries nothing
// about the expected token's length.
func tokenMatches(got, want string) bool {
	g := sha256.Sum256([]byte(got))
	w := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(g[:], w[:]) == 1
}
