package adf

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var (
	errEmptyCodeSpan    = errors.New("empty code span")
	errCodeInFormatting = errors.New("a code span inside **, * or ~~ cannot be represented in ADF")
	errTripleAsterisk   = errors.New("***...*** is not supported; nest ** and * explicitly")
	errNestedLink       = errors.New("a link inside a link")
	errEmptyLinkLabel   = errors.New("link with an empty label")
	errEmptyLinkURL     = errors.New("link with an empty URL")
	errLinkTitle        = errors.New("link titles and spaces in link URLs are not supported")
	errImage            = errors.New("images are not supported")
	errNested           = errors.New("nested")
)

var (
	brTag    = regexp.MustCompile(`(?i)^<br\s*/?>`)
	autolink = regexp.MustCompile(`^<(https?://[^\s<>]+)>`)
	bareURL  = regexp.MustCompile(`^https?://[A-Za-z0-9\-._~:/?#@!$&'()+,;=%]+`)
)

// parseInline converts one line of inline Markdown into ADF inline nodes.
//
// Supported: `code`, **strong**, *em*, ~~strike~~, [label](url), <url>, bare
// http(s) URLs, <br>, and backslash escapes. A delimiter without a matching closer
// stays literal text, as in CommonMark, so `name-*` or a stray backtick survive.
// Formatting ADF cannot represent is an error.
func parseInline(s string) ([]*Node, error) {
	return parseSpan(s, nil)
}

//nolint:gocyclo // one flat dispatch over the inline constructs is easier to follow than a split one.
func parseSpan(s string, marks []Mark) ([]*Node, error) {
	var (
		out []*Node
		buf strings.Builder
	)

	flush := func() {
		if buf.Len() > 0 {
			out = append(out, textNode(buf.String(), marks))
			buf.Reset()
		}
	}

	for i := 0; i < len(s); {
		c := s[i]

		switch {
		case c == '\\' && i+1 < len(s) && isASCIIPunct(s[i+1]):
			buf.WriteByte(s[i+1])
			i += 2

		case c == '`':
			n := runLen(s, i, '`')

			end := findBacktickRun(s, i+n, n)
			if end < 0 {
				buf.WriteString(s[i : i+n])
				i += n

				continue
			}

			code := trimCodeSpan(s[i+n : end])
			if code == "" {
				return nil, errEmptyCodeSpan
			}

			if hasMark(marks, "strong", "em", "strike") {
				return nil, errCodeInFormatting
			}

			flush()
			out = append(out, textNode(code, withMark(marks, Mark{Type: "code"})))
			i = end + n

		case c == '*' || c == '~':
			n := runLen(s, i, c)

			end := -1
			if isOpener(s, i, n, c) {
				end = findCloser(s, i+n, c, n)
			}

			if end < 0 {
				buf.WriteString(s[i : i+n])
				i += n

				continue
			}

			if n >= 3 {
				return nil, errTripleAsterisk
			}

			mark := map[string]string{"*": "em", "**": "strong", "~~": "strike"}[s[i:i+n]]

			if hasMark(marks, mark) {
				return nil, fmt.Errorf("%w %s", errNested, s[i:i+n])
			}

			inner, err := parseSpan(s[i+n:end], withMark(marks, Mark{Type: mark}))
			if err != nil {
				return nil, err
			}

			flush()
			out = append(out, inner...)
			i = end + n

		case c == '!' && i+1 < len(s) && s[i+1] == '[':
			if _, ok := parseLink(s, i+1); ok {
				return nil, errImage
			}

			buf.WriteByte(c)
			i++

		case c == '[':
			l, ok := parseLink(s, i)
			if !ok {
				buf.WriteByte(c)
				i++

				continue
			}

			switch {
			case hasMark(marks, "link"):
				return nil, errNestedLink
			case l.label == "":
				return nil, errEmptyLinkLabel
			case l.href == "":
				return nil, errEmptyLinkURL
			case strings.ContainsAny(l.href, " \t"):
				return nil, errLinkTitle
			}

			inner, err := parseSpan(l.label, withMark(marks, linkMark(l.href)))
			if err != nil {
				return nil, err
			}

			flush()
			out = append(out, inner...)
			i = l.end

		case c == '<':
			if m := brTag.FindString(s[i:]); m != "" {
				flush()
				out = append(out, &Node{Type: "hardBreak"})
				i += len(m)

				continue
			}

			if m := autolink.FindStringSubmatch(s[i:]); m != nil {
				if hasMark(marks, "link") {
					return nil, errNestedLink
				}

				flush()
				out = append(out, textNode(m[1], withMark(marks, linkMark(m[1]))))
				i += len(m[0])

				continue
			}

			buf.WriteByte(c)
			i++

		case c == 'h' && (i == 0 || !isASCIIAlnum(s[i-1])) && !hasMark(marks, "link"):
			url := trimURL(bareURL.FindString(s[i:]))
			if url == "" {
				buf.WriteByte(c)
				i++

				continue
			}

			flush()
			out = append(out, textNode(url, withMark(marks, linkMark(url))))
			i += len(url)

		default:
			buf.WriteByte(c)
			i++
		}
	}

	flush()

	return out, nil
}

// isOpener reports whether the delimiter run at s[i:i+n] opens a span. A run
// followed by whitespace or the end of the line is literal, so `a * b` and
// `a ** b` stay text.
func isOpener(s string, i, n int, c byte) bool {
	if c == '~' && n != 2 {
		return false
	}

	return i+n < len(s) && !isSpace(s[i+n])
}

// findCloser returns the index of the run of exactly n c's that closes a span
// opened before from, skipping escapes and code spans. A closer must follow a
// non-space character.
func findCloser(s string, from int, c byte, n int) int {
	for j := from; j < len(s); {
		switch s[j] {
		case '\\':
			j += 2
		case '`':
			m := runLen(s, j, '`')
			if end := findBacktickRun(s, j+m, m); end >= 0 {
				j = end + m
			} else {
				j += m
			}
		case c:
			m := runLen(s, j, c)
			if m == n && !isSpace(s[j-1]) {
				return j
			}

			j += m
		default:
			j++
		}
	}

	return -1
}

type link struct {
	label string
	href  string
	end   int
}

// parseLink parses [label](href) starting at the '[' at s[i].
func parseLink(s string, i int) (link, bool) {
	closeBracket := matchBracket(s, i, '[', ']')
	if closeBracket < 0 || closeBracket+1 >= len(s) || s[closeBracket+1] != '(' {
		return link{}, false
	}

	closeParen := matchBracket(s, closeBracket+1, '(', ')')
	if closeParen < 0 {
		return link{}, false
	}

	href := strings.TrimSpace(s[closeBracket+2 : closeParen])
	href = strings.TrimSuffix(strings.TrimPrefix(href, "<"), ">")

	return link{label: s[i+1 : closeBracket], href: href, end: closeParen + 1}, true
}

func matchBracket(s string, i int, open, closing byte) int {
	depth := 0

	for j := i; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '`':
			m := runLen(s, j, '`')
			if end := findBacktickRun(s, j+m, m); end >= 0 {
				j = end + m - 1
			} else {
				j += m - 1
			}
		case open:
			depth++
		case closing:
			depth--
			if depth == 0 {
				return j
			}
		}
	}

	return -1
}

func findBacktickRun(s string, from, n int) int {
	for j := from; j < len(s); {
		if s[j] != '`' {
			j++

			continue
		}

		m := runLen(s, j, '`')
		if m == n {
			return j
		}

		j += m
	}

	return -1
}

// trimCodeSpan strips one leading and one trailing space when both are present,
// as CommonMark does, so a span can start or end with a backtick.
func trimCodeSpan(code string) string {
	if len(code) >= 2 && code[0] == ' ' && code[len(code)-1] == ' ' && strings.Trim(code, " ") != "" {
		return code[1 : len(code)-1]
	}

	return code
}

// trimURL drops trailing punctuation that ends a sentence rather than the URL,
// and a closing parenthesis that has no opening one inside the URL.
func trimURL(url string) string {
	for url != "" {
		last := url[len(url)-1]

		switch {
		case strings.IndexByte(".,:;!?'", last) >= 0:
			url = url[:len(url)-1]
		case last == ')' && strings.Count(url, "(") < strings.Count(url, ")"):
			url = url[:len(url)-1]
		default:
			if strings.HasSuffix(url, "://") {
				return ""
			}

			return url
		}
	}

	return ""
}

func linkMark(href string) Mark {
	return Mark{Type: "link", Attrs: map[string]any{"href": href}}
}

func withMark(marks []Mark, m Mark) []Mark {
	return append(slices.Clip(marks), m)
}

func hasMark(marks []Mark, types ...string) bool {
	for _, m := range marks {
		if slices.Contains(types, m.Type) {
			return true
		}
	}

	return false
}

func runLen(s string, i int, c byte) int {
	n := 0
	for i+n < len(s) && s[i+n] == c {
		n++
	}

	return n
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t'
}

func isASCIIAlnum(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
}

func isASCIIPunct(c byte) bool {
	return strings.IndexByte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", c) >= 0
}
