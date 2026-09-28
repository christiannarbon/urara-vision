package httpapi_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/config"
	"urara-vision/backend/internal/chat/httpapi"
	"urara-vision/backend/internal/chat/reqctx"
	"urara-vision/backend/internal/model"
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

	convErr   error           // every conversation call fails with this
	messages  []model.Message // on a fetched conversation
	convUsers []string        // the user ID on each conversation call
	created   []createCall
	listed    []listCall
	fetched   []string
	deleted   []string

	title     *string // on a fetched conversation; nil: "golden"
	appendErr error   // an assistant append fails with this
	titleErr  error
	steps     []string // turn steps in order: get, append:<role>, agent, patch
	appended  []appendCall
	titles    []string
}

type appendCall struct {
	role, content string
	citations     []string
	meta          map[string]any
}

type createCall struct{ snapshotID, title string }

type listCall struct {
	snapshotID string
	limit      int
}

var convCreated = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

// stored is what the backend returns: a concrete snapshot ID, never "latest".
func stored(cid, title string) model.Conversation {
	return model.Conversation{
		ID: cid, SnapshotID: "5b0c1a52-3d8e-4f7a-9c21-6e4d8b2f1a90", Title: title,
		CreatedAt: convCreated, UpdatedAt: convCreated,
	}
}

func (f *fakeBackend) convCall(ctx context.Context, record func()) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.convUsers = append(f.convUsers, reqctx.UserID(ctx))
	record()
	return f.convErr
}

func (f *fakeBackend) CreateConversation(ctx context.Context, snapshotID, title string) (model.Conversation, error) {
	if err := f.convCall(ctx, func() { f.created = append(f.created, createCall{snapshotID, title}) }); err != nil {
		return model.Conversation{}, err
	}
	return stored("0d3c5e6f-7a8b-4c9d-8e1f-2a3b4c5d6e7f", title), nil
}

func (f *fakeBackend) ListConversations(ctx context.Context, snapshotID string, limit int) ([]model.Conversation, error) {
	if err := f.convCall(ctx, func() { f.listed = append(f.listed, listCall{snapshotID, limit}) }); err != nil {
		return nil, err
	}
	return []model.Conversation{stored("0d3c5e6f-7a8b-4c9d-8e1f-2a3b4c5d6e7f", "golden")}, nil
}

func (f *fakeBackend) GetConversation(ctx context.Context, cid string) (model.Conversation, error) {
	if err := f.convCall(ctx, func() { f.fetched = append(f.fetched, cid); f.steps = append(f.steps, "get") }); err != nil {
		return model.Conversation{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	c := stored(cid, "golden")
	if f.title != nil {
		c.Title = *f.title
	}
	c.Messages = slices.Clone(f.messages)
	return c, nil
}

func (f *fakeBackend) AppendMessage(ctx context.Context, cid, role, content string, citations []string, meta map[string]any) (model.Message, error) {
	if err := f.convCall(ctx, func() {
		f.steps = append(f.steps, "append:"+role)
		f.appended = append(f.appended, appendCall{role, content, citations, meta})
	}); err != nil {
		return model.Message{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if role == model.RoleAssistant && f.appendErr != nil {
		return model.Message{}, f.appendErr
	}
	return model.Message{
		Ordinal: len(f.messages) + len(f.appended) - 1, Role: role, Content: content,
		Citations: citations, Meta: meta, CreatedAt: convCreated,
	}, nil
}

func (f *fakeBackend) SetConversationTitle(ctx context.Context, cid, title string) (model.Conversation, error) {
	if err := f.convCall(ctx, func() {
		f.steps = append(f.steps, "patch")
		f.titles = append(f.titles, title)
	}); err != nil {
		return model.Conversation{}, err
	}
	if f.titleErr != nil {
		return model.Conversation{}, f.titleErr
	}
	return stored(cid, title), nil
}

// note records a step taken outside the backend, such as the agent running.
func (f *fakeBackend) note(step string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.steps = append(f.steps, step)
}

func (f *fakeBackend) DeleteConversation(ctx context.Context, cid string) error {
	return f.convCall(ctx, func() { f.deleted = append(f.deleted, cid) })
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

// asUser requests the conversation list, with a user ID unless it is empty.
func asUser(h http.Handler, user string) int {
	req := httptest.NewRequest("GET", "/api/chat/conversations?snapshot=latest", nil)
	if user != "" {
		req.Header.Set("X-User-Id", user)
	}
	return serve(h, req).Code
}
