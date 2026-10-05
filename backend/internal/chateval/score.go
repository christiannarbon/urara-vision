package chateval

import (
	"encoding/json"
	"slices"
	"strings"
)

// AnswerResponse is the part of POST /api/chat/answer that eval reads.
type AnswerResponse struct {
	Text      string         `json:"text"`
	Citations []string       `json:"citations"`
	ToolCalls []ToolCall     `json:"toolCalls"`
	Model     string         `json:"model"`
	Usage     map[string]int `json:"usage"`
	LatencyMS int            `json:"latencyMs"`
}

type ToolCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// Scores leaves a check nil when the question sets no expectation for it.
type Scores struct {
	Recall     *float64 `json:"recall"`
	Precision  *float64 `json:"precision"`
	Tools      *bool    `json:"tools"`
	Substr     *bool    `json:"substr"`
	Violations []string `json:"violations"`
	RefusalOK  *bool    `json:"refusal_ok"`
}

func (s Scores) Passed() bool {
	return (s.Recall == nil || *s.Recall == 1) &&
		!isFalse(s.Tools) && !isFalse(s.Substr) &&
		len(s.Violations) == 0 && !isFalse(s.RefusalOK)
}

func isFalse(b *bool) bool { return b != nil && !*b }

// Score is Python's score(). Absent and empty lists score alike, as there.
func Score(q Question, a AnswerResponse) Scores {
	expected, actual := lowerSet(q.ExpectCitations), lowerSet(a.Citations)
	hit := 0
	for c := range expected {
		if actual[c] {
			hit++
		}
	}

	s := Scores{Violations: []string{}}
	// Nothing required means nothing to recall; a refusal's citations are judged below instead.
	if len(expected) > 0 {
		s.Recall = ratio(hit, len(expected))
		// An empty citation list claims nothing, so precision is undefined rather than zero.
		if len(actual) > 0 {
			s.Precision = ratio(hit, len(actual))
		}
	}

	// Nothing required: every turn retrieves first (08.7), so a call is not a miss.
	if len(q.ExpectToolsAny) > 0 {
		called := slices.ContainsFunc(a.ToolCalls, func(c ToolCall) bool { return slices.Contains(q.ExpectToolsAny, c.Name) })
		s.Tools = &called
	}

	lowered := strings.ToLower(a.Text)
	contains := func(sub string) bool { return strings.Contains(lowered, strings.ToLower(sub)) }
	if len(q.ExpectContainsAny) > 0 {
		found := slices.ContainsFunc(q.ExpectContainsAny, contains)
		s.Substr = &found
	}
	for _, sub := range q.MustNotContain {
		if contains(sub) {
			s.Violations = append(s.Violations, sub)
		}
	}

	// A correct refusal may name the table it checked; anything else cited is suspect.
	if q.Category == "refusal" {
		allowed := lowerSet(q.AllowCitations)
		ok := len(s.Violations) == 0
		for c := range actual {
			if !expected[c] && !allowed[c] {
				ok = false
			}
		}
		s.RefusalOK = &ok
	}
	return s
}

func lowerSet(xs []string) map[string]bool {
	out := make(map[string]bool, len(xs))
	for _, x := range xs {
		out[strings.ToLower(x)] = true
	}
	return out
}

func ratio(a, b int) *float64 {
	v := float64(a) / float64(b)
	return &v
}
