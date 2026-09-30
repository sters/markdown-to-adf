// Package adf converts a strict subset of Markdown into Atlassian Document Format.
//
// Anything outside the supported subset is rejected with an error instead of being
// passed through, because the failure this package exists to prevent is text that
// silently disappears or changes meaning on the way into JIRA. See Verify for the
// independent check that every source line survived the conversion.
package adf

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Doc is the root of an ADF document.
type Doc struct {
	Type    string  `json:"type"`
	Version int     `json:"version"`
	Content []*Node `json:"content"`
}

// Node is any ADF block or inline node.
type Node struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []*Node        `json:"content,omitempty"`
	Text    string         `json:"text,omitempty"`
	Marks   []Mark         `json:"marks,omitempty"`
}

// Mark is an inline formatting mark on a text node.
type Mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// Marshal encodes doc as JSON without HTML escaping, so `<`, `>` and `&` in code
// stay readable in the payload.
func Marshal(doc *Doc) ([]byte, error) {
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("encode ADF: %w", err)
	}

	return buf.Bytes(), nil
}

// SyntaxError reports Markdown that this package does not model.
type SyntaxError struct {
	Line int
	Msg  string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
}

func textNode(text string, marks []Mark) *Node {
	n := &Node{Type: "text", Text: text}
	if len(marks) > 0 {
		n.Marks = append([]Mark(nil), marks...)
	}

	return n
}

func flatText(n *Node, out *[]byte) {
	if n.Type == "text" {
		*out = append(*out, n.Text...)
	}

	for _, c := range n.Content {
		flatText(c, out)
	}
}
