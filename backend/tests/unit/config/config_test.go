// Unit tests for environment parsing: defaults, overrides and the one setting
// the server refuses to start without.
package config_test

import (
	"testing"
	"time"

	"urara-vision/backend/internal/config"
)

// setRequired supplies the only variable Load insists on, so each test can
// speak about the setting it actually cares about.
func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("NEO4J_PASSWORD", "test-password")
}

func TestLoadAppliesDefaults(t *testing.T) {
	setRequired(t)
	// Clear everything else so an inherited value cannot make this pass.
	for _, k := range []string{
		"APP_ADDR", "POSTGRES_DSN", "NEO4J_URI", "NEO4J_USER", "CORS_ORIGINS",
		"MAX_UPLOAD_BYTES", "MAX_FILES", "MAX_CONTEXT_TABLES",
		"SHUTDOWN_TIMEOUT_SECONDS", "LOG_LEVEL",
	} {
		t.Setenv(k, "")
	}

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if c.Addr != ":8080" {
		t.Errorf("Addr = %q", c.Addr)
	}
	if c.Neo4jURI != "bolt://localhost:7687" || c.Neo4jUser != "neo4j" {
		t.Errorf("neo4j = %q / %q", c.Neo4jURI, c.Neo4jUser)
	}
	if c.MaxUploadBytes != 64<<20 {
		t.Errorf("MaxUploadBytes = %d, want %d", c.MaxUploadBytes, 64<<20)
	}
	if c.MaxFiles != 5000 {
		t.Errorf("MaxFiles = %d", c.MaxFiles)
	}
	if c.MaxContextTables != 400 {
		t.Errorf("MaxContextTables = %d", c.MaxContextTables)
	}
	if c.ShutdownTimeout != 20*time.Second {
		t.Errorf("ShutdownTimeout = %s", c.ShutdownTimeout)
	}
	if c.LogLevel != "info" {
		t.Errorf("LogLevel = %q", c.LogLevel)
	}
	if len(c.CORSOrigins) != 2 {
		t.Errorf("CORSOrigins = %v, want the two dev origins", c.CORSOrigins)
	}
}

func TestLoadReadsOverrides(t *testing.T) {
	setRequired(t)
	t.Setenv("APP_ADDR", ":9999")
	t.Setenv("POSTGRES_DSN", "postgres://u:p@db:5432/x?sslmode=disable")
	t.Setenv("MAX_FILES", "12")
	t.Setenv("MAX_UPLOAD_BYTES", "2048")
	t.Setenv("MAX_CONTEXT_TABLES", "25")
	t.Setenv("SHUTDOWN_TIMEOUT_SECONDS", "5")
	t.Setenv("LOG_LEVEL", "debug")

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if c.Addr != ":9999" {
		t.Errorf("Addr = %q", c.Addr)
	}
	if c.PostgresDSN != "postgres://u:p@db:5432/x?sslmode=disable" {
		t.Errorf("PostgresDSN = %q", c.PostgresDSN)
	}
	if c.MaxFiles != 12 || c.MaxUploadBytes != 2048 {
		t.Errorf("limits = %d files / %d bytes", c.MaxFiles, c.MaxUploadBytes)
	}
	if c.MaxContextTables != 25 {
		t.Errorf("MaxContextTables = %d", c.MaxContextTables)
	}
	if c.ShutdownTimeout != 5*time.Second {
		t.Errorf("ShutdownTimeout = %s", c.ShutdownTimeout)
	}
	if c.LogLevel != "debug" {
		t.Errorf("LogLevel = %q", c.LogLevel)
	}
}

// TestMissingNeo4jPasswordIsFatal: the graph store is not optional, and an
// empty password would fail later at connect time with a worse message.
func TestMissingNeo4jPasswordIsFatal(t *testing.T) {
	t.Setenv("NEO4J_PASSWORD", "")
	if _, err := config.Load(); err == nil {
		t.Fatal("Load() succeeded without NEO4J_PASSWORD")
	}
}

// TestCORSOriginsAreTrimmedAndCompacted covers a hand-edited env var: spaces
// around the commas and a trailing one are normal, and an empty origin would
// otherwise reach the CORS middleware as a valid entry.
func TestCORSOriginsAreTrimmedAndCompacted(t *testing.T) {
	setRequired(t)
	t.Setenv("CORS_ORIGINS", " http://a.example , ,http://b.example, ")

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	want := []string{"http://a.example", "http://b.example"}
	if len(c.CORSOrigins) != len(want) {
		t.Fatalf("CORSOrigins = %v, want %v", c.CORSOrigins, want)
	}
	for i := range want {
		if c.CORSOrigins[i] != want[i] {
			t.Errorf("CORSOrigins[%d] = %q, want %q", i, c.CORSOrigins[i], want[i])
		}
	}
}

// TestUnparseableNumbersFallBackToDefaults: a typo in a numeric limit must not
// silently become zero, which would reject every upload.
func TestUnparseableNumbersFallBackToDefaults(t *testing.T) {
	setRequired(t)
	t.Setenv("MAX_FILES", "lots")
	t.Setenv("MAX_UPLOAD_BYTES", "64MB")

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if c.MaxFiles != 5000 {
		t.Errorf("MaxFiles = %d, want the default 5000", c.MaxFiles)
	}
	if c.MaxUploadBytes != 64<<20 {
		t.Errorf("MaxUploadBytes = %d, want the default", c.MaxUploadBytes)
	}
}

func TestAPITokenRejectsShortValue(t *testing.T) {
	t.Setenv("NEO4J_PASSWORD", "x")
	t.Setenv("API_TOKEN", "tooshort")

	if _, err := config.Load(); err == nil {
		t.Fatal("Load() accepted an 8-character API_TOKEN, want an error")
	}
}

func TestAPITokenAccepted(t *testing.T) {
	const tok = "0123456789abcdef0123456789abcdef"
	t.Setenv("NEO4J_PASSWORD", "x")
	t.Setenv("API_TOKEN", tok)

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if c.APIToken != tok {
		t.Errorf("APIToken = %q, want %q", c.APIToken, tok)
	}
}

// An unset token is the documented way to run without authentication.
func TestAPITokenDefaultsEmpty(t *testing.T) {
	t.Setenv("NEO4J_PASSWORD", "x")

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if c.APIToken != "" {
		t.Errorf("APIToken = %q, want empty", c.APIToken)
	}
}
