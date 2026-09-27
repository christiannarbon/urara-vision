// Ported from the redaction cases in chat/tests/unit/test_turn.py, plus new key shapes.
package llm_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"urara-vision/backend/internal/chat/llm"
)

func TestTheReasonIsKeptWithoutTheKey(t *testing.T) {
	got := llm.Redact("429 quota exceeded for key AIza-shaped-thing-0123", "q")
	if !strings.Contains(got, "429 quota exceeded") || strings.Contains(got, "AIza-shaped-thing") || !strings.Contains(got, "[key]") {
		t.Errorf("Redact = %q", got)
	}
}

func TestAQuotedQuestionIsRemoved(t *testing.T) {
	q := "what is the grain of fact_orders?"
	got := llm.Redact("400 invalid request: "+q, q)
	if strings.Contains(got, q) || !strings.Contains(got, "[question]") {
		t.Errorf("Redact = %q", got)
	}
}

func TestAShortQuestionIsLeftAlone(t *testing.T) {
	if got := llm.Redact("the model said no", "no"); got != "the model said no" {
		t.Errorf("Redact = %q", got)
	}
}

func TestOtherKeyShapes(t *testing.T) {
	for _, key := range []string{"sk-ant-api03-abcdefghij_KL", "sk-proj-abcdefghijklmnopqrstuv"} {
		if got := llm.Redact("bad key "+key+" given", ""); strings.Contains(got, key) || !strings.Contains(got, "[key]") {
			t.Errorf("Redact(%q) = %q", key, got)
		}
	}
	if got := llm.Redact("sk-short is fine", ""); got != "sk-short is fine" {
		t.Errorf("a short sk- word was redacted: %q", got)
	}
}

// Counted in characters: a Japanese question is replaced at 8 characters, not 8 bytes.
func TestLengthsCountCharacters(t *testing.T) {
	q := "注文の粒度は何ですか"
	if got := llm.Redact("400: "+q, q); strings.Contains(got, q) {
		t.Errorf("a %d-character question was kept: %q", utf8.RuneCountInString(q), got)
	}
	short := "粒度は何"
	if got := llm.Redact("400: "+short, short); !strings.Contains(got, short) {
		t.Errorf("a 4-character question was replaced: %q", got)
	}

	got := llm.Redact(strings.Repeat("あ", 400), "")
	if n := utf8.RuneCountInString(got); n != 300 || !utf8.ValidString(got) {
		t.Errorf("cut to %d characters, valid UTF-8 %v", n, utf8.ValidString(got))
	}
}
