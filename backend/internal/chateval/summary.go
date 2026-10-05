package chateval

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Result is one question asked once. Error is set instead of Scores when
// the question got no answer.
type Result struct {
	ID        string         `json:"id"`
	Set       string         `json:"set"`
	Category  string         `json:"category"`
	Language  string         `json:"language"`
	Question  string         `json:"question"`
	Run       int            `json:"run"`
	Answer    string         `json:"answer"`
	Citations []string       `json:"citations"`
	ToolCalls []ToolCall     `json:"tool_calls"`
	Model     string         `json:"model"`
	Usage     map[string]int `json:"usage"`
	LatencyMS int            `json:"latency_ms"`
	WallMS    int            `json:"wall_ms"`
	Error     string         `json:"error,omitempty"`
	Scores    *Scores        `json:"scores"`
}

type Row struct {
	N          int      `json:"n"`
	Recall     *float64 `json:"recall"`
	Precision  *float64 `json:"precision"`
	Tools      *float64 `json:"tools"`
	Substr     *float64 `json:"substr"`
	Violations int      `json:"violations"`
}

type Violation struct {
	ID      string   `json:"id"`
	Run     int      `json:"run"`
	Strings []string `json:"strings"`
}

type Failure struct {
	ID    string `json:"id"`
	Run   int    `json:"run"`
	Error string `json:"error"`
}

type QuestionStats struct {
	Runs        int      `json:"runs"`
	Passes      int      `json:"passes"`
	Errors      int      `json:"errors"`
	RecallMean  *float64 `json:"recall_mean"`
	RecallStdev float64  `json:"recall_stdev"`
}

// Summary has the keys of Python's summarise() dict.
type Summary struct {
	Categories      map[string]Row           `json:"categories"`
	Overall         Row                      `json:"overall"`
	RefusalAccuracy *float64                 `json:"refusal_accuracy"`
	Violations      []Violation              `json:"violations"`
	Errors          []Failure                `json:"errors"`
	PerQuestion     map[string]QuestionStats `json:"per_question"`
	TokensIn        int                      `json:"tokens_in"`
	TokensOut       int                      `json:"tokens_out"`
	MeanWallMS      *float64                 `json:"mean_wall_ms"`
}

func Summarise(results []Result) Summary {
	s := Summary{
		Categories: map[string]Row{}, PerQuestion: map[string]QuestionStats{},
		Violations: []Violation{}, Errors: []Failure{},
	}
	byCategory, byQuestion := map[string][]Result{}, map[string][]Result{}
	var refusals, walls []float64
	for _, r := range results {
		byCategory[r.Category] = append(byCategory[r.Category], r)
		byQuestion[r.ID] = append(byQuestion[r.ID], r)
		if r.Scores != nil {
			if r.Scores.RefusalOK != nil {
				refusals = append(refusals, boolFloat(*r.Scores.RefusalOK))
			}
			if len(r.Scores.Violations) > 0 {
				s.Violations = append(s.Violations, Violation{r.ID, r.Run, r.Scores.Violations})
			}
		}
		if r.Error != "" {
			s.Errors = append(s.Errors, Failure{r.ID, r.Run, r.Error})
		}
		s.TokensIn += r.Usage["input_tokens"]
		s.TokensOut += r.Usage["output_tokens"]
		walls = append(walls, float64(r.WallMS))
	}

	for c, rs := range byCategory {
		s.Categories[c] = row(rs)
	}
	s.Overall = row(results)
	s.RefusalAccuracy = Mean(refusals)
	for id, rs := range byQuestion {
		var recalls []float64
		q := QuestionStats{Runs: len(rs)}
		for _, r := range rs {
			if r.Scores != nil {
				if r.Scores.Recall != nil {
					recalls = append(recalls, *r.Scores.Recall)
				}
				if r.Scores.Passed() {
					q.Passes++
				}
			}
			if r.Error != "" {
				q.Errors++
			}
		}
		q.RecallMean = Mean(recalls)
		if len(recalls) > 1 {
			q.RecallStdev = pstdev(recalls)
		}
		s.PerQuestion[id] = q
	}
	s.MeanWallMS = Mean(walls)
	return s
}

func row(rs []Result) Row {
	var recall, precision, tools, substr []float64
	out := Row{N: len(rs)}
	for _, r := range rs {
		x := r.Scores
		if x == nil {
			continue
		}
		if x.Recall != nil {
			recall = append(recall, *x.Recall)
		}
		if x.Precision != nil {
			precision = append(precision, *x.Precision)
		}
		if x.Tools != nil {
			tools = append(tools, boolFloat(*x.Tools))
		}
		if x.Substr != nil {
			substr = append(substr, boolFloat(*x.Substr))
		}
		out.Violations += len(x.Violations)
	}
	out.Recall, out.Precision, out.Tools, out.Substr = Mean(recall), Mean(precision), Mean(tools), Mean(substr)
	return out
}

// Mean is nil for no values.
func Mean(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	m := sum / float64(len(values))
	return &m
}

func pstdev(values []float64) float64 {
	m := *Mean(values)
	ss := 0.0
	for _, v := range values {
		ss += (v - m) * (v - m)
	}
	return math.Sqrt(ss / float64(len(values)))
}

func boolFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// Format is Python's fmt(): two decimals, or "--" for nil.
func Format(v *float64) string {
	if v == nil {
		return "--"
	}
	return fmt.Sprintf("%.2f", *v)
}

// Report writes the text table Python's report() printed.
func Report(w io.Writer, s Summary, repeat int, wall time.Duration) {
	fmt.Fprintf(w, "\n%-14s%4s%9s%11s%8s%8s%12s\n", "category", "n", "recall", "precision", "tools", "substr", "violations")
	line := func(name string, r Row) {
		fmt.Fprintf(w, "%-14s%4d%9s%11s%8s%8s%12d\n",
			name, r.N, Format(r.Recall), Format(r.Precision), Format(r.Tools), Format(r.Substr), r.Violations)
	}
	for _, c := range sortedKeys(s.Categories) {
		line(c, s.Categories[c])
	}
	line("overall", s.Overall)
	fmt.Fprintf(w, "\nrefusal accuracy: %s\n", Format(s.RefusalAccuracy))

	if len(s.Violations) > 0 {
		fmt.Fprint(w, "\nVIOLATIONS\n")
		for _, v := range s.Violations {
			fmt.Fprintf(w, "  %s (run %d): %s\n", v.ID, v.Run, strings.Join(v.Strings, ", "))
		}
	}
	if len(s.Errors) > 0 {
		fmt.Fprint(w, "\nERRORS\n")
		for _, e := range s.Errors {
			fmt.Fprintf(w, "  %s (run %d): %s\n", e.ID, e.Run, e.Error)
		}
	}

	if repeat > 1 {
		fmt.Fprintf(w, "\n%-44s%8s%9s%8s\n", "question", "passes", "recall", "spread")
		for _, id := range sortedKeys(s.PerQuestion) {
			q := s.PerQuestion[id]
			fmt.Fprintf(w, "%-44s%4d/%-3d%9s%8s\n", id, q.Passes, q.Runs, Format(q.RecallMean), Format(&q.RecallStdev))
		}
	}

	// Python's round() is half to even.
	total := int(math.RoundToEven(wall.Seconds()))
	meanS := 0.0
	if s.MeanWallMS != nil {
		meanS = *s.MeanWallMS / 1000
	}
	fmt.Fprintf(w, "\ntokens: %s in / %s out    wall: %dm%02ds    mean %.1fs/question\n",
		thousands(s.TokensIn), thousands(s.TokensOut), total/60, total%60, meanS)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0 && s[i-1] != '-'; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

type Thresholds struct {
	OverallCitationRecall float64 `yaml:"overall_citation_recall" json:"overall_citation_recall"`
	RefusalAccuracy       float64 `yaml:"refusal_accuracy" json:"refusal_accuracy"`
	MaxViolations         int     `yaml:"max_violations" json:"max_violations"`
}

// LoadThresholds refuses a file missing a key: a zero threshold passes everything.
func LoadThresholds(path string) (Thresholds, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Thresholds{}, err
	}
	var present map[string]any
	var t Thresholds
	if err := yaml.Unmarshal(raw, &present); err != nil {
		return Thresholds{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	for _, k := range []string{"overall_citation_recall", "refusal_accuracy", "max_violations"} {
		if _, ok := present[k]; !ok {
			return Thresholds{}, errors.New(path + ": " + k + " is missing")
		}
	}
	if err := yaml.Unmarshal(raw, &t); err != nil {
		return Thresholds{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return t, nil
}

// CheckThresholds lists every threshold the summary missed.
func CheckThresholds(s Summary, t Thresholds) []string {
	var missed []string
	// nil when the selection expects no citations; that is not a failure to cite.
	if r := s.Overall.Recall; r != nil && *r < t.OverallCitationRecall {
		missed = append(missed, fmt.Sprintf("overall citation recall %.2f < %s", *r, pyFloat(t.OverallCitationRecall)))
	}
	if r := s.RefusalAccuracy; r != nil && *r < t.RefusalAccuracy {
		missed = append(missed, fmt.Sprintf("refusal accuracy %.2f < %s", *r, pyFloat(t.RefusalAccuracy)))
	}
	if v := s.Overall.Violations; v > t.MaxViolations {
		missed = append(missed, fmt.Sprintf("violations %d > %d", v, t.MaxViolations))
	}
	if len(s.Errors) > 0 {
		missed = append(missed, fmt.Sprintf("%d question(s) failed to get an answer", len(s.Errors)))
	}
	return missed
}

// pyFloat prints a float as Python's str() does for these values: 1.0, 0.93.
func pyFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}
