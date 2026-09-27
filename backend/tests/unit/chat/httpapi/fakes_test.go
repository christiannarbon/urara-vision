package httpapi_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/config"
	"urara-vision/backend/internal/chat/httpapi"
	"urara-vision/backend/internal/chat/reqctx"
)

const featuresTTL = 15 * time.Second

type fakeBackend struct {
	mu           sync.Mutex
	enabled      bool
	featuresErr  error
	unhealthy    bool
	delay        time.Duration
	featureCalls int
	healthCalls  int
	featureUsers []string // the user ID on each features call
	cancelled    bool     // a features call arrived with a cancelled context
}

func (f *fakeBackend) Health(context.Context) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.healthCalls++
	return !f.unhealthy
}

func (f *fakeBackend) Features(ctx context.Context) (apiclient.Features, error) {
	f.mu.Lock()
	f.featureCalls++
	f.featureUsers = append(f.featureUsers, reqctx.UserID(ctx))
	f.cancelled = f.cancelled || ctx.Err() != nil
	delay, err, on := f.delay, f.featuresErr, f.enabled
	f.mu.Unlock()

	time.Sleep(delay)
	if err != nil {
		return apiclient.Features{}, err
	}
	return apiclient.Features{Chat: apiclient.ChatFeature{Available: true, Enabled: on}}, nil
}

func (f *fakeBackend) set(fn func(*fakeBackend)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeBackend) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.featureCalls
}

func (f *fakeBackend) health() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.healthCalls
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock { return &clock{t: time.Unix(1000, 0)} }

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func chatSettings() *config.Settings {
	return &config.Settings{
		MaxRequestBytes: 1 << 20,
		FeaturesCache:   featuresTTL,
		LLMProvider:     "vertex",
		LLMModel:        "gemini-2.5-flash",
		VertexLocation:  "us-central1",
	}
}

// gated builds the full handler over a fake backend and clock.
func gated(t *testing.T, f *fakeBackend, c *clock) http.Handler {
	t.Helper()
	settings := chatSettings()
	return httpapi.New(httpapi.Deps{
		Settings: settings,
		Log:      slog.New(slog.NewJSONHandler(&logs{}, nil)),
		Backend:  f,
		Clock:    c.now,
	}).Handler()
}

// asUser requests the stub chat route, with a user ID unless it is empty.
func asUser(h http.Handler, user string) int {
	req := httptest.NewRequest("GET", "/api/chat/conversations?snapshot=latest", nil)
	if user != "" {
		req.Header.Set("X-User-Id", user)
	}
	return serve(h, req).Code
}
