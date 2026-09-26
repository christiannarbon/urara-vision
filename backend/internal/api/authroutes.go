// Login, logout, me, password change and the nginx session check.
package api

import (
	"errors"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"urara-vision/backend/internal/auth"
	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/store/postgres"
)

const errBadLogin = "invalid username or password"

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	// Login runs outside authenticate, so it needs its own check against login CSRF.
	if refuseWithoutCSRFHeader(w, r) {
		return
	}
	var req loginRequest
	if err := decodeBody(w, r, &req); err != nil {
		s.badRequest(w, err.Error())
		return
	}
	username, nameErr := auth.NormaliseUsername(req.Username)
	if nameErr != nil {
		username = strings.ToLower(req.Username)
	}
	userKey, ipKey := "user:"+username, "ip:"+clientIP(r)
	if !s.allowAttempt(w, userKey, ipKey) {
		return
	}

	// Unknown and invalid usernames cost a bcrypt compare too, so timing matches a wrong password.
	var u *model.User
	ok := false
	if nameErr != nil {
		auth.VerifyAgainstDummy(req.Password)
	} else {
		var hash string
		var err error
		u, hash, err = s.pg.PasswordIdentity(r.Context(), username)
		switch {
		case errors.Is(err, postgres.ErrNotFound):
			auth.VerifyAgainstDummy(req.Password)
		case err != nil:
			s.fail(w, r, err)
			return
		default:
			ok = auth.VerifyPassword(hash, req.Password)
		}
	}
	if !ok {
		s.limiter.Fail(userKey)
		s.limiter.Fail(ipKey)
		s.log.Info("login failed", "username", username, "request_id", middleware.GetReqID(r.Context()))
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": errBadLogin})
		return
	}
	s.limiter.Reset(userKey)

	// A new login replaces any session this browser already holds.
	if old, err := r.Cookie(sessionCookie); err == nil {
		if err := s.pg.DeleteSession(r.Context(), auth.HashToken(old.Value)); err != nil {
			s.log.Warn("delete previous session", "error", err, "request_id", middleware.GetReqID(r.Context()))
		}
	}

	token, hash, err := auth.NewSessionToken()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	expires := time.Now().Add(s.cfg.SessionTTL)
	if err := s.pg.CreateSession(r.Context(), hash, u.ID, expires); err != nil {
		s.fail(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	s.log.Info("login succeeded", "username", username, "request_id", middleware.GetReqID(r.Context()))
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

// allowAttempt answers 429 when either key is over its limit.
func (s *Server) allowAttempt(w http.ResponseWriter, keys ...string) bool {
	for _, k := range keys {
		if ok, wait := s.limiter.Allow(k); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many failed attempts, try again later"})
			return false
		}
	}
	return true
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if hash, ok := sessionHash(r); ok {
		if err := s.pg.DeleteSession(r.Context(), hash); err != nil {
			s.fail(w, r, err)
			return
		}
		s.clearSessionCookie(w)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	u, _ := r.Context().Value(userKey{}).(*model.User)
	perms := p.Permissions()
	if perms == nil {
		perms = []auth.Permission{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u, "kind": p.Kind, "permissions": perms})
}

type passwordRequest struct {
	Current string `json:"current"`
	New     string `json:"new"`
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	if p.Kind != auth.KindUser {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only a signed-in user can change a password"})
		return
	}
	var req passwordRequest
	if err := decodeBody(w, r, &req); err != nil {
		s.badRequest(w, err.Error())
		return
	}
	userKey, ipKey := "user:"+p.Username, "ip:"+clientIP(r)
	if !s.allowAttempt(w, userKey, ipKey) {
		return
	}
	_, hash, err := s.pg.PasswordIdentity(r.Context(), p.Username)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !auth.VerifyPassword(hash, req.Current) {
		s.limiter.Fail(userKey)
		s.limiter.Fail(ipKey)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "current password is incorrect"})
		return
	}
	newHash, err := auth.HashPassword(req.New)
	if errors.Is(err, auth.ErrPasswordPolicy) {
		s.badRequest(w, err.Error())
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.pg.SetPasswordHash(r.Context(), p.UserID, newHash); err != nil {
		s.fail(w, r, err)
		return
	}
	keep, _ := sessionHash(r)
	if err := s.pg.DeleteUserSessions(r.Context(), p.UserID, keep); err != nil {
		s.fail(w, r, err)
		return
	}
	s.limiter.Reset(userKey)
	w.WriteHeader(http.StatusNoContent)
}

// handleSession is for nginx auth_request: 204 with identity headers, or 401.
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	switch p.Kind {
	case auth.KindUser:
		w.Header().Set("X-User-Id", p.UserID)
		w.Header().Set("X-User-Role", string(p.Role))
	case auth.KindAnonymous:
		// AUTH_DISABLED: chat's X-Acting-User is ignored by a backend that is also disabled.
		w.Header().Set("X-User-Id", "anonymous")
		w.Header().Set("X-User-Role", string(auth.RoleAdmin))
	default:
		unauthorized(w, "not signed in")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sessionHash is the hash of the cookie that authenticated this request, if any.
func sessionHash(r *http.Request) (string, bool) {
	if fromCookie, _ := r.Context().Value(fromCookieKey{}).(bool); !fromCookie {
		return "", false
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", false
	}
	return auth.HashToken(c.Value), true
}

// clientIP falls back to the TCP peer for direct calls with a short XFF chain.
func clientIP(r *http.Request) string {
	if ip := middleware.GetClientIP(r.Context()); ip != "" {
		return ip
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
