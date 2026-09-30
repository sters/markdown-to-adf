package adf

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	verifySkipRe   = regexp.MustCompile("^(?:`{3,}|~{3,}).*$|^\\|?(?:\\s*:?-+:?\\s*\\|)+\\s*:?-*:?\\s*$")
	verifyMarkerRe = regexp.MustCompile(`^\s*(?:>\s?)*\s*(?:(?:[-*+]|\d{1,9}[.)])\s+(?:\[[ xX]\]\s+)?)?`)
	linkTargetRe   = regexp.MustCompile(`\]\([^)]*\)`)
	brRe           = regexp.MustCompile(`(?i)<br\s*/?>`)
	syntaxCharsRe  = regexp.MustCompile("[\\s*`\\\\#\\[\\]|()~<>]")
)

// LossError reports source lines whose text does not appear in the converted document.
type LossError struct {
	Missing []MissingLine
}

// MissingLine is one source line, or one table cell of it, that was not found.
type MissingLine struct {
	Line int
	Text string
}

func (e *LossError) Error() string {
	var b strings.Builder

	fmt.Fprintf(&b, "content lost in conversion (%d)", len(e.Missing))

	for _, m := range e.Missing {
		fmt.Fprintf(&b, "\n  line %d: %s", m.Line, m.Text)
	}

	return b.String()
}

// Verify checks, independently of the parser, that every non-blank line of
// markdown contributes its text to doc, in source order. It compares the source
// with Markdown syntax characters, list markers and link targets removed against
// the concatenated text nodes with the same characters removed.
//
// The check exists because a parser bug that drops text would otherwise look
// exactly like a successful conversion. It returns a *LossError on failure.
func Verify(markdown string, doc *Doc) error {
	var flatBytes []byte
	for _, n := range doc.Content {
		flatText(n, &flatBytes)
	}

	flat := normalize(string(flatBytes))

	var (
		missing []MissingLine
		cursor  int
	)

	for _, l := range splitLines(markdown) {
		t := strings.TrimSpace(l.text)
		if t == "" || verifySkipRe.MatchString(t) || ruleRe.MatchString(t) {
			continue
		}

		t = verifyMarkerRe.ReplaceAllString(t, "")
		t = linkTargetRe.ReplaceAllString(t, "]")
		t = brRe.ReplaceAllString(t, "")

		pieces := []string{t}
		if isTableRow(t) {
			pieces = strings.Split(t, "|")
		}

		for _, p := range pieces {
			norm := normalize(p)
			if norm == "" {
				continue
			}

			idx := strings.Index(flat[cursor:], norm)
			if idx < 0 {
				missing = append(missing, MissingLine{Line: l.no, Text: strings.TrimSpace(p)})

				continue
			}

			cursor += idx + len(norm)
		}
	}

	if len(missing) > 0 {
		return &LossError{Missing: missing}
	}

	return nil
}

func normalize(s string) string {
	return syntaxCharsRe.ReplaceAllString(s, "")
}
