// Reading an upload.
//
// The frontend reads the chosen directory in the browser and posts the markdown
// as text, either as JSON or as multipart, so nothing here touches the user's
// filesystem. The paths arrive from the browser, which makes them the one piece
// of genuinely untrusted input; normalisePath is where that is dealt with.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"urara-vision/backend/internal/parser"
	"urara-vision/backend/internal/projectmeta"
)

// ingestRequest is the JSON body accepted by POST /api/v1/ingest. The frontend
// reads the selected directory client-side and posts the markdown as text, so
// the backend never needs filesystem access to the user's machine.
type ingestRequest struct {
	Name        string `json:"name"`
	SourceLabel string `json:"sourceLabel"`
	Files       []struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	} `json:"files"`
}

// upload is one read request body: the markdown to parse, the manifest that
// has to come with it, and the labels the snapshot is named from.
type upload struct {
	files       []parser.File
	meta        string
	metaFound   bool
	name        string
	sourceLabel string
	// nestedMeta holds manifests found below the root. They are not read --
	// only the root one counts -- but they turn "there is no manifest" into
	// "the manifest is in the wrong place", which is a different fix.
	nestedMeta []string
}

// add files one uploaded file, sorting it into the markdown to parse, the
// manifest, or neither.
func (u *upload) add(rawPath, content string) {
	p := normalisePath(rawPath)
	switch {
	case p == "":
	case strings.EqualFold(p, projectmeta.FileName):
		u.meta = content
		u.metaFound = true
	case strings.EqualFold(path.Base(p), projectmeta.FileName):
		u.nestedMeta = append(u.nestedMeta, p)
	case strings.EqualFold(path.Ext(p), ".md"):
		u.files = append(u.files, parser.File{Path: p, Content: content})
	}
}

// readJSON decodes a JSON ingest body.
func (s *Server) readJSON(r *http.Request) (*upload, error) {
	var req ingestRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		return nil, fmt.Errorf("invalid JSON body: %w", err)
	}
	up := &upload{
		files:       make([]parser.File, 0, len(req.Files)),
		name:        req.Name,
		sourceLabel: req.SourceLabel,
	}
	for _, f := range req.Files {
		up.add(f.Path, f.Content)
	}
	return up, nil
}

// readMultipart streams a multipart ingest body. Each file part carries its
// relative path as the form field name; the "name" and "sourceLabel" fields
// are plain values.
func (s *Server) readMultipart(r *http.Request) (*upload, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, fmt.Errorf("invalid multipart body: %w", err)
	}

	up := &upload{}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read multipart part: %w", err)
		}

		field := part.FormName()
		switch field {
		case "name", "sourceLabel":
			b, err := io.ReadAll(io.LimitReader(part, 4096))
			_ = part.Close()
			if err != nil {
				return nil, err
			}
			if field == "name" {
				up.name = string(b)
			} else {
				up.sourceLabel = string(b)
			}
			continue
		}

		// Prefer the declared filename, falling back to the field name, so the
		// frontend can use either convention.
		p := normalisePath(part.FileName())
		if p == "" || !strings.Contains(p, "/") {
			if fp := normalisePath(field); fp != "" {
				p = fp
			}
		}
		if !wanted(p) {
			_ = part.Close()
			continue
		}

		b, err := io.ReadAll(part)
		_ = part.Close()
		if err != nil {
			return nil, fmt.Errorf("read file %q: %w", p, err)
		}
		up.add(p, string(b))
	}
	return up, nil
}

// wanted reports whether a multipart part is worth reading into memory at all:
// the markdown to parse, or a manifest wherever it turned up.
func wanted(p string) bool {
	if p == "" {
		return false
	}
	return strings.EqualFold(path.Ext(p), ".md") ||
		strings.EqualFold(path.Base(p), projectmeta.FileName)
}

// normalisePath cleans an uploaded relative path and rejects anything that
// tries to escape the selected directory. Paths are only ever used as
// identifiers, never to touch the filesystem, but keeping them well-formed
// stops confusing IDs from reaching the graph.
func normalisePath(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	if p == "" {
		return ""
	}
	p = path.Clean(p)
	p = strings.TrimPrefix(p, "/")
	for strings.HasPrefix(p, "../") {
		p = strings.TrimPrefix(p, "../")
	}
	if p == "." || p == ".." {
		return ""
	}
	// Ignore editor and OS cruft that would otherwise become phantom tables.
	base := path.Base(p)
	if strings.HasPrefix(base, ".") {
		return ""
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "node_modules" || seg == ".git" {
			return ""
		}
	}
	return p
}

// defaultName derives a readable snapshot name from the selected directory.
func defaultName(sourceLabel string) string {
	base := path.Base(strings.TrimSuffix(normalisePath(sourceLabel), "/"))
	if base == "" || base == "." {
		base = "documentation"
	}
	return fmt.Sprintf("%s — %s", base, time.Now().Format("2006-01-02 15:04"))
}
