package chateval_test

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"urara-vision/backend/internal/chateval"
)

var thresholds = chateval.Thresholds{OverallCitationRecall: 0.93, RefusalAccuracy: 1.0, MaxViolations: 0}

func scored(q chateval.Question, citations ...string) chateval.Result {
	s := chateval.Score(q, cites(citations...))
	return chateval.Result{ID: q.ID, Set: "set", Category: q.Category, Run: 1, Answer: "answer", Citations: citations, Scores: &s}
}

func TestASelectionWithNoCitationExpectationsPasses(t *testing.T) {
	s := chateval.Summarise([]chateval.Result{scored(question("refusal"))})
	if s.Overall.Recall != nil {
		t.Errorf("recall = %v", *s.Overall.Recall)
	}
	if missed := chateval.CheckThresholds(s, thresholds); len(missed) != 0 {
		t.Errorf("missed %v", missed)
	}
}

func TestLowRecallStillFails(t *testing.T) {
	q := question("lookup")
	q.ExpectCitations = []string{"a/b", "a/c"}
	missed := chateval.CheckThresholds(chateval.Summarise([]chateval.Result{scored(q, "a/b")}), thresholds)
	if want := []string{"overall citation recall 0.50 < 0.93"}; !reflect.DeepEqual(missed, want) {
		t.Errorf("missed %q, want %q", missed, want)
	}
}

func TestEveryThresholdIsChecked(t *testing.T) {
	q := question("refusal")
	q.MustNotContain = []string{"x"}
	results := []chateval.Result{scored(q), {ID: "q2", Category: "lookup", Run: 1, Error: "502"}}
	want := []string{"refusal accuracy 0.00 < 1.0", "violations 1 > 0", "1 question(s) failed to get an answer"}
	if missed := chateval.CheckThresholds(chateval.Summarise(results), thresholds); !reflect.DeepEqual(missed, want) {
		t.Errorf("missed %q", missed)
	}
}

func TestLoadThresholds(t *testing.T) {
	got, err := chateval.LoadThresholds(evalDir + "/thresholds.yaml")
	if err != nil || got != thresholds {
		t.Errorf("got %+v, %v", got, err)
	}
	path := t.TempDir() + "/t.yaml"
	_ = os.WriteFile(path, []byte("overall_citation_recall: 0.9\nrefusal_accuracy: 1.0\n"), 0o600)
	if _, err := chateval.LoadThresholds(path); err == nil || !strings.Contains(err.Error(), "max_violations") {
		t.Errorf("err = %v", err)
	}
}

// The same results as testdata/report.txt, which Python's report() printed.
func TestReportMatchesPython(t *testing.T) {
	lookup := chateval.Question{ID: "jaffle-lookup", Category: "lookup",
		ExpectCitations: []string{"a/b", "a/c"}, ExpectToolsAny: []string{"get_tables"}, ExpectContainsAny: []string{"grain"}}
	refusal := chateval.Question{ID: "jaffle-refusal", Category: "refusal",
		MustNotContain: []string{"fabricated"}, AllowCitations: []string{"a/b"}}
	traversal := chateval.Question{ID: "jaffle-traversal", Category: "traversal",
		ExpectCitations: []string{"a/b"}, ExpectToolsAny: []string{"find_join_paths"}}

	result := func(q chateval.Question, run int, a chateval.AnswerResponse, wall int, err string) chateval.Result {
		r := chateval.Result{ID: q.ID, Category: q.Category, Run: run, Usage: a.Usage, WallMS: wall, Error: err}
		if err == "" {
			s := chateval.Score(q, a)
			r.Scores = &s
		}
		return r
	}
	answer := func(text string, citations []string, usage map[string]int, tools ...string) chateval.AnswerResponse {
		a := chateval.AnswerResponse{Text: text, Citations: citations, Usage: usage}
		for _, name := range tools {
			a.ToolCalls = append(a.ToolCalls, chateval.ToolCall{Name: name})
		}
		return a
	}
	usage := func(in, out int) map[string]int { return map[string]int{"input_tokens": in, "output_tokens": out} }

	results := []chateval.Result{
		result(lookup, 1, answer("The grain is one row per order", []string{"A/B", "a/c", "a/d"}, usage(1234567, 890), "get_tables"), 3000, ""),
		result(refusal, 1, answer("Not documented; fabricated", []string{"a/b"}, usage(100, 10)), 1500, ""),
		result(traversal, 1, chateval.AnswerResponse{}, 2000, "HTTPStatusError: 502 the language model did not answer"),
		result(lookup, 2, answer("no", []string{"a/b"}, usage(1000, 100), "search_model"), 2500, ""),
		result(refusal, 2, answer("I cannot find it", nil, nil), 1200, ""),
		result(traversal, 2, answer("join on id", []string{"a/b"}, usage(500, 50), "find_join_paths"), 1800, ""),
	}

	var got strings.Builder
	chateval.Report(&got, chateval.Summarise(results), 2, 62500*time.Millisecond)
	want, err := os.ReadFile("testdata/report.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != string(want) {
		t.Errorf("report differs from Python's\n got:\n%s\nwant:\n%s", got.String(), want)
	}
}

func TestFormat(t *testing.T) {
	if chateval.Format(nil) != "--" || chateval.Format(ptr(2.0/3)) != "0.67" {
		t.Error("format")
	}
	if m := chateval.Mean(nil); m != nil {
		t.Errorf("mean of nothing = %v", *m)
	}
}
