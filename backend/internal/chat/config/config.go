// Package config reads the chat service's settings from the environment.
package config

import (
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	MaxAnswerTimeout    = 300 * time.Second
	MaxAdmissionWait    = 5 * time.Second
	defaultLogLevel     = "info"
	defaultProviderName = "vertex"
)

// Settings is everything the chat service reads from the environment.
type Settings struct {
	BackendBaseURL  string
	BackendAPIToken string
	BackendTimeout  time.Duration
	LogLevel        string
	AppAddr         string

	LLMProvider        string
	LLMModel           string
	LLMTemperature     *float64 // nil: the provider's default
	LLMMaxOutputTokens int
	LLMTimeout         time.Duration
	VertexProject      string
	VertexLocation     string

	ContextCacheTTL      time.Duration
	FeaturesCache        time.Duration
	MaxHistoryMessages   int
	MaxToolIterations    int
	MaxTurnTokens        int
	MaxConversationTurns int
	MaxConcurrentTurns   int
	TurnAdmissionWait    time.Duration
	MaxQuestionChars     int
	AnswerTimeout        time.Duration
	MaxRequestBytes      int64
}

// defaults maps each variable to its default; "" means none.
var defaults = map[string]string{
	"BACKEND_BASE_URL":            "http://backend:8080",
	"BACKEND_API_TOKEN":           "",
	"BACKEND_TIMEOUT_SECONDS":     "30",
	"LOG_LEVEL":                   defaultLogLevel,
	"APP_ADDR":                    ":8090",
	"LLM_PROVIDER":                defaultProviderName,
	"LLM_MODEL":                   "gemini-2.5-flash",
	"LLM_TEMPERATURE":             "",
	"LLM_MAX_OUTPUT_TOKENS":       "2048",
	"LLM_TIMEOUT_SECONDS":         "60",
	"VERTEX_PROJECT":              "",
	"VERTEX_LOCATION":             "us-central1",
	"CONTEXT_CACHE_TTL_SECONDS":   "300",
	"FEATURES_CACHE_SECONDS":      "15",
	"MAX_HISTORY_MESSAGES":        "20",
	"MAX_TOOL_ITERATIONS":         "6",
	"MAX_TURN_TOKENS":             "32000",
	"MAX_CONVERSATION_TURNS":      "50",
	"MAX_CONCURRENT_TURNS":        "4",
	"TURN_ADMISSION_WAIT_SECONDS": "0.5",
	"MAX_QUESTION_CHARS":          "4000",
	"ANSWER_TIMEOUT_SECONDS":      "120",
	"MAX_REQUEST_BYTES":           "1048576",
}

// EnvNames lists every variable Load reads, sorted.
func EnvNames() []string {
	names := make([]string, 0, len(defaults))
	for name := range defaults {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Default is the value Load uses when name is unset; "" means none.
func Default(name string) string { return defaults[name] }

type loader struct {
	problems []string
	bad      map[string]bool // unparsable, so range checks skip them
}

func (l *loader) fail(format string, args ...any) {
	l.problems = append(l.problems, fmt.Sprintf(format, args...))
}

// An empty variable counts as unset, as in the backend's config.
func (l *loader) str(name string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return defaults[name]
}

func (l *loader) int(name string) int {
	v, err := strconv.Atoi(strings.TrimSpace(l.str(name)))
	if err != nil {
		l.fail("%s must be an integer, got %q", name, l.str(name))
		l.bad[name] = true
	}
	return v
}

func (l *loader) float(name string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(l.str(name)), 64)
	if err != nil {
		l.fail("%s must be a number, got %q", name, l.str(name))
		l.bad[name] = true
	}
	return v
}

func (l *loader) seconds(name string) time.Duration {
	v := l.float(name)
	ns := v * float64(time.Second)
	if math.IsNaN(ns) || math.Abs(ns) >= math.MaxInt64 {
		if !l.bad[name] {
			l.fail("%s is too large, got %q", name, l.str(name))
			l.bad[name] = true
		}
		return 0
	}
	return time.Duration(ns)
}

// Load reads and validates the settings, reporting every problem at once.
// allowedProviders is owned by the provider registry.
func Load(allowedProviders []string) (*Settings, error) {
	l := &loader{bad: map[string]bool{}}
	s := &Settings{
		BackendBaseURL:       strings.TrimRight(l.str("BACKEND_BASE_URL"), "/"),
		BackendAPIToken:      l.str("BACKEND_API_TOKEN"),
		BackendTimeout:       l.seconds("BACKEND_TIMEOUT_SECONDS"),
		LogLevel:             logLevel(l.str("LOG_LEVEL")),
		AppAddr:              l.str("APP_ADDR"),
		LLMProvider:          strings.TrimSpace(l.str("LLM_PROVIDER")),
		LLMModel:             l.str("LLM_MODEL"),
		LLMMaxOutputTokens:   l.int("LLM_MAX_OUTPUT_TOKENS"),
		LLMTimeout:           l.seconds("LLM_TIMEOUT_SECONDS"),
		VertexProject:        strings.TrimSpace(l.str("VERTEX_PROJECT")),
		VertexLocation:       strings.TrimSpace(l.str("VERTEX_LOCATION")),
		ContextCacheTTL:      l.seconds("CONTEXT_CACHE_TTL_SECONDS"),
		MaxHistoryMessages:   l.int("MAX_HISTORY_MESSAGES"),
		MaxToolIterations:    l.int("MAX_TOOL_ITERATIONS"),
		MaxTurnTokens:        l.int("MAX_TURN_TOKENS"),
		MaxConversationTurns: l.int("MAX_CONVERSATION_TURNS"),
		MaxConcurrentTurns:   l.int("MAX_CONCURRENT_TURNS"),
		MaxQuestionChars:     l.int("MAX_QUESTION_CHARS"),
		MaxRequestBytes:      int64(l.int("MAX_REQUEST_BYTES")),
	}
	if l.str("LLM_TEMPERATURE") != "" {
		t := l.float("LLM_TEMPERATURE")
		s.LLMTemperature = &t
	}

	if !slices.Contains(allowedProviders, s.LLMProvider) {
		l.fail("LLM_PROVIDER must be one of %v, got %q", allowedProviders, s.LLMProvider)
	}
	if strings.HasPrefix(s.LLMProvider, "vertex") && s.VertexProject == "" {
		l.fail("VERTEX_PROJECT must be set when LLM_PROVIDER is '%s'", s.LLMProvider)
	}
	if strings.HasPrefix(s.LLMProvider, "vertex") && s.VertexLocation == "" {
		l.fail("VERTEX_LOCATION must be set when LLM_PROVIDER is '%s'", s.LLMProvider)
	}
	if !endsInPort(s.AppAddr) {
		l.fail("APP_ADDR must end in a port, got %q", s.AppAddr)
	}
	for _, c := range []struct {
		name string
		v    int
	}{
		{"MAX_CONCURRENT_TURNS", s.MaxConcurrentTurns},
		{"MAX_TURN_TOKENS", s.MaxTurnTokens},
		{"MAX_CONVERSATION_TURNS", s.MaxConversationTurns},
	} {
		if c.v < 1 && !l.bad[c.name] {
			l.fail("%s must be at least 1, got %d", c.name, c.v)
		}
	}

	switch features := l.seconds("FEATURES_CACHE_SECONDS"); {
	case l.bad["FEATURES_CACHE_SECONDS"]:
	case features < time.Second:
		l.fail("FEATURES_CACHE_SECONDS must be at least 1, got %g", features.Seconds())
	default:
		s.FeaturesCache = features
	}
	switch wait := l.seconds("TURN_ADMISSION_WAIT_SECONDS"); {
	case l.bad["TURN_ADMISSION_WAIT_SECONDS"]:
	case wait <= 0:
		l.fail("TURN_ADMISSION_WAIT_SECONDS must be greater than zero, got %g; "+
			"zero would refuse every turn, not only the ones over the cap", wait.Seconds())
	default:
		s.TurnAdmissionWait = min(wait, MaxAdmissionWait)
	}
	switch answer := l.seconds("ANSWER_TIMEOUT_SECONDS"); {
	case l.bad["ANSWER_TIMEOUT_SECONDS"]:
	case answer <= 0:
		l.fail("ANSWER_TIMEOUT_SECONDS must be greater than zero, got %g", answer.Seconds())
	default:
		s.AnswerTimeout = min(answer, MaxAnswerTimeout)
	}

	// Zero means no timeout in Go.
	for _, c := range []struct {
		name string
		v    time.Duration
	}{
		{"BACKEND_TIMEOUT_SECONDS", s.BackendTimeout},
		{"LLM_TIMEOUT_SECONDS", s.LLMTimeout},
	} {
		if c.v <= 0 && !l.bad[c.name] {
			l.fail("%s must be greater than zero, got %g", c.name, c.v.Seconds())
		}
	}
	if s.MaxRequestBytes < 1 && !l.bad["MAX_REQUEST_BYTES"] {
		l.fail("MAX_REQUEST_BYTES must be at least 1, got %d", s.MaxRequestBytes)
	}
	if s.ContextCacheTTL < 0 && !l.bad["CONTEXT_CACHE_TTL_SECONDS"] {
		l.fail("CONTEXT_CACHE_TTL_SECONDS must not be negative, got %g", s.ContextCacheTTL.Seconds())
	}

	if len(l.problems) > 0 {
		return nil, errors.New(strings.Join(l.problems, "; "))
	}
	return s, nil
}

// Unknown levels fall back rather than fail, as the backend's parseLevel does.
func logLevel(v string) string {
	switch level := strings.ToLower(strings.TrimSpace(v)); level {
	case "debug", "info", "warn", "error":
		return level
	default:
		return defaultLogLevel
	}
}

func endsInPort(addr string) bool {
	port := addr[strings.LastIndex(addr, ":")+1:]
	if port == "" {
		return false
	}
	for _, r := range port {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
