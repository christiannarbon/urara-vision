// Working out who is calling.
package api

import (
	"context"
	"errors"
	"net/http"

	"urara-vision/backend/internal/auth"
	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/store/postgres"
)

const sessionCookie = "urara_session"

type fromCookieKey struct{}

// Authenticated chains authenticate and requireCSRFHeader, the order Routes uses.
func (s *Server) Authenticated(next http.Handler) http.Handler {
	return s.authenticate(s.requireCSRFHeader(next))
}

// authenticate puts exactly one Principal in the context, or answers 401.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AuthDisabled {
			next.ServeHTTP(w, withPrincipal(r, auth.Principal{Kind: auth.KindAnonymous}, false))
			return
		}
		// A CORS preflight carries no credentials; the CORS handler answers it.
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		validBearer := s.cfg.APIToken != "" && tokenMatches(bearerToken(r), s.cfg.APIToken)
		// A wrong bearer is refused even alongside a valid cookie.
		if r.Header.Get("Authorization") != "" && !validBearer {
			unauthorized(w, "unauthorized: invalid bearer token")
			return
		}

		if c, err := r.Cookie(sessionCookie); err == nil {
			u, err := s.pg.SessionUser(r.Context(), auth.HashToken(c.Value))
			if errors.Is(err, postgres.ErrNotFound) {
				s.clearSessionCookie(w)
				unauthorized(w, "not signed in")
				return
			}
			if err != nil {
				s.fail(w, r, err)
				return
			}
			next.ServeHTTP(w, withPrincipal(r, userPrincipal(u), true))
			return
		}

		if !validBearer {
			unauthorized(w, "not signed in")
			return
		}
		// X-Acting-User is trusted only behind a valid bearer token.
		if id := r.Header.Get("X-Acting-User"); id != "" {
			u, err := s.pg.GetUser(r.Context(), id)
			if errors.Is(err, postgres.ErrNotFound) {
				unauthorized(w, "acting user not found")
				return
			}
			if err != nil {
				s.fail(w, r, err)
				return
			}
			next.ServeHTTP(w, withPrincipal(r, userPrincipal(u), false))
			return
		}
		next.ServeHTTP(w, withPrincipal(r, auth.Principal{Kind: auth.KindService}, false))
	})
}

// requireCSRFHeader: a cross-site form cannot set custom headers, so requiring
// one on cookie-authenticated writes blocks CSRF. Bearer calls are exempt.
func (s *Server) requireCSRFHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			fromCookie, _ := r.Context().Value(fromCookieKey{}).(bool)
			if fromCookie && r.Header.Get("X-Requested-With") != "urara" {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing X-Requested-With header"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func withPrincipal(r *http.Request, p auth.Principal, fromCookie bool) *http.Request {
	ctx := auth.WithPrincipal(r.Context(), p)
	ctx = context.WithValue(ctx, fromCookieKey{}, fromCookie)
	return r.WithContext(ctx)
}

func userPrincipal(u *model.User) auth.Principal {
	return auth.Principal{
		Kind:        auth.KindUser,
		UserID:      u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Role:        auth.Role(u.Role),
	}
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func unauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="urara-vision"`)
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": msg})
}
