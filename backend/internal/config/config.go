// Package config reads runtime settings from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// minAPITokenLen is the shortest token Load will accept. 24 characters is
// comfortably past the point where an online guessing attack is practical
// against a random token.
const minAPITokenLen = 24

// Config holds every setting the server needs.
type Config struct {
	Addr            string
	PostgresDSN     string
	Neo4jURI        string
	Neo4jUser       string
	Neo4jPassword   string
	APIToken        string
	CORSOrigins     []string
	MaxUploadBytes  int64
	MaxFiles        int
	ShutdownTimeout time.Duration
	LogLevel        string
}

// Load builds a Config from the environment, applying defaults that work with
// the bundled docker-compose stack.
func Load() (*Config, error) {
	c := &Config{
		Addr:            env("APP_ADDR", ":8080"),
		PostgresDSN:     env("POSTGRES_DSN", "postgres://relviz:relviz@localhost:5432/relviz?sslmode=disable"),
		Neo4jURI:        env("NEO4J_URI", "bolt://localhost:7687"),
		Neo4jUser:       env("NEO4J_USER", "neo4j"),
		Neo4jPassword:   env("NEO4J_PASSWORD", ""),
		APIToken:        env("API_TOKEN", ""),
		CORSOrigins:     envList("CORS_ORIGINS", "http://localhost:5173,http://localhost:8081"),
		MaxUploadBytes:  envInt64("MAX_UPLOAD_BYTES", 64<<20),
		MaxFiles:        int(envInt64("MAX_FILES", 5000)),
		ShutdownTimeout: time.Duration(envInt64("SHUTDOWN_TIMEOUT_SECONDS", 20)) * time.Second,
		LogLevel:        env("LOG_LEVEL", "info"),
	}
	if c.Neo4jPassword == "" {
		return nil, fmt.Errorf("NEO4J_PASSWORD must be set")
	}
	// An unset token disables authentication, which is the documented way to
	// run locally. A short one is worse than that: it looks like a control
	// while being trivially guessable, so it is refused outright.
	if c.APIToken != "" && len(c.APIToken) < minAPITokenLen {
		return nil, fmt.Errorf("API_TOKEN must be at least %d characters (leave it unset to disable authentication)", minAPITokenLen)
	}
	return c, nil
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envInt64(key string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func envList(key, def string) []string {
	raw := env(key, def)
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
