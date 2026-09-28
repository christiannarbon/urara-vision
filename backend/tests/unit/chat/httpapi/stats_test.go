// Ported from chat/tests/unit/test_stats.py.
package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"sync"
	"testing"

	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/httpapi"
	"urara-vision/backend/internal/model"
)

const statsSnapshot = "5b0c1a52-3d8e-4f7a-9c21-6e4d8b2f1a90"

// statsBackend serves conversations by ID; a deleted one lists but 404s on fetch.
type statsBackend struct {
	*fakeBackend
	convs   map[string][]model.Message
	deleted map[string]bool
	mine    map[string]bool // when set, only these are listed, as the backend lists per user
	getErr  error
	failing string // this fetch fails with a 500; every other waits for its context

	mu      sync.Mutex
	limits  []int
	fetches int
}

func (b *statsBackend) ListConversations(_ context.Context, sid string, limit int) ([]model.Conversation, error) {
	b.mu.Lock()
	b.limits = append(b.limits, limit)
	b.mu.Unlock()
	ids := make([]string, 0, len(b.convs))
	for id := range b.convs {
		if b.mine == nil || b.mine[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	out := make([]model.Conversation, len(ids))
	for i, id := range ids {
		out[i] = model.Conversation{ID: id, SnapshotID: sid}
	}
	return out, nil
}

func (b *statsBackend) GetConversation(ctx context.Context, cid string) (model.Conversation, error) {
	b.mu.Lock()
	b.fetches++
	b.mu.Unlock()
	switch {
	case b.failing == cid:
		return model.Conversation{}, &apiclient.Error{Status: 500, Message: "upstream"}
	case b.failing != "":
		<-ctx.Done()
		return model.Conversation{}, ctx.Err()
	case b.getErr != nil:
		return model.Conversation{}, b.getErr
	case b.deleted[cid]:
		return model.Conversation{}, &apiclient.Error{Status: 404, Message: "conversation not found"}
	}
	return model.Conversation{ID: cid, SnapshotID: statsSnapshot, Messages: b.convs[cid]}, nil
}

// turnOf is one question and its answer, with meta decoded from JSON as the client would.
func turnOf(t *testing.T, metaJSON string) []model.Message {
	t.Helper()
	var meta map[string]any
	if err := json.Unmarshal([]byte(metaJSON), &meta); err != nil {
		t.Fatal(err)
	}
	return []model.Message{{Role: "user", Content: "q"}, {Role: "assistant", Content: "a", Meta: meta}}
}

func metaJSON(over string) string {
	base := map[string]any{
		"model": "gemini-2.5-flash", "promptTokens": 1000, "completionTokens": 100,
		"tokensEstimated": false, "latencyMs": 2000, "truncated": false,
	}
	if over != "" {
		_ = json.Unmarshal([]byte(over), &base)
	}
	b, _ := json.Marshal(base)
	return string(b)
}

func joined(turns ...[]model.Message) []model.Message {
	var out []model.Message
	for _, t := range turns {
		out = append(out, t...)
	}
	return out
}

func latencyTurns(t *testing.T, latencies ...int) []model.Message {
	var out []model.Message
	for _, ms := range latencies {
		out = append(out, turnOf(t, metaJSON(fmt.Sprintf(`{"latencyMs":%d}`, ms)))...)
	}
	return out
}

func getStats(t *testing.T, b *statsBackend, query string) *httptest.ResponseRecorder {
	t.Helper()
	b.fakeBackend = &fakeBackend{enabled: true}
	h := httpapi.New(httpapi.Deps{
		Settings: chatSettings(),
		Log:      slog.New(slog.NewJSONHandler(&logs{}, nil)),
		Backend:  b,
		Clock:    newClock().now,
	}).Handler()
	req := httptest.NewRequest("GET", "/api/chat/stats"+query, nil)
	req.Header.Set("X-User-Id", "user-1")
	return serve(h, req)
}

func statsOf(t *testing.T, convs map[string][]model.Message) map[string]any {
	t.Helper()
	rec := getStats(t, &statsBackend{convs: convs}, "?snapshot="+statsSnapshot)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	return decoded(t, rec)
}

func TestStatsAggregatesAcrossConversations(t *testing.T) {
	b := &statsBackend{convs: map[string][]model.Message{
		"c1": joined(turnOf(t, metaJSON("")), turnOf(t, metaJSON(`{"latencyMs":4000,"truncated":true}`))),
		"c2": turnOf(t, metaJSON(`{"promptTokens":500,"completionTokens":50,"tokensEstimated":true}`)),
	}}
	body := decoded(t, getStats(t, b, "?snapshot="+statsSnapshot))
	want := map[string]any{
		"snapshotId": statsSnapshot, "conversations": 2.0, "turns": 3.0, "promptTokens": 2500.0,
		"completionTokens": 250.0, "estimatedTokenTurns": 1.0, "meanLatencyMs": 2667.0,
		"truncatedTurns": 1.0, "conversationsCapped": false,
	}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("%s = %v, want %v", k, body[k], v)
		}
	}
	if !reflect.DeepEqual(b.limits, []int{200}) {
		t.Errorf("limits %v", b.limits)
	}
}

func TestStatsByModelCountsTwoModels(t *testing.T) {
	body := statsOf(t, map[string][]model.Message{
		"c1": joined(turnOf(t, metaJSON("")), turnOf(t, metaJSON(`{"model":"gemini-2.5-pro"}`))),
		"c2": turnOf(t, metaJSON("")),
	})
	if want := map[string]any{"gemini-2.5-flash": 2.0, "gemini-2.5-pro": 1.0}; !reflect.DeepEqual(body["byModel"], want) {
		t.Errorf("byModel %v", body["byModel"])
	}
}

func TestStatsMessageWithNoMetaCountsButAddsNothing(t *testing.T) {
	body := statsOf(t, map[string][]model.Message{"c1": joined(turnOf(t, `{}`), turnOf(t, metaJSON("")))})
	if body["turns"] != 2.0 || body["promptTokens"] != 1000.0 || body["meanLatencyMs"] != 2000.0 {
		t.Errorf("%v", body)
	}
}

func TestStatsPartialMetaContributesWhatItHas(t *testing.T) {
	body := statsOf(t, map[string][]model.Message{
		"c1": joined(turnOf(t, `{"latencyMs":3000}`), turnOf(t, metaJSON(`{"latencyMs":1000}`))),
	})
	if body["meanLatencyMs"] != 2000.0 || body["promptTokens"] != 1000.0 ||
		!reflect.DeepEqual(body["byModel"], map[string]any{"gemini-2.5-flash": 1.0}) {
		t.Errorf("%v", body)
	}
}

func TestStatsPythonEraMetaFallsBackToUsage(t *testing.T) {
	body := statsOf(t, map[string][]model.Message{
		"c1": turnOf(t, `{"model":"gemini-2.5-flash","usage":{"input_tokens":900,"output_tokens":90}}`),
	})
	if body["promptTokens"] != 900.0 || body["completionTokens"] != 90.0 || body["estimatedTokenTurns"] != nil {
		t.Errorf("%v", body)
	}
}

func TestStatsMalformedValuesAreIgnored(t *testing.T) {
	bad := `{"promptTokens":"lots","latencyMs":true,"model":7,"usage":"x","truncated":1}`
	body := statsOf(t, map[string][]model.Message{"c1": joined(turnOf(t, bad), turnOf(t, metaJSON("")))})
	if body["promptTokens"] != 1000.0 || body["meanLatencyMs"] != 2000.0 || body["truncatedTurns"] != 0.0 ||
		!reflect.DeepEqual(body["byModel"], map[string]any{"gemini-2.5-flash": 1.0}) {
		t.Errorf("%v", body)
	}
}

func TestStatsBooleansAndFractionsAreNotCounts(t *testing.T) {
	body := statsOf(t, map[string][]model.Message{
		"c1": turnOf(t, `{"promptTokens":true,"completionTokens":1.5,"usage":{"input_tokens":false,"output_tokens":-3}}`),
	})
	if body["promptTokens"] != nil || body["completionTokens"] != nil {
		t.Errorf("%v", body)
	}
	// A bad promptTokens still falls back to usage.
	body = statsOf(t, map[string][]model.Message{"c1": turnOf(t, `{"promptTokens":true,"usage":{"input_tokens":900}}`)})
	if body["promptTokens"] != 900.0 {
		t.Errorf("promptTokens %v", body["promptTokens"])
	}
}

func TestStatsMeanRoundsHalfToEven(t *testing.T) {
	for _, c := range []struct {
		latencies []int
		want      float64
	}{{[]int{1, 2}, 2}, {[]int{2, 3}, 2}, {[]int{1, 2, 2}, 2}} {
		body := statsOf(t, map[string][]model.Message{"c1": latencyTurns(t, c.latencies...)})
		if body["meanLatencyMs"] != c.want {
			t.Errorf("%v: mean %v, want %v", c.latencies, body["meanLatencyMs"], c.want)
		}
	}
}

func TestStatsP95IsNullBelowTheThreshold(t *testing.T) {
	latencies := make([]int, 19)
	for i := range latencies {
		latencies[i] = i
	}
	if body := statsOf(t, map[string][]model.Message{"c1": latencyTurns(t, latencies...)}); body["p95LatencyMs"] != nil {
		t.Errorf("p95 %v", body["p95LatencyMs"])
	}
}

func TestStatsP95IsNearestRankAtTheThreshold(t *testing.T) {
	latencies := make([]int, 20)
	for i := range latencies {
		latencies[len(latencies)-1-i] = (i + 1) * 100 // unsorted on purpose
	}
	if body := statsOf(t, map[string][]model.Message{"c1": latencyTurns(t, latencies...)}); body["p95LatencyMs"] != 1900.0 {
		t.Errorf("p95 %v", body["p95LatencyMs"])
	}
}

func TestStatsEmptySnapshotGolden(t *testing.T) {
	assertGolden(t, "stats/empty.json", getStats(t, &statsBackend{}, "?snapshot="+statsSnapshot))
}

func TestStatsSnapshotQuery(t *testing.T) {
	for query, want := range map[string]int{"": 400, "?snapshot=": 400, "?snapshot=nosuch": 404} {
		if rec := getStats(t, &statsBackend{}, query); rec.Code != want {
			t.Errorf("%q: %d, want %d", query, rec.Code, want)
		}
	}
	rec := getStats(t, &statsBackend{}, "")
	if f := firstField(t, rec); f["field"] != "snapshot" || f["location"] != "query" {
		t.Errorf("field %v", f)
	}
}

func TestStatsLatestResolves(t *testing.T) {
	rec := getStats(t, &statsBackend{convs: map[string][]model.Message{"c1": turnOf(t, metaJSON(""))}}, "?snapshot=latest")
	body := decoded(t, rec)
	if body["snapshotId"] != statsSnapshot || body["turns"] != 1.0 {
		t.Errorf("%v", body)
	}
}

func TestStatsFullListingIsCapped(t *testing.T) {
	convs := map[string][]model.Message{}
	for i := range 200 {
		convs[fmt.Sprintf("c%d", i)] = nil
	}
	body := statsOf(t, convs)
	if body["conversationsCapped"] != true || body["conversations"] != 200.0 {
		t.Errorf("capped %v, conversations %v", body["conversationsCapped"], body["conversations"])
	}
	if body := statsOf(t, map[string][]model.Message{"c1": nil}); body["conversationsCapped"] != false {
		t.Error("one conversation marked capped")
	}
}

func TestStatsConversationDeletedMidReadIsSkipped(t *testing.T) {
	b := &statsBackend{
		convs:   map[string][]model.Message{"c1": turnOf(t, metaJSON("")), "gone": turnOf(t, metaJSON(""))},
		deleted: map[string]bool{"gone": true},
	}
	rec := getStats(t, b, "?snapshot="+statsSnapshot)
	body := decoded(t, rec)
	if rec.Code != http.StatusOK || body["conversations"] != 1.0 || body["turns"] != 1.0 {
		t.Errorf("%d %v", rec.Code, body)
	}
}

func TestStatsOtherFetchErrorsFail(t *testing.T) {
	for status, want := range map[int]int{403: 403, 500: 502} {
		b := &statsBackend{
			convs:  map[string][]model.Message{"c1": nil},
			getErr: &apiclient.Error{Status: status, Message: "upstream"},
		}
		if rec := getStats(t, b, "?snapshot="+statsSnapshot); rec.Code != want {
			t.Errorf("backend %d: %d, want %d", status, rec.Code, want)
		}
	}
}

func TestStatsCountOnlyWhatTheBackendLists(t *testing.T) {
	b := &statsBackend{
		convs: map[string][]model.Message{
			"mine":   turnOf(t, metaJSON("")),
			"theirs": joined(turnOf(t, metaJSON("")), turnOf(t, metaJSON(""))),
		},
		mine: map[string]bool{"mine": true},
	}
	body := decoded(t, getStats(t, b, "?snapshot="+statsSnapshot))
	if body["conversations"] != 1.0 || body["turns"] != 1.0 {
		t.Errorf("%v", body)
	}
}

func TestStatsStopsFetchingAfterAFailure(t *testing.T) {
	convs := map[string][]model.Message{}
	for i := range 50 {
		convs[fmt.Sprintf("c%02d", i)] = nil
	}
	b := &statsBackend{convs: convs, failing: "c00"}
	if rec := getStats(t, b, "?snapshot="+statsSnapshot); rec.Code != http.StatusBadGateway {
		t.Errorf("status %d", rec.Code)
	}
	// At most the first batch: errgroup cancels before it frees a slot.
	if b.fetches > 8 {
		t.Errorf("%d fetches after the first failed", b.fetches)
	}
}
