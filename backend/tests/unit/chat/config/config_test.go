// Ported from chat/tests/unit/test_config.py and test_llm_config.py.
package config_test

import (
	"strings"
	"testing"
	"time"

	"urara-vision/backend/internal/chat/config"
)

var providers = []string{"vertex"}

// clean clears every setting and supplies the one Load requires.
func clean(t *testing.T) {
	t.Helper()
	for _, name := range config.EnvNames() {
		t.Setenv(name, "")
	}
	t.Setenv("VERTEX_PROJECT", "my-project")
}

func load(t *testing.T) *config.Settings {
	t.Helper()
	s, err := config.Load(providers)
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	return s
}

func refused(t *testing.T, want ...string) string {
	t.Helper()
	_, err := config.Load(providers)
	if err == nil {
		t.Fatal("Load() succeeded, want an error")
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("error %q does not mention %q", err, w)
		}
	}
	return err.Error()
}

func TestDefaults(t *testing.T) {
	clean(t)
	s := load(t)

	checks := []struct {
		name      string
		got, want any
	}{
		{"BackendBaseURL", s.BackendBaseURL, "http://backend:8080"},
		{"BackendAPIToken", s.BackendAPIToken, ""},
		{"BackendTimeout", s.BackendTimeout, 30 * time.Second},
		{"LogLevel", s.LogLevel, "info"},
		{"AppAddr", s.AppAddr, ":8090"},
		{"LLMProvider", s.LLMProvider, "vertex"},
		{"LLMModel", s.LLMModel, "gemini-2.5-flash"},
		{"LLMMaxOutputTokens", s.LLMMaxOutputTokens, 2048},
		{"LLMTimeout", s.LLMTimeout, 60 * time.Second},
		{"VertexLocation", s.VertexLocation, "us-central1"},
		{"ContextCacheTTL", s.ContextCacheTTL, 300 * time.Second},
		{"FeaturesCache", s.FeaturesCache, 15 * time.Second},
		{"MaxHistoryMessages", s.MaxHistoryMessages, 20},
		{"MaxToolIterations", s.MaxToolIterations, 6},
		{"MaxTurnTokens", s.MaxTurnTokens, 32000},
		{"MaxConversationTurns", s.MaxConversationTurns, 50},
		{"MaxConcurrentTurns", s.MaxConcurrentTurns, 4},
		{"TurnAdmissionWait", s.TurnAdmissionWait, 500 * time.Millisecond},
		{"MaxQuestionChars", s.MaxQuestionChars, 4000},
		{"AnswerTimeout", s.AnswerTimeout, 120 * time.Second},
		{"MaxRequestBytes", s.MaxRequestBytes, int64(1048576)},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if s.LLMTemperature != nil {
		t.Errorf("LLMTemperature = %v, want nil (provider default)", *s.LLMTemperature)
	}
}

func TestEverySettingReadsItsVariable(t *testing.T) {
	clean(t)
	env := map[string]string{
		"BACKEND_BASE_URL":            "http://elsewhere:9000",
		"BACKEND_API_TOKEN":           "a-token",
		"BACKEND_TIMEOUT_SECONDS":     "5.5",
		"LOG_LEVEL":                   "debug",
		"APP_ADDR":                    "127.0.0.1:9999",
		"LLM_MODEL":                   "gemini-2.5-pro",
		"LLM_TEMPERATURE":             "0.9",
		"LLM_MAX_OUTPUT_TOKENS":       "512",
		"LLM_TIMEOUT_SECONDS":         "12.5",
		"VERTEX_PROJECT":              "other-project",
		"VERTEX_LOCATION":             "europe-west2",
		"CONTEXT_CACHE_TTL_SECONDS":   "10",
		"FEATURES_CACHE_SECONDS":      "2",
		"MAX_HISTORY_MESSAGES":        "3",
		"MAX_TOOL_ITERATIONS":         "4",
		"MAX_TURN_TOKENS":             "1000",
		"MAX_CONVERSATION_TURNS":      "7",
		"MAX_CONCURRENT_TURNS":        "1",
		"TURN_ADMISSION_WAIT_SECONDS": "1.5",
		"MAX_QUESTION_CHARS":          "100",
		"ANSWER_TIMEOUT_SECONDS":      "30",
		"MAX_REQUEST_BYTES":           "2048",
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	s := load(t)

	checks := []struct {
		name      string
		got, want any
	}{
		{"BackendBaseURL", s.BackendBaseURL, "http://elsewhere:9000"},
		{"BackendAPIToken", s.BackendAPIToken, "a-token"},
		{"BackendTimeout", s.BackendTimeout, 5500 * time.Millisecond},
		{"LogLevel", s.LogLevel, "debug"},
		{"AppAddr", s.AppAddr, "127.0.0.1:9999"},
		{"LLMModel", s.LLMModel, "gemini-2.5-pro"},
		{"LLMMaxOutputTokens", s.LLMMaxOutputTokens, 512},
		{"LLMTimeout", s.LLMTimeout, 12500 * time.Millisecond},
		{"VertexProject", s.VertexProject, "other-project"},
		{"VertexLocation", s.VertexLocation, "europe-west2"},
		{"ContextCacheTTL", s.ContextCacheTTL, 10 * time.Second},
		{"FeaturesCache", s.FeaturesCache, 2 * time.Second},
		{"MaxHistoryMessages", s.MaxHistoryMessages, 3},
		{"MaxToolIterations", s.MaxToolIterations, 4},
		{"MaxTurnTokens", s.MaxTurnTokens, 1000},
		{"MaxConversationTurns", s.MaxConversationTurns, 7},
		{"MaxConcurrentTurns", s.MaxConcurrentTurns, 1},
		{"TurnAdmissionWait", s.TurnAdmissionWait, 1500 * time.Millisecond},
		{"MaxQuestionChars", s.MaxQuestionChars, 100},
		{"AnswerTimeout", s.AnswerTimeout, 30 * time.Second},
		{"MaxRequestBytes", s.MaxRequestBytes, int64(2048)},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if s.LLMTemperature == nil || *s.LLMTemperature != 0.9 {
		t.Errorf("LLMTemperature = %v, want 0.9", s.LLMTemperature)
	}
}

func TestBaseURLTrailingSlashIsStripped(t *testing.T) {
	for in, want := range map[string]string{"http://x:8080/": "http://x:8080", "http://x:8080": "http://x:8080"} {
		clean(t)
		t.Setenv("BACKEND_BASE_URL", in)
		if got := load(t).BackendBaseURL; got != want {
			t.Errorf("BACKEND_BASE_URL %q -> %q, want %q", in, got, want)
		}
	}
}

func TestLogLevel(t *testing.T) {
	for in, want := range map[string]string{
		"debug": "debug", "info": "info", "warn": "warn", "error": "error",
		"shouting": "info", " DEBUG ": "debug",
	} {
		clean(t)
		t.Setenv("LOG_LEVEL", in)
		if got := load(t).LogLevel; got != want {
			t.Errorf("LOG_LEVEL %q -> %q, want %q", in, got, want)
		}
	}
}

func TestAppAddrWithoutANumericPortIsRefused(t *testing.T) {
	for _, addr := range []string{"not-an-address", "localhost:", "localhost:http"} {
		clean(t)
		t.Setenv("APP_ADDR", addr)
		refused(t, "APP_ADDR")
	}
}

func TestVertexWithoutAProjectIsRefused(t *testing.T) {
	for _, project := range []string{"", "   "} {
		clean(t)
		t.Setenv("VERTEX_PROJECT", project)
		refused(t, "VERTEX_PROJECT must be set when LLM_PROVIDER is 'vertex'")
	}
}

func TestProjectAndLocationAreTrimmed(t *testing.T) {
	clean(t)
	t.Setenv("VERTEX_PROJECT", "  my-project\n")
	t.Setenv("VERTEX_LOCATION", " europe-west2 ")
	s := load(t)
	if s.VertexProject != "my-project" || s.VertexLocation != "europe-west2" {
		t.Errorf("project %q, location %q", s.VertexProject, s.VertexLocation)
	}
}

func TestProviderMustBeAllowed(t *testing.T) {
	clean(t)
	t.Setenv("LLM_PROVIDER", "gemini-studio")
	refused(t, "LLM_PROVIDER", "gemini-studio")

	clean(t)
	t.Setenv("LLM_PROVIDER", "vertex-claude")
	t.Setenv("VERTEX_PROJECT", "")
	if _, err := config.Load([]string{"vertex", "vertex-claude"}); err == nil ||
		!strings.Contains(err.Error(), "VERTEX_PROJECT must be set when LLM_PROVIDER is 'vertex-claude'") {
		t.Errorf("Load() = %v, want VERTEX_PROJECT required for any vertex provider", err)
	}
}

func TestCeilingsBelowOneAreRefused(t *testing.T) {
	for _, name := range []string{"MAX_CONCURRENT_TURNS", "MAX_TURN_TOKENS", "MAX_CONVERSATION_TURNS"} {
		clean(t)
		t.Setenv(name, "0")
		refused(t, name+" must be at least 1, got 0")
	}
}

func TestFeaturesCacheBelowOneSecondIsRefused(t *testing.T) {
	clean(t)
	t.Setenv("FEATURES_CACHE_SECONDS", "0.5")
	refused(t, "FEATURES_CACHE_SECONDS must be at least 1")
}

func TestTheTurnDeadline(t *testing.T) {
	if config.MaxAnswerTimeout != 5*time.Minute {
		t.Errorf("MaxAnswerTimeout = %v", config.MaxAnswerTimeout)
	}
	for in, want := range map[string]time.Duration{"3600": config.MaxAnswerTimeout, "30": 30 * time.Second} {
		clean(t)
		t.Setenv("ANSWER_TIMEOUT_SECONDS", in)
		if got := load(t).AnswerTimeout; got != want {
			t.Errorf("ANSWER_TIMEOUT_SECONDS %s -> %v, want %v", in, got, want)
		}
	}
	clean(t)
	t.Setenv("ANSWER_TIMEOUT_SECONDS", "0")
	refused(t, "ANSWER_TIMEOUT_SECONDS")
}

func TestTheAdmissionWait(t *testing.T) {
	clean(t)
	t.Setenv("TURN_ADMISSION_WAIT_SECONDS", "60")
	if got := load(t).TurnAdmissionWait; got != config.MaxAdmissionWait {
		t.Errorf("TurnAdmissionWait = %v, want the %v ceiling", got, config.MaxAdmissionWait)
	}
	for _, v := range []string{"0", "-1"} {
		clean(t)
		t.Setenv("TURN_ADMISSION_WAIT_SECONDS", v)
		refused(t, "TURN_ADMISSION_WAIT_SECONDS")
	}
}

func TestEveryProblemIsReportedTogether(t *testing.T) {
	clean(t)
	t.Setenv("VERTEX_PROJECT", "")
	t.Setenv("APP_ADDR", "localhost:http")
	t.Setenv("MAX_CONCURRENT_TURNS", "0")
	msg := refused(t, "VERTEX_PROJECT", "APP_ADDR", "MAX_CONCURRENT_TURNS")
	if n := strings.Count(msg, "; "); n != 2 {
		t.Errorf("want three problems joined by '; ', got %q", msg)
	}
}

// A value that does not parse is reported once, not again by its range check.
func TestAnUnparsableValueIsReportedOnce(t *testing.T) {
	clean(t)
	t.Setenv("MAX_CONCURRENT_TURNS", "four")
	msg := refused(t, `MAX_CONCURRENT_TURNS must be an integer, got "four"`)
	if strings.Contains(msg, ";") {
		t.Errorf("want one problem, got %q", msg)
	}
}
