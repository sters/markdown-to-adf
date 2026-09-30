package adf

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestConvert(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		md   string
		want string
	}{
		{
			name: "empty",
			md:   "",
			want: `[]`,
		},
		{
			name: "heading with closing hashes",
			md:   "## Title ##",
			want: `[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Title"}]}]`,
		},
		{
			name: "consecutive lines join with hard breaks",
			md:   "one\ntwo\n\nthree",
			want: `[{"type":"paragraph","content":[{"type":"text","text":"one"},{"type":"hardBreak"},{"type":"text","text":"two"}]},` +
				`{"type":"paragraph","content":[{"type":"text","text":"three"}]}]`,
		},
		{
			name: "inline marks",
			md:   "a **b** *c* ~~d~~ `e` [f](https://x.test/g)",
			want: `[{"type":"paragraph","content":[{"type":"text","text":"a "},` +
				`{"type":"text","text":"b","marks":[{"type":"strong"}]},{"type":"text","text":" "},` +
				`{"type":"text","text":"c","marks":[{"type":"em"}]},{"type":"text","text":" "},` +
				`{"type":"text","text":"d","marks":[{"type":"strike"}]},{"type":"text","text":" "},` +
				`{"type":"text","text":"e","marks":[{"type":"code"}]},{"type":"text","text":" "},` +
				`{"type":"text","text":"f","marks":[{"type":"link","attrs":{"href":"https://x.test/g"}}]}]}]`,
		},
		{
			name: "nested marks",
			md:   "**a *b* [c](https://x.test)** [`d`](https://y.test)",
			want: `[{"type":"paragraph","content":[{"type":"text","text":"a ","marks":[{"type":"strong"}]},` +
				`{"type":"text","text":"b","marks":[{"type":"strong"},{"type":"em"}]},{"type":"text","text":" ","marks":[{"type":"strong"}]},` +
				`{"type":"text","text":"c","marks":[{"type":"strong"},{"type":"link","attrs":{"href":"https://x.test"}}]},` +
				`{"type":"text","text":" "},` +
				`{"type":"text","text":"d","marks":[{"type":"link","attrs":{"href":"https://y.test"}},{"type":"code"}]}]}]`,
		},
		{
			name: "literal delimiters and escapes",
			md:   `a * b ** c \*d\* [WIP] 5 ~ 6 snake_case`,
			want: `[{"type":"paragraph","content":[{"type":"text","text":"a * b ** c *d* [WIP] 5 ~ 6 snake_case"}]}]`,
		},
		{
			name: "unclosed delimiters stay literal",
			md:   "name-*) **a ~~b `c ***d",
			want: `[{"type":"paragraph","content":[{"type":"text","text":"name-*) **a ~~b ` + "`" + `c ***d"}]}]`,
		},
		{
			name: "bare url drops sentence punctuation",
			md:   "see https://x.test/a_(b)?q=1.",
			want: `[{"type":"paragraph","content":[{"type":"text","text":"see "},` +
				`{"type":"text","text":"https://x.test/a_(b)?q=1","marks":[{"type":"link","attrs":{"href":"https://x.test/a_(b)?q=1"}}]},` +
				`{"type":"text","text":"."}]}]`,
		},
		{
			name: "autolink and br",
			md:   "<https://x.test><br/>next",
			want: `[{"type":"paragraph","content":[` +
				`{"type":"text","text":"https://x.test","marks":[{"type":"link","attrs":{"href":"https://x.test"}}]},` +
				`{"type":"hardBreak"},{"type":"text","text":"next"}]}]`,
		},
		{
			name: "code span keeps its content verbatim",
			md:   "`` a`b `` and `**x**`",
			want: `[{"type":"paragraph","content":[{"type":"text","text":"a` + "`" + `b","marks":[{"type":"code"}]},` +
				`{"type":"text","text":" and "},{"type":"text","text":"**x**","marks":[{"type":"code"}]}]}]`,
		},
		{
			name: "nested lists",
			md:   "- a\n  continued\n  1. b\n  2. c\n- d",
			want: `[{"type":"bulletList","content":[` +
				`{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"a"},{"type":"hardBreak"},{"type":"text","text":"continued"}]},` +
				`{"type":"orderedList","attrs":{"order":1},"content":[` +
				`{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"b"}]}]},` +
				`{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"c"}]}]}]}]},` +
				`{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"d"}]}]}]}]`,
		},
		{
			name: "blank lines between items keep one list",
			md:   "* a\n\n* b",
			want: `[{"type":"bulletList","content":[` +
				`{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]}]},` +
				`{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"b"}]}]}]}]`,
		},
		{
			name: "marker kind change starts a new list",
			md:   "- a\n1. b",
			want: `[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]}]}]},` +
				`{"type":"orderedList","attrs":{"order":1},"content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"b"}]}]}]}]`,
		},
		{
			name: "ordered list keeps its start",
			md:   "3. a",
			want: `[{"type":"orderedList","attrs":{"order":3},"content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]}]}]}]`,
		},
		{
			name: "ordered list not starting at 1 does not interrupt a paragraph",
			md:   "text\n2024. more",
			want: `[{"type":"paragraph","content":[{"type":"text","text":"text"},{"type":"hardBreak"},{"type":"text","text":"2024. more"}]}]`,
		},
		{
			name: "task list nests as a sibling taskList",
			md:   "- [ ] a\n- [x] b\n  - [ ] c",
			want: `[{"type":"taskList","attrs":{"localId":"tl-1"},"content":[` +
				`{"type":"taskItem","attrs":{"localId":"t-2","state":"TODO"},"content":[{"type":"text","text":"a"}]},` +
				`{"type":"taskItem","attrs":{"localId":"t-3","state":"DONE"},"content":[{"type":"text","text":"b"}]},` +
				`{"type":"taskList","attrs":{"localId":"tl-4"},"content":[` +
				`{"type":"taskItem","attrs":{"localId":"t-5","state":"TODO"},"content":[{"type":"text","text":"c"}]}]}]}]`,
		},
		{
			name: "blockquote parses its content",
			md:   "> a\n>\n> - b",
			want: `[{"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]},` +
				`{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"b"}]}]}]}]}]`,
		},
		{
			name: "table with escaped pipe and empty cell",
			md:   "| h1 | h2 |\n| --- | :-: |\n| `a \\| b` | |",
			want: `[{"type":"table","attrs":{"isNumberColumnEnabled":false,"layout":"default"},"content":[` +
				`{"type":"tableRow","content":[{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"h1"}]}]},` +
				`{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"h2"}]}]}]},` +
				`{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"a | b","marks":[{"type":"code"}]}]}]},` +
				`{"type":"tableCell","content":[{"type":"paragraph"}]}]}]}]`,
		},
		{
			name: "code fence keeps content and language",
			md:   "````md\n```\n<b>&\n```\n````",
			want: `[{"type":"codeBlock","attrs":{"language":"md"},"content":[{"type":"text","text":"` + "```" + `\n<b>&\n` + "```" + `"}]}]`,
		},
		{
			name: "code fence inside a list item",
			md:   "- a\n  ```\n  x\n  ```",
			want: `[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]},` +
				`{"type":"codeBlock","content":[{"type":"text","text":"x"}]}]}]}]`,
		},
		{
			name: "rules",
			md:   "---\n* * *",
			want: `[{"type":"rule"},{"type":"rule"}]`,
		},
		{
			name: "CRLF, BOM and tab indentation",
			md:   "\uFEFF- a\r\n\t- b\r\n",
			want: `[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]},` +
				`{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"b"}]}]}]}]}]}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc, err := Convert(tt.md)
			if err != nil {
				t.Fatalf("Convert: %v", err)
			}

			var got bytes.Buffer

			enc := json.NewEncoder(&got)
			enc.SetEscapeHTML(false)

			if err := enc.Encode(doc.Content); err != nil {
				t.Fatal(err)
			}

			var want bytes.Buffer
			if err := json.Compact(&want, []byte(tt.want)); err != nil {
				t.Fatalf("bad want JSON: %v", err)
			}

			if !bytes.Equal(bytes.TrimSpace(got.Bytes()), want.Bytes()) {
				t.Errorf("got\n%s\nwant\n%s", got.String(), want.String())
			}

			if err := Verify(tt.md, doc); err != nil {
				t.Errorf("Verify: %v", err)
			}
		})
	}
}

func TestConvertRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		md   string
		line int
		msg  string
	}{
		{"triple asterisk", "ok\n***a***", 2, "***...*** is not supported"},
		{"code inside strong", "**a `b`**", 1, "code span inside"},
		{"code inside em", "*a `b`*", 1, "code span inside"},
		{"nested link", "[a [b](https://x.test)](https://y.test)", 1, "link inside a link"},
		{"empty link url", "[a]()", 1, "empty URL"},
		{"link title", `[a](https://x.test "t")`, 1, "link titles"},
		{"image", "![a](https://x.test/a.png)", 1, "images"},
		{"empty heading", "#", 1, "empty heading"},
		{"indented code", "    code", 1, "indented line"},
		{"unterminated fence", "\n```\nx", 2, "unterminated code fence"},
		{"empty list item", "- ", 1, "empty list item"},
		{"heading in list item", "- # a", 1, "cannot start with heading"},
		{"task under bullet", "- a\n  - [ ] b", 1, "list item cannot contain taskList"},
		{"bullet under task", "- [ ] a\n  - b", 1, "task item cannot contain bulletList"},
		{"heading in blockquote", "> # a", 1, "blockquote cannot contain heading"},
		{"table in blockquote", "> | a |\n> | - |", 1, "blockquote cannot contain table"},
		{"table without separator", "| a |\n| b |", 1, "header separator"},
		{"separator width mismatch", "| a | b |\n| - |", 1, "header separator"},
		{"ragged row", "| a | b |\n| - | - |\n| c |", 3, "1 cells, header has 2"},
		{"inline error in a cell", "| a |\n| - |\n| ![b](https://x.test/b.png) |", 3, "images"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := Convert(tt.md)

			var se *SyntaxError
			if !errors.As(err, &se) {
				t.Fatalf("want *SyntaxError, got %v", err)
			}

			if se.Line != tt.line || !strings.Contains(se.Msg, tt.msg) {
				t.Errorf("got line %d %q, want line %d containing %q", se.Line, se.Msg, tt.line, tt.msg)
			}
		})
	}
}

func TestMarshalDoesNotEscapeHTML(t *testing.T) {
	t.Parallel()

	doc, err := Convert("`<a> & <b>`")
	if err != nil {
		t.Fatal(err)
	}

	out, err := Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(out, []byte(`"<a> & <b>"`)) {
		t.Errorf("HTML characters were escaped: %s", out)
	}
}
