package httpapi

import (
	"errors"
	"math"
	"net/http"
	"slices"

	"golang.org/x/sync/errgroup"

	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/model"
)

// Per user: the backend lists only the caller's conversations, and each is fetched.
// Fine at this scale; the fix beyond it is a backend aggregate.
const (
	statsConversationLimit = 200 // the backend's own list cap
	statsFetchConcurrency  = 8
	minP95Samples          = 20 // nearest-rank p95 over fewer is not a percentile
)

// statsResponse is Python's StatsResponse; nil means nothing was recorded.
type statsResponse struct {
	SnapshotID          string         `json:"snapshotId"`
	Conversations       int            `json:"conversations"`
	ConversationsCapped bool           `json:"conversationsCapped"`
	Turns               int            `json:"turns"`
	PromptTokens        *int           `json:"promptTokens"`
	CompletionTokens    *int           `json:"completionTokens"`
	EstimatedTokenTurns *int           `json:"estimatedTokenTurns"`
	MeanLatencyMS       *int           `json:"meanLatencyMs"`
	P95LatencyMS        *int           `json:"p95LatencyMs"`
	TruncatedTurns      *int           `json:"truncatedTurns"`
	ByModel             map[string]int `json:"byModel"`
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if problems := snapshotProblems(q); problems != nil {
		FieldError(w, r, problems...)
		return
	}
	ctx := r.Context()
	snapshotID, err := s.backend.ResolveSnapshot(ctx, q.Get("snapshot"))
	if err != nil {
		s.RenderBackendError(w, r, err)
		return
	}
	listed, err := s.backend.ListConversations(ctx, snapshotID, statsConversationLimit)
	if err != nil {
		s.RenderBackendError(w, r, err)
		return
	}

	fetched := make([]*model.Conversation, len(listed))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(statsFetchConcurrency)
	for i, c := range listed {
		g.Go(func() error {
			conv, err := s.backend.GetConversation(gctx, c.ID)
			switch {
			case errors.Is(err, apiclient.ErrNotFound):
				return nil // deleted after it was listed
			case err != nil:
				return err
			}
			fetched[i] = &conv
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		s.RenderBackendError(w, r, err)
		return
	}
	var convs []model.Conversation
	for _, c := range fetched {
		if c != nil {
			convs = append(convs, *c)
		}
	}
	writeJSON(w, http.StatusOK, aggregateStats(snapshotID, convs, len(listed) >= statsConversationLimit))
}

func aggregateStats(snapshotID string, convs []model.Conversation, capped bool) statsResponse {
	out := statsResponse{SnapshotID: snapshotID, Conversations: len(convs), ConversationsCapped: capped, ByModel: map[string]int{}}
	var prompt, completion, latencies []int
	var estimated, truncated, recordedEstimated, recordedTruncated int

	for _, c := range convs {
		for _, m := range c.Messages {
			if m.Role != model.RoleAssistant {
				continue
			}
			out.Turns++
			usage, _ := m.Meta["usage"].(map[string]any)

			// Python-era messages carry only the provider's usage block.
			if n, ok := count(m.Meta["promptTokens"], usage["input_tokens"]); ok {
				prompt = append(prompt, n)
			}
			if n, ok := count(m.Meta["completionTokens"], usage["output_tokens"]); ok {
				completion = append(completion, n)
			}
			if n, ok := count(m.Meta["latencyMs"]); ok {
				latencies = append(latencies, n)
			}
			if name, _ := m.Meta["model"].(string); name != "" {
				out.ByModel[name]++
			}
			if b, ok := m.Meta["tokensEstimated"].(bool); ok {
				recordedEstimated++
				if b {
					estimated++
				}
			}
			if b, ok := m.Meta["truncated"].(bool); ok {
				recordedTruncated++
				if b {
					truncated++
				}
			}
		}
	}

	if len(prompt) > 0 {
		out.PromptTokens = ptr(sum(prompt))
	}
	if len(completion) > 0 {
		out.CompletionTokens = ptr(sum(completion))
	}
	if recordedEstimated > 0 {
		out.EstimatedTokenTurns = ptr(estimated)
	}
	if recordedTruncated > 0 {
		out.TruncatedTurns = ptr(truncated)
	}
	if len(latencies) > 0 {
		// Python's round(): half to even.
		out.MeanLatencyMS = ptr(int(math.RoundToEven(float64(sum(latencies)) / float64(len(latencies)))))
	}
	if len(latencies) >= minP95Samples {
		sorted := slices.Sorted(slices.Values(latencies))
		out.P95LatencyMS = ptr(sorted[int(math.Ceil(0.95*float64(len(sorted))))-1])
	}
	return out
}

// count is the first non-negative whole number; booleans and malformed values are skipped.
func count(candidates ...any) (int, bool) {
	for _, v := range candidates {
		switch n := v.(type) {
		case float64: // what json.Unmarshal makes of every number
			if n >= 0 && n == math.Trunc(n) {
				return int(n), true
			}
		case int:
			if n >= 0 {
				return n, true
			}
		}
	}
	return 0, false
}

func sum(xs []int) int {
	total := 0
	for _, x := range xs {
		total += x
	}
	return total
}

func ptr(n int) *int { return &n }
