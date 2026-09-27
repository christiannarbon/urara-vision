package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/reqctx"
	"urara-vision/backend/internal/chat/tools"
)

// ToolBackend is what /debug/tool needs: the tools' calls plus snapshot resolution.
type ToolBackend interface {
	tools.Backend
	ResolveSnapshot(ctx context.Context, sid string) (string, error)
}

type toolListing struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}

func (s *Server) debugTools(w http.ResponseWriter, _ *http.Request) {
	specs := tools.Build(nil, "unused-for-schemas")
	out := make([]toolListing, len(specs))
	for i, spec := range specs {
		out[i] = toolListing{Name: spec.Name, Description: spec.Description, Schema: spec.Schema}
	}
	writeJSON(w, http.StatusOK, out)
}

type toolInvokeRequest struct {
	SnapshotID      string                     `json:"snapshotId"`
	SnapshotIDSnake string                     `json:"snapshot_id"` // pydantic's populate_by_name
	Tool            string                     `json:"tool"`
	Args            map[string]json.RawMessage `json:"args"`
}

// debugTool runs one tool unguarded and returns exactly what it returned.
func (s *Server) debugTool(w http.ResponseWriter, r *http.Request) {
	var body toolInvokeRequest
	if !s.DecodeJSON(w, r, &body) {
		return
	}
	sid := body.SnapshotID
	if sid == "" {
		sid = body.SnapshotIDSnake
	}
	var missing []FieldProblem
	if sid == "" {
		missing = append(missing, Required("snapshotId"))
	}
	if body.Tool == "" {
		missing = append(missing, Required("tool"))
	}
	if len(missing) > 0 {
		FieldError(w, r, missing...)
		return
	}

	// Resolved first, so "latest" works and an unknown snapshot is a 404.
	resolved, err := s.tools.ResolveSnapshot(r.Context(), sid)
	if err != nil {
		s.writeToolError(w, r, err)
		return
	}
	specs := tools.Build(s.tools, resolved)
	i := slices.IndexFunc(specs, func(spec tools.Spec) bool { return spec.Name == body.Tool })
	if i < 0 {
		names := tools.Names()
		slices.Sort(names)
		quoted := make([]string, len(names))
		for j, n := range names {
			quoted[j] = pyRepr(n)
		}
		WriteError(w, r, http.StatusBadRequest, map[string]any{
			"detail": "unknown tool " + pyRepr(body.Tool) + "; expected one of [" + strings.Join(quoted, ", ") + "]",
		})
		return
	}

	args, _ := json.Marshal(body.Args)
	if body.Args == nil {
		args = []byte("{}")
	}
	if err := tools.Validate(body.Tool, args); err != nil {
		var argsErr *tools.ArgsError
		if errors.As(err, &argsErr) {
			WriteError(w, r, http.StatusBadRequest, map[string]any{"detail": argsErr.Problems})
		} else {
			WriteError(w, r, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		}
		return
	}
	result, err := specs[i].Run(r.Context(), args)
	if err != nil {
		s.writeToolError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// writeToolError maps a backend failure as Python's _run: 404, else 502.
func (s *Server) writeToolError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apiclient.Error
	if !errors.As(err, &apiErr) {
		s.log.Error("debug tool failed", "request_id", reqctx.RequestID(r.Context()), "error", err.Error())
		WriteError(w, r, http.StatusInternalServerError, map[string]any{"error": MsgInternal})
		return
	}
	status := http.StatusBadGateway
	if errors.Is(err, apiclient.ErrNotFound) {
		status = http.StatusNotFound
	}
	WriteError(w, r, status, map[string]any{"detail": apiErr.Message})
}

// pyRepr quotes a string as Python's repr does for the common cases.
func pyRepr(s string) string {
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		return `"` + s + `"`
	}
	return "'" + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), "'", `\'`) + "'"
}
