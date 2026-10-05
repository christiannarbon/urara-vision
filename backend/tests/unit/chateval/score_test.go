package chateval_test

import (
	"testing"

	"urara-vision/backend/internal/chateval"
)

func question(category string) chateval.Question {
	return chateval.Question{
		ID: "q1", Category: category,
		ExpectCitations: []string{}, ExpectToolsAny: []string{}, ExpectContainsAny: []string{}, MustNotContain: []string{},
	}
}

func cites(c ...string) chateval.AnswerResponse {
	return chateval.AnswerResponse{Text: "x", Citations: c}
}

func ptr[T any](v T) *T { return &v }

func TestNoExpectedCitationsMeansRecallIsNotScored(t *testing.T) {
	if s := chateval.Score(question("lookup"), cites("a/b")); s.Recall != nil {
		t.Errorf("recall = %v", *s.Recall)
	}
}

// 08.7 F1: [] is "none required", not "must cite none".
func TestEmptyExpectCitationsIsNoneRequired(t *testing.T) {
	s := chateval.Score(question("lookup"), cites("a/b", "a/c"))
	if s.Recall != nil || s.Precision != nil || !s.Passed() {
		t.Errorf("got %+v, passed %v", s, s.Passed())
	}
}

func TestARefusalMayCiteTheTableItChecked(t *testing.T) {
	q := question("refusal")
	q.AllowCitations = []string{"a/b"}
	s := chateval.Score(q, cites("A/B"))
	if s.RefusalOK == nil || !*s.RefusalOK || !s.Passed() {
		t.Errorf("got %+v", s)
	}
}

func TestARefusalCitingAnythingElseFails(t *testing.T) {
	q := question("refusal")
	q.AllowCitations = []string{"a/b"}
	if s := chateval.Score(q, cites("a/c")); s.RefusalOK == nil || *s.RefusalOK {
		t.Errorf("got %+v", s)
	}
}

func TestARefusalWithAViolationFails(t *testing.T) {
	q := question("refusal")
	q.MustNotContain = []string{"Fabricated"}
	s := chateval.Score(q, chateval.AnswerResponse{Text: "a FABRICATED column"})
	if s.RefusalOK == nil || *s.RefusalOK || len(s.Violations) != 1 || s.Violations[0] != "Fabricated" {
		t.Errorf("got %+v", s)
	}
}

func TestRecallAndPrecisionCountDistinctCaseInsensitiveIDs(t *testing.T) {
	q := question("lookup")
	q.ExpectCitations = []string{"a/b", "a/c"}
	s := chateval.Score(q, cites("A/B", "a/b", "a/d"))
	if *s.Recall != 0.5 || *s.Precision != 0.5 {
		t.Errorf("recall %v, precision %v", *s.Recall, *s.Precision)
	}
}

func TestToolsAndSubstrings(t *testing.T) {
	q := question("lookup")
	q.ExpectToolsAny = []string{"get_tables", "search_model"}
	q.ExpectContainsAny = []string{"One Row Per Order"}
	a := chateval.AnswerResponse{Text: "one row per order", ToolCalls: []chateval.ToolCall{{Name: "search_model"}}}
	if s := chateval.Score(q, a); !*s.Tools || !*s.Substr {
		t.Errorf("got %+v", s)
	}
	a.ToolCalls = []chateval.ToolCall{{Name: "list_domains"}}
	a.Text = "per table"
	if s := chateval.Score(q, a); *s.Tools || *s.Substr || s.Passed() {
		t.Errorf("got %+v", s)
	}
}

func TestRecallNilPassesZeroFails(t *testing.T) {
	if !(chateval.Scores{}).Passed() {
		t.Error("nil recall failed")
	}
	if (chateval.Scores{Recall: ptr(0.0)}).Passed() {
		t.Error("zero recall passed")
	}
	if (chateval.Scores{Recall: ptr(0.99)}).Passed() {
		t.Error("partial recall passed")
	}
}
