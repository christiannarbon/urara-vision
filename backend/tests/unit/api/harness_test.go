// Wiring a Server onto the fakes, and issuing requests against its router.
package api_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"urara-vision/backend/internal/api"
	"urara-vision/backend/internal/config"
)

// newServer wires a Server onto the two fakes. Logs go nowhere: the handlers
// log expected failures, and a test run should stay readable.
func newServer(t *testing.T, meta *fakeMeta, graphs *fakeGraphs) http.Handler {
	return newServerWithMaxFiles(t, meta, graphs, 100)
}

// newServerWithMaxFiles is newServer with the upload cap lowered, so the limit
// can be tested without building thousands of documents.
func newServerWithMaxFiles(t *testing.T, meta *fakeMeta, graphs *fakeGraphs, maxFiles int) http.Handler {
	t.Helper()
	cfg := &config.Config{
		CORSOrigins:    []string{"http://localhost:5173"},
		MaxUploadBytes: 64 << 20,
		MaxFiles:       maxFiles,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return api.New(cfg, meta, graphs, log).Routes()
}

// newServerWithContextCap is newServer with the /context table cap lowered, so
// truncation can be tested without building hundreds of tables.
func newServerWithContextCap(t *testing.T, meta *fakeMeta, graphs *fakeGraphs, maxTables int) http.Handler {
	t.Helper()
	cfg := &config.Config{
		CORSOrigins:      []string{"http://localhost:5173"},
		MaxUploadBytes:   64 << 20,
		MaxFiles:         100,
		MaxContextTables: maxTables,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return api.New(cfg, meta, graphs, log).Routes()
}

// do issues a request against the router and returns the recorder.
func do(t *testing.T, h http.Handler, method, target string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
