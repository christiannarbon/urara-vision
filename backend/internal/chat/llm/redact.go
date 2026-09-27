package llm

import (
	"regexp"
	"strings"
)

// maxReasonChars counts characters, not bytes, as Python's slice does.
const maxReasonChars = 300

var keyShaped = regexp.MustCompile(`AIza[0-9A-Za-z_\-]{10,}|sk-ant-[A-Za-z0-9_\-]{10,}|sk-[A-Za-z0-9_\-]{20,}`)

// Redact strips the question and key-shaped strings from a provider message
// before it is logged.
func Redact(text, question string) string {
	// Shorter questions are ordinary words; replacing them would shred the reason.
	if len([]rune(question)) >= 8 {
		text = strings.ReplaceAll(text, question, "[question]")
	}
	text = keyShaped.ReplaceAllString(text, "[key]")
	if r := []rune(text); len(r) > maxReasonChars {
		text = string(r[:maxReasonChars])
	}
	return text
}
