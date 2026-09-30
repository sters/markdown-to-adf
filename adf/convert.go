package adf

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	headingRe    = regexp.MustCompile(`^(#{1,6})(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$`)
	fenceRe      = regexp.MustCompile("^(`{3,}|~{3,})[ \t]*([^\\s`]*)[^`]*$")
	ruleRe       = regexp.MustCompile(`^(?:(?:-[ \t]*){3,}|(?:\*[ \t]*){3,}|(?:_[ \t]*){3,})$`)
	listMarkerRe = regexp.MustCompile(`^( *)([-*+]|\d{1,9}[.)])( +|$)(.*)$`)
	taskBoxRe    = regexp.MustCompile(`^\[([ xX])\](?: +(.*))?$`)
	quoteRe      = regexp.MustCompile(`^ {0,3}> ?(.*)$`)
	sepCellRe    = regexp.MustCompile(`^:?-+:?$`)
)

// Convert parses markdown into an ADF document. It returns a *SyntaxError for any
// construct it does not model.
//
// Supported blocks: ATX headings, paragraphs (consecutive lines are joined with
// hard breaks), fenced code blocks, `---` rules, bullet / ordered / task lists with
// nesting by indentation, blockquotes, and pipe tables with a header separator row.
func Convert(markdown string) (*Doc, error) {
	c := &converter{}

	content, err := c.blocks(splitLines(markdown))
	if err != nil {
		return nil, err
	}

	if content == nil {
		content = []*Node{}
	}

	seq := 0
	for _, n := range content {
		assignLocalIDs(n, &seq)
	}

	return &Doc{Type: "doc", Version: 1, Content: content}, nil
}

// assignLocalIDs numbers task lists and items in document order. ADF requires a
// localId on both; it only has to be unique within the document.
func assignLocalIDs(n *Node, seq *int) {
	prefix := map[string]string{"taskList": "tl", "taskItem": "t"}[n.Type]
	if prefix != "" {
		*seq++
		n.Attrs["localId"] = prefix + "-" + strconv.Itoa(*seq)
	}

	for _, c := range n.Content {
		assignLocalIDs(c, seq)
	}
}

type line struct {
	text string
	no   int
}

type converter struct{}

func splitLines(markdown string) []line {
	markdown = strings.TrimPrefix(markdown, "\uFEFF")
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	raw := strings.Split(markdown, "\n")

	lines := make([]line, len(raw))
	for i, t := range raw {
		lines[i] = line{text: expandLeadingTabs(strings.TrimRight(t, " \t\r")), no: i + 1}
	}

	return lines
}

func expandLeadingTabs(s string) string {
	n := 0
	for n < len(s) && (s[n] == ' ' || s[n] == '\t') {
		n++
	}

	return strings.ReplaceAll(s[:n], "\t", "    ") + s[n:]
}

func (c *converter) blocks(lines []line) ([]*Node, error) {
	var out []*Node

	for i := 0; i < len(lines); {
		l := lines[i]

		var (
			node     *Node
			consumed int
			err      error
		)

		switch {
		case l.text == "":
			i++

			continue
		case fenceRe.MatchString(l.text):
			node, consumed, err = c.fence(lines[i:])
		case headingRe.MatchString(l.text):
			node, consumed, err = c.heading(l)
		case ruleRe.MatchString(strings.TrimSpace(l.text)):
			node, consumed = &Node{Type: "rule"}, 1
		case listMarkerRe.MatchString(l.text):
			node, consumed, err = c.list(lines[i:])
		case quoteRe.MatchString(l.text):
			node, consumed, err = c.quote(lines[i:])
		case isTableRow(l.text):
			node, consumed, err = c.table(lines[i:])
		case indent(l.text) > 0:
			err = &SyntaxError{l.no, "indented line outside a list; indented code blocks are not supported, use a fence"}
		default:
			node, consumed, err = c.paragraph(lines[i:])
		}

		if err != nil {
			return nil, err
		}

		if node != nil {
			out = append(out, node)
		}

		i += consumed
	}

	return out, nil
}

func (c *converter) heading(l line) (*Node, int, error) {
	m := headingRe.FindStringSubmatch(l.text)
	if strings.TrimSpace(m[2]) == "" {
		return nil, 0, &SyntaxError{l.no, "empty heading"}
	}

	content, err := inlineAt(l.no, m[2])
	if err != nil {
		return nil, 0, err
	}

	return &Node{Type: "heading", Attrs: map[string]any{"level": len(m[1])}, Content: content}, 1, nil
}

func (c *converter) fence(lines []line) (*Node, int, error) {
	m := fenceRe.FindStringSubmatch(lines[0].text)
	delim := m[1]

	var body []string

	for i := 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i].text)
		if strings.HasPrefix(t, delim) && strings.Trim(t, delim[:1]) == "" {
			node := &Node{Type: "codeBlock"}
			if m[2] != "" {
				node.Attrs = map[string]any{"language": m[2]}
			}

			if text := strings.Join(body, "\n"); text != "" {
				node.Content = []*Node{textNode(text, nil)}
			}

			return node, i + 1, nil
		}

		body = append(body, lines[i].text)
	}

	return nil, 0, &SyntaxError{lines[0].no, "unterminated code fence"}
}

func (c *converter) paragraph(lines []line) (*Node, int, error) {
	var content []*Node

	i := 0
	for ; i < len(lines); i++ {
		l := lines[i]
		if i > 0 && (l.text == "" || indent(l.text) > 0 || interruptsParagraph(l.text)) {
			break
		}

		nodes, err := inlineAt(l.no, strings.TrimSpace(l.text))
		if err != nil {
			return nil, 0, err
		}

		if i > 0 {
			content = append(content, &Node{Type: "hardBreak"})
		}

		content = append(content, nodes...)
	}

	return &Node{Type: "paragraph", Content: content}, i, nil
}

// interruptsParagraph reports whether t starts a new block. As in CommonMark, an
// ordered list only interrupts a paragraph when it starts at 1, so a line such as
// "2024. ..." continues the paragraph.
func interruptsParagraph(t string) bool {
	if m := listMarkerRe.FindStringSubmatch(t); m != nil {
		n, err := strconv.Atoi(strings.TrimRight(m[2], ".)"))

		return m[4] != "" && (err != nil || n == 1)
	}

	return fenceRe.MatchString(t) || headingRe.MatchString(t) || ruleRe.MatchString(strings.TrimSpace(t)) ||
		quoteRe.MatchString(t) || isTableRow(t)
}

func (c *converter) quote(lines []line) (*Node, int, error) {
	var body []line

	i := 0
	for ; i < len(lines); i++ {
		m := quoteRe.FindStringSubmatch(lines[i].text)
		if m == nil {
			break
		}

		body = append(body, line{text: m[1], no: lines[i].no})
	}

	content, err := c.blocks(body)
	if err != nil {
		return nil, 0, err
	}

	if len(content) == 0 {
		return nil, 0, &SyntaxError{lines[0].no, "empty blockquote"}
	}

	if err := allowOnly(lines[0].no, "blockquote", content, "paragraph", "bulletList", "orderedList", "codeBlock"); err != nil {
		return nil, 0, err
	}

	return &Node{Type: "blockquote", Content: content}, i, nil
}

type listKind int

const (
	bulletKind listKind = iota
	orderedKind
	taskKind
)

type listItem struct {
	indent int
	kind   listKind
	number int
	done   bool
	text   string
	no     int
}

func parseListItem(l line) (listItem, bool) {
	m := listMarkerRe.FindStringSubmatch(l.text)
	if m == nil || ruleRe.MatchString(strings.TrimSpace(l.text)) {
		return listItem{}, false
	}

	item := listItem{indent: len(m[1]), kind: bulletKind, text: m[4], no: l.no}

	if n, err := strconv.Atoi(strings.TrimRight(m[2], ".)")); err == nil {
		item.kind, item.number = orderedKind, n
	} else if t := taskBoxRe.FindStringSubmatch(m[4]); t != nil {
		item.kind, item.done, item.text = taskKind, t[1] != " ", t[2]
	}

	return item, true
}

// list consumes one list: consecutive items of the same kind at the same indent.
// Each item's body (its first line plus every following line indented deeper than
// its marker) is dedented and parsed recursively, which is what gives nesting.
//
// A task list nested under a task item becomes a sibling taskList inside the
// parent taskList, because that is how ADF nests task lists.
func (c *converter) list(lines []line) (*Node, int, error) {
	first, _ := parseListItem(lines[0])

	var items []*Node

	i := 0
	for i < len(lines) {
		item, ok := parseListItem(lines[i])
		if !ok || item.indent != first.indent || item.kind != first.kind {
			break
		}

		if strings.TrimSpace(item.text) == "" {
			return nil, 0, &SyntaxError{item.no, "empty list item"}
		}

		end := itemEnd(lines, i, item.indent)
		body := append([]line{{text: item.text, no: item.no}}, dedent(lines[i+1:end])...)

		content, err := c.blocks(body)
		if err != nil {
			return nil, 0, err
		}

		nodes, err := c.listItemNodes(item, content)
		if err != nil {
			return nil, 0, err
		}

		items = append(items, nodes...)
		i = end

		// Blank lines between siblings keep the list going.
		next := i
		for next < len(lines) && lines[next].text == "" {
			next++
		}

		if next >= len(lines) {
			break
		}

		if sib, ok := parseListItem(lines[next]); !ok || sib.indent != first.indent || sib.kind != first.kind {
			break
		}

		i = next
	}

	var list *Node

	switch first.kind {
	case bulletKind:
		list = &Node{Type: "bulletList", Content: items}
	case orderedKind:
		list = &Node{Type: "orderedList", Attrs: map[string]any{"order": first.number}, Content: items}
	case taskKind:
		list = &Node{Type: "taskList", Attrs: map[string]any{"localId": ""}, Content: items}
	}

	return list, i, nil
}

func (c *converter) listItemNodes(item listItem, content []*Node) ([]*Node, error) {
	if item.kind != taskKind {
		if content[0].Type != "paragraph" && content[0].Type != "codeBlock" {
			return nil, &SyntaxError{item.no, fmt.Sprintf("a list item cannot start with %s in ADF", content[0].Type)}
		}

		if err := allowOnly(item.no, "list item", content, "paragraph", "bulletList", "orderedList", "codeBlock"); err != nil {
			return nil, err
		}

		return []*Node{{Type: "listItem", Content: content}}, nil
	}

	if content[0].Type != "paragraph" {
		return nil, &SyntaxError{item.no, "task item text must be inline"}
	}

	state := "TODO"
	if item.done {
		state = "DONE"
	}

	out := []*Node{{
		Type:    "taskItem",
		Attrs:   map[string]any{"localId": "", "state": state},
		Content: content[0].Content,
	}}

	nested := content[1:]
	if err := allowOnly(item.no, "task item", nested, "taskList"); err != nil {
		return nil, err
	}

	return append(out, nested...), nil
}

// itemEnd returns the index just past the body of the item at lines[start]: the
// lines indented deeper than the marker, with blank lines included only when an
// indented line follows them.
func itemEnd(lines []line, start, markerIndent int) int {
	end := start + 1

	for j := start + 1; j < len(lines); j++ {
		if lines[j].text == "" {
			continue
		}

		if indent(lines[j].text) <= markerIndent {
			break
		}

		end = j + 1
	}

	return end
}

func dedent(lines []line) []line {
	lowest := -1

	for _, l := range lines {
		if n := indent(l.text); l.text != "" && (lowest < 0 || n < lowest) {
			lowest = n
		}
	}

	out := make([]line, len(lines))
	for i, l := range lines {
		if l.text != "" {
			l.text = l.text[lowest:]
		}

		out[i] = l
	}

	return out
}

func (c *converter) table(lines []line) (*Node, int, error) {
	n := 0
	for n < len(lines) && isTableRow(lines[n].text) {
		n++
	}

	rows := lines[:n]

	header := tableCells(rows[0].text)
	if n < 2 || !isSeparatorRow(tableCells(rows[1].text), len(header)) {
		return nil, 0, &SyntaxError{rows[0].no, "table without a header separator row matching its header"}
	}

	trows := make([]*Node, 0, n-1)

	for i, r := range append(rows[:1:1], rows[2:]...) {
		cells := tableCells(r.text)
		if len(cells) != len(header) {
			return nil, 0, &SyntaxError{r.no, fmt.Sprintf("table row has %d cells, header has %d", len(cells), len(header))}
		}

		cellType := "tableCell"
		if i == 0 {
			cellType = "tableHeader"
		}

		row := &Node{Type: "tableRow"}

		for _, text := range cells {
			para := &Node{Type: "paragraph"}

			if text != "" {
				content, err := inlineAt(r.no, text)
				if err != nil {
					return nil, 0, err
				}

				para.Content = content
			}

			row.Content = append(row.Content, &Node{Type: cellType, Content: []*Node{para}})
		}

		trows = append(trows, row)
	}

	return &Node{
		Type:    "table",
		Attrs:   map[string]any{"isNumberColumnEnabled": false, "layout": "default"},
		Content: trows,
	}, n, nil
}

func isTableRow(t string) bool {
	t = strings.TrimSpace(t)

	return len(t) >= 2 && t[0] == '|' && t[len(t)-1] == '|'
}

func isSeparatorRow(cells []string, width int) bool {
	if len(cells) != width {
		return false
	}

	for _, c := range cells {
		if !sepCellRe.MatchString(c) {
			return false
		}
	}

	return true
}

// tableCells splits a row on unescaped pipes. `\|` becomes a literal pipe in the
// cell, including inside code spans, as in GFM.
func tableCells(row string) []string {
	row = strings.TrimSpace(row)
	row = row[1 : len(row)-1]

	var (
		cells []string
		cur   strings.Builder
	)

	for i := 0; i < len(row); i++ {
		switch {
		case row[i] == '\\' && i+1 < len(row) && row[i+1] == '|':
			cur.WriteByte('|')
			i++
		case row[i] == '|':
			cells = append(cells, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteByte(row[i])
		}
	}

	return append(cells, strings.TrimSpace(cur.String()))
}

func allowOnly(no int, parent string, nodes []*Node, types ...string) error {
	for _, n := range nodes {
		if !slices.Contains(types, n.Type) {
			return &SyntaxError{no, fmt.Sprintf("%s cannot contain %s in ADF", parent, n.Type)}
		}
	}

	return nil
}

func inlineAt(no int, text string) ([]*Node, error) {
	nodes, err := parseInline(text)
	if err != nil {
		return nil, &SyntaxError{no, err.Error()}
	}

	return nodes, nil
}

func indent(t string) int {
	return len(t) - len(strings.TrimLeft(t, " "))
}
