package httpapi

import (
	"strings"
	"unicode/utf8"
)

const (
	maxTitleRunes       = 60
	titleBoundaryWindow = 15 // how far back a word boundary is worth cutting at
)

// Quotes a pasted question tends to arrive wrapped in.
const titleQuotes = "\"'“”‘’「」『』"

// TitleFromQuestion derives a conversation title from its first question.
func TitleFromQuestion(q string) string {
	collapsed := strings.TrimSpace(strings.Trim(strings.Join(strings.Fields(q), " "), titleQuotes))
	if utf8.RuneCountInString(collapsed) <= maxTitleRunes {
		return collapsed
	}
	head := []rune(collapsed)[:maxTitleRunes]
	if boundary := lastSpace(head); boundary >= maxTitleRunes-titleBoundaryWindow {
		head = head[:boundary]
	}
	return strings.TrimRight(string(head), " ") + "…"
}

func lastSpace(runes []rune) int {
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] == ' ' {
			return i
		}
	}
	return -1
}
