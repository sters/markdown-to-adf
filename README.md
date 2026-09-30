# markdown-to-adf

[![go](https://github.com/sters/markdown-to-adf/workflows/Go/badge.svg)](https://github.com/sters/markdown-to-adf/actions?query=workflow%3AGo)
[![coverage](docs/coverage.svg)](https://github.com/sters/markdown-to-adf)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Convert Markdown to [Atlassian Document Format](https://developer.atlassian.com/cloud/jira/platform/apis/document/structure/) (ADF) JSON, for JIRA descriptions and comments posted with `acli`.

`acli jira workitem edit --description-file` and `comment create --body-file` accept plain text or ADF only; Markdown is stored literally. This tool fills that gap, and is built around one rule: **text must never silently disappear**.

- Markdown it does not model is an error with a line number, not a best-effort pass-through.
- After converting, it checks independently of the parser that the text of every source line appears in the output, in order. Nothing is written unless both steps pass.

## Usage

```shell
go run github.com/sters/markdown-to-adf@latest -o body.adf.json body.md
acli jira workitem edit --key PROJ-123 --description-file body.adf.json --yes
```

Input is read from stdin when the file is omitted or `-`; output goes to stdout without `-o`.

## Supported Markdown

| Construct | ADF |
| --- | --- |
| `#` to `######` headings | `heading` |
| Paragraphs; consecutive lines are joined with line breaks | `paragraph` + `hardBreak` |
| Fenced code blocks (backticks or tildes, optional language) | `codeBlock` |
| `---`, `***`, `___` | `rule` |
| `-` / `*` / `+` and `1.` / `1)` lists, nested by indentation | `bulletList`, `orderedList` |
| `- [ ]` / `- [x]` task lists, nested | `taskList`, `taskItem` |
| `>` blockquotes containing paragraphs, lists or code | `blockquote` |
| Pipe tables with a header separator row; `\|` escapes a pipe | `table` |
| `**strong**`, `*em*`, `~~strike~~`, `` `code` `` | marks |
| `[label](url)`, `<url>`, bare `http(s)://` URLs | `link` mark |
| `<br>` | `hardBreak` |

Rejected with an error: images, link titles, `***both***`, a code span inside `**`/`*`/`~~` (ADF only lets code combine with a link), headings or tables inside lists and blockquotes, task items under bullet items and vice versa, indented code blocks, tables whose rows do not match the header.

Not modelled and kept as literal text: `_underscore_` emphasis, HTML other than `<br>`, reference-style links, setext headings (`---` is always a rule). Table column alignment is dropped.
