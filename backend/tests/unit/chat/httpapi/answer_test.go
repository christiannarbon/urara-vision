package httpapi_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"urara-vision/backend/internal/chat/agent"
	"urara-vision/backend/internal/chat/httpapi"
	"urara-vision/backend/internal/model"
)

// blockingAgent holds a "block" question until released; any other answers at once.
type blockingAgent struct {
	entered chan struct{}
	release chan struct{}
}

func (a *blockingAgent) Answer(_ context.Context, question, _ string, _ []model.Message, _ string) (agent.Answer, error) {
	if question == "block" {
		a.entered <- struct{}{}
		<-a.release
	}
	return agent.Answer{Text: "ok"}, nil
}

func chatAnswerServer(t *testing.T, a httpapi.Agent, limit int) (http.Handler, *logs) {
	t.Helper()
	settings := chatSettings()
	settings.MaxQuestionChars = 4000
	settings.AnswerTimeout = time.Minute
	settings.MaxConcurrentTurns = limit
	settings.TurnAdmissionWait = 50 * time.Millisecond
	l := &logs{}
	return httpapi.New(httpapi.Deps{
		Settings: settings,
		Log:      slog.New(slog.NewJSONHandler(l, nil)),
		Backend:  &fakeBackend{enabled: true},
		Tools:    &toolStore{t: t},
		Agent:    a,
		Clock:    newClock().now,
	}).Handler(), l
}

func post(h http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-1")
	return serve(h, req)
}

func TestChatAnswerOverTheCapIs429(t *testing.T) {
	a := &blockingAgent{entered: make(chan struct{}), release: make(chan struct{})}
	h, l := chatAnswerServer(t, a, 1)

	first := make(chan *httptest.ResponseRecorder)
	go func() { first <- post(h, "/api/chat/answer", answerBody("snap-1", "block")) }()
	<-a.entered

	rec := post(h, "/api/chat/answer", answerBody("snap-1", "two"))
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "5" {
		t.Errorf("status %d, Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	body := decoded(t, rec)
	if body["detail"] != "too many turns in flight; the limit is 1. Retry in 5 seconds." {
		t.Errorf("detail %q", body["detail"])
	}
	if body["requestId"] == "" || body["requestId"] != rec.Header().Get("X-Request-Id") {
		t.Errorf("requestId %v", body["requestId"])
	}

	// /debug/answer has no limiter.
	if rec := post(h, "/debug/answer", answerBody("snap-1", "three")); rec.Code != http.StatusOK {
		t.Errorf("/debug/answer while busy: %d", rec.Code)
	}

	close(a.release)
	if rec := <-first; rec.Code != http.StatusOK {
		t.Errorf("first: %d %s", rec.Code, rec.Body)
	}

	var refused bool
	for _, line := range l.lines(t) {
		if line["msg"] == "turn refused: all slots busy" && line["limit"] == float64(1) {
			refused = true
		}
	}
	if !refused {
		t.Error("no refusal log")
	}
}

func TestChatAnswerReleasesItsSlot(t *testing.T) {
	h, _ := chatAnswerServer(t, &blockingAgent{}, 1)
	for i := range 3 {
		if rec := post(h, "/api/chat/answer", answerBody("latest", "q")); rec.Code != http.StatusOK {
			t.Fatalf("request %d: %d %s", i, rec.Code, rec.Body)
		}
	}
	// A refused question gives its slot back too.
	post(h, "/api/chat/answer", answerBody("latest", "   "))
	if rec := post(h, "/api/chat/answer", answerBody("latest", "q")); rec.Code != http.StatusOK {
		t.Errorf("after a 400: %d", rec.Code)
	}
}

func TestChatAnswerSharesTheDebugFlow(t *testing.T) {
	h, _ := chatAnswerServer(t, &blockingAgent{}, 1)
	assertGolden(t, "errors/question-empty.json", post(h, "/api/chat/answer", answerBody("snap-1", "   ")))
	assertGolden(t, "errors/snapshot-unknown.json", post(h, "/api/chat/answer", answerBody("nosuch", "hi")))
	if rec := post(h, "/api/chat/answer", `{"question":"q"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("missing snapshotId: %d", rec.Code)
	}
}

func TestChatAnswerNeedsIdentity(t *testing.T) {
	h, _ := chatAnswerServer(t, &blockingAgent{}, 1)
	req := httptest.NewRequest("POST", "/api/chat/answer", strings.NewReader(answerBody("snap-1", "q")))
	if rec := serve(h, req); rec.Code != http.StatusUnauthorized {
		t.Errorf("status %d", rec.Code)
	}
}
