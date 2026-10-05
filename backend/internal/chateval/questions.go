// Package chateval scores the chat agent over the golden question set.
package chateval

import (
	"bytes"
	"fmt"
	"os"
	"slices"

	"gopkg.in/yaml.v3"
)

// Question is one entry in questions.yaml. A nil list was absent; [] decodes
// to an empty, non-nil one.
type Question struct {
	ID                string   `yaml:"id"`
	Set               string   `yaml:"set"`
	Language          string   `yaml:"language"`
	Question          string   `yaml:"question"`
	Category          string   `yaml:"category"`
	ExpectCitations   []string `yaml:"expect_citations"`
	ExpectToolsAny    []string `yaml:"expect_tools_any"`
	ExpectContainsAny []string `yaml:"expect_contains_any"`
	MustNotContain    []string `yaml:"must_not_contain"`
	AllowCitations    []string `yaml:"allow_citations"`
}

func Load(path string) ([]Question, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var qs []Question
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&qs); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	seen := map[string]bool{}
	for _, q := range qs {
		if seen[q.ID] {
			return nil, fmt.Errorf("%s: question id %q appears twice", path, q.ID)
		}
		seen[q.ID] = true
	}
	return qs, nil
}

// Select keeps the questions matching every non-empty filter.
func Select(qs []Question, sets, categories, ids []string) []Question {
	var out []Question
	for _, q := range qs {
		if matches(sets, q.Set) && matches(categories, q.Category) && matches(ids, q.ID) {
			out = append(out, q)
		}
	}
	return out
}

func matches(filter []string, v string) bool {
	return len(filter) == 0 || slices.Contains(filter, v)
}
