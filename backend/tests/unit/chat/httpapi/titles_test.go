// Ported from chat/tests/unit/test_titles.py.
package httpapi_test

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/httpapi"
)

const (
	ellipsis        = "…"
	maxTitleRunes   = 60
	quotesOnlyCJK   = "「」"
	quotesOnlyASCII = `"""`
)

// Every character after the first few is three bytes in UTF-8.
var japanese = "fact_orders の粒度と、顧客テーブルとの関係について教えてください。" + strings.Repeat("詳しく説明してほしいです。", 6)

func TestTitleShortQuestionIsUsedWhole(t *testing.T) {
	if got := httpapi.TitleFromQuestion("What is fact_orders?"); got != "What is fact_orders?" {
		t.Errorf("%q", got)
	}
}

func TestTitleShortQuestionGetsNoEllipsis(t *testing.T) {
	if strings.Contains(httpapi.TitleFromQuestion("What is fact_orders?"), ellipsis) {
		t.Error("ellipsis")
	}
}

func TestTitleWhitespaceIsCollapsed(t *testing.T) {
	if got := httpapi.TitleFromQuestion("  What\n  is\tthe   grain?  "); got != "What is the grain?" {
		t.Errorf("%q", got)
	}
}

func TestTitleSurroundingQuotesAreStripped(t *testing.T) {
	for _, q := range []string{`"`, "'", "“", "「"} {
		t.Run(q, func(t *testing.T) {
			if got := httpapi.TitleFromQuestion(q + "What is fact_orders?" + q); !strings.HasPrefix(got, "What") {
				t.Errorf("%q", got)
			}
		})
	}
}

func TestTitleLongQuestionTrimsAtAWordBoundary(t *testing.T) {
	title := httpapi.TitleFromQuestion("What is the grain of fact_orders and how does it relate to customers?")
	head := strings.TrimSuffix(title, ellipsis)
	if !strings.HasSuffix(title, ellipsis) || utf8.RuneCountInString(title) > maxTitleRunes+1 ||
		strings.HasSuffix(head, " ") || !strings.HasSuffix(head, " to") {
		t.Errorf("%q", title)
	}
}

func TestTitleNoBoundaryMeansAHardCut(t *testing.T) {
	if got := httpapi.TitleFromQuestion(strings.Repeat("a", 100)); got != strings.Repeat("a", maxTitleRunes)+ellipsis {
		t.Errorf("%q", got)
	}
}

func TestTitleDistantBoundaryIsNotUsed(t *testing.T) {
	if got := httpapi.TitleFromQuestion("word " + strings.Repeat("x", 100)); !strings.HasPrefix(got, "word x") {
		t.Errorf("%q", got)
	}
}

func TestTitleJapaneseTruncatesToSixtyRunes(t *testing.T) {
	title := httpapi.TitleFromQuestion(japanese)
	if n := utf8.RuneCountInString(title); n != maxTitleRunes+1 {
		t.Errorf("%d runes", n)
	}
	if len(title) <= maxTitleRunes {
		t.Error("the test text is not multi-byte")
	}
}

func TestTitleJapaneseIsStillValidText(t *testing.T) {
	title := httpapi.TitleFromQuestion(japanese)
	if !utf8.ValidString(title) || strings.Contains(title, "�") || !strings.HasPrefix(title, "fact_orders の粒度") {
		t.Errorf("%q", title)
	}
}

func TestTitleShortJapaneseIsUntouched(t *testing.T) {
	if got := httpapi.TitleFromQuestion("粒度を教えてください"); got != "粒度を教えてください" {
		t.Errorf("%q", got)
	}
}

func TestTitleJapaneseWithNoSpacesIsCutHard(t *testing.T) {
	if got := httpapi.TitleFromQuestion(strings.Repeat("粒", 70)); got != strings.Repeat("粒", maxTitleRunes)+ellipsis {
		t.Errorf("%q", got)
	}
}

func titled(t *testing.T, title *string, question string) (*turnEnv, int) {
	t.Helper()
	e := newTurnEnv(t, &fakeBackend{title: title}, nil)
	return e, e.ask(question).Code
}

func TestTitleUntitledThreadIsTitledFromItsQuestion(t *testing.T) {
	e, code := titled(t, new(string), "What is the grain of fact_orders?")
	if code != http.StatusOK || !reflect.DeepEqual(e.f.titles, []string{"What is the grain of fact_orders?"}) {
		t.Errorf("%d %v", code, e.f.titles)
	}
}

func TestTitleIsSetAfterTheAnswerIsStored(t *testing.T) {
	e, _ := titled(t, new(string), "What is fact_orders?")
	if want := []string{"get", "append:user", "agent", "append:assistant", "patch"}; !reflect.DeepEqual(e.f.steps, want) {
		t.Errorf("steps %v", e.f.steps)
	}
}

func TestTitleATitledThreadIsLeftAlone(t *testing.T) {
	chosen := "Something the reader chose"
	e, _ := titled(t, &chosen, "Something completely different about payments")
	if len(e.f.titles) != 0 {
		t.Errorf("titles %v", e.f.titles)
	}
}

func TestTitleWhitespaceTitleCountsAsAbsent(t *testing.T) {
	blank := "   "
	e, _ := titled(t, &blank, "What is fact_orders?")
	if !reflect.DeepEqual(e.f.titles, []string{"What is fact_orders?"}) {
		t.Errorf("titles %v", e.f.titles)
	}
}

func TestTitleEmptyTitleIsNotWritten(t *testing.T) {
	for _, q := range []string{quotesOnlyASCII, quotesOnlyCJK} {
		t.Run(q, func(t *testing.T) {
			if httpapi.TitleFromQuestion(q) != "" {
				t.Fatal("the question derives a title")
			}
			e, code := titled(t, new(string), q)
			if code != http.StatusOK || len(e.f.titles) != 0 {
				t.Errorf("%d %v", code, e.f.titles)
			}
			if want := []string{"get", "append:user", "agent", "append:assistant"}; !reflect.DeepEqual(e.f.steps, want) {
				t.Errorf("steps %v", e.f.steps)
			}
		})
	}
}

func TestTitleFailureDoesNotFailTheTurn(t *testing.T) {
	for name, err := range map[string]error{
		"backend":    &apiclient.Error{Status: 500, Message: "titles table is on fire"},
		"unexpected": errors.New("boom"),
	} {
		t.Run(name, func(t *testing.T) {
			e := newTurnEnv(t, &fakeBackend{title: new(string), titleErr: err}, nil)
			rec := e.ask("What is fact_orders?")
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d", rec.Code)
			}
			if got := decoded(t, rec)["assistantMessage"].(map[string]any)["content"]; got != "fact_orders is one row per order." {
				t.Errorf("content %v", got)
			}
			line := e.logLine(t, "could not set the conversation title")
			if line["level"] != "WARN" || line["request_id"] == "" || line["conversation_id"] != "conv-1" {
				t.Errorf("log %v", line)
			}
		})
	}
}
