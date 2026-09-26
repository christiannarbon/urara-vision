// What each /api/v1 route requires of the caller.
package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"urara-vision/backend/internal/auth"
)

// routePermissions maps "METHOD pattern" (as chi.Walk reports it) to what the caller needs.
var routePermissions = map[string]auth.Permission{
	"GET /api/v1/features": auth.PermProjectView,

	"GET /api/v1/snapshots":                     auth.PermProjectView,
	"GET /api/v1/snapshots/{sid}/":              auth.PermProjectView,
	"GET /api/v1/snapshots/{sid}/context":       auth.PermProjectView,
	"GET /api/v1/snapshots/{sid}/diagnostics":   auth.PermProjectView,
	"GET /api/v1/snapshots/{sid}/domains":       auth.PermProjectView,
	"GET /api/v1/snapshots/{sid}/graph":         auth.PermProjectView,
	"GET /api/v1/snapshots/{sid}/lineage":       auth.PermProjectView,
	"GET /api/v1/snapshots/{sid}/neighborhood":  auth.PermProjectView,
	"GET /api/v1/snapshots/{sid}/paths":         auth.PermProjectView,
	"GET /api/v1/snapshots/{sid}/search":        auth.PermProjectView,
	"GET /api/v1/snapshots/{sid}/sources":       auth.PermProjectView,
	"GET /api/v1/snapshots/{sid}/table":         auth.PermProjectView,
	"GET /api/v1/snapshots/{sid}/tables":        auth.PermProjectView,
	"GET /api/v1/snapshots/{sid}/tables/detail": auth.PermProjectView,
	"DELETE /api/v1/snapshots/{sid}/":           auth.PermProjectDelete,

	"GET /api/v1/projects":                                 auth.PermProjectView,
	"GET /api/v1/projects/{project}/":                      auth.PermProjectView,
	"GET /api/v1/projects/{project}/diff":                  auth.PermProjectView,
	"GET /api/v1/projects/{project}/versions":              auth.PermProjectView,
	"GET /api/v1/projects/{project}/versions/{version}":    auth.PermProjectView,
	"DELETE /api/v1/projects/{project}/":                   auth.PermProjectDelete,
	"DELETE /api/v1/projects/{project}/versions/{version}": auth.PermVersionDelete,

	"POST /api/v1/conversations/":               auth.PermChatUse,
	"GET /api/v1/conversations/":                auth.PermChatUse,
	"GET /api/v1/conversations/{cid}/":          auth.PermChatUse,
	"PATCH /api/v1/conversations/{cid}/":        auth.PermChatUse,
	"DELETE /api/v1/conversations/{cid}/":       auth.PermChatUse,
	"POST /api/v1/conversations/{cid}/messages": auth.PermChatUse,

	"PATCH /api/v1/settings": auth.PermSettingsManage,

	"GET /api/v1/users":                auth.PermUserManage,
	"POST /api/v1/users":               auth.PermUserManage,
	"PATCH /api/v1/users/{id}":         auth.PermUserManage,
	"POST /api/v1/users/{id}/password": auth.PermUserManage,
	"DELETE /api/v1/users/{id}":        auth.PermUserDelete,
}

// publicRoutes need only a principal, or nothing at all.
var publicRoutes = map[string]bool{
	"POST /api/v1/auth/login":    true,
	"POST /api/v1/auth/logout":   true,
	"GET /api/v1/auth/me":        true,
	"POST /api/v1/auth/password": true,
	"GET /api/v1/auth/session":   true,
}

// ingestRoute picks its permission in the handler; see handleIngest.
const ingestRoute = "POST /api/v1/ingest"

// routeRule is a table entry keyed without the trailing slash: chi.Find
// returns "/x" or "/x/" depending on the request, chi.Walk always "/x/".
type routeRule struct {
	perm   auth.Permission
	public bool
}

var routeRules = func() map[string]routeRule {
	m := map[string]routeRule{routeKey(ingestRoute): {public: true}}
	for k, p := range routePermissions {
		m[routeKey(k)] = routeRule{perm: p}
	}
	for k := range publicRoutes {
		m[routeKey(k)] = routeRule{public: true}
	}
	return m
}()

func routeKey(k string) string { return strings.TrimSuffix(k, "/") }

// DeclaredRoutes lists every route the permission table covers.
func DeclaredRoutes() []string {
	out := []string{ingestRoute}
	for k := range routePermissions {
		out = append(out, k)
	}
	for k := range publicRoutes {
		out = append(out, k)
	}
	return out
}

// RequirePermission enforces the table. Used inside a group, RoutePattern()
// only reaches the subrouter mount ("/projects/{project}/*"), so the full
// pattern is looked up on root instead.
func (s *Server) RequirePermission(root *chi.Mux) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.RawPath
			if path == "" {
				path = r.URL.Path
			}
			route := r.Method + " " + root.Find(chi.NewRouteContext(), r.Method, path)
			rule, ok := routeRules[routeKey(route)]
			if !ok {
				s.log.Error("route has no permission entry", "route", route)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
				return
			}
			if !rule.public && !s.allowed(w, r, route, rule.perm) {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// allowed answers 403 and reports false when the caller lacks perm.
func (s *Server) allowed(w http.ResponseWriter, r *http.Request, route string, perm auth.Permission) bool {
	p, _ := auth.PrincipalFrom(r.Context())
	if p.Can(perm) {
		return true
	}
	s.log.Info("permission denied",
		"kind", p.Kind, "user_id", p.UserID, "route", route, "permission", perm)
	writeJSON(w, http.StatusForbidden, map[string]string{"error": "not allowed"})
	return false
}
