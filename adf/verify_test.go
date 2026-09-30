package adf

import (
	"errors"
	"testing"
)

func TestVerifyDetectsLoss(t *testing.T) {
	t.Parallel()

	md := "# Title\n\n- first\n- second\n\n| a | b |\n| - | - |\n| c | d |\n"

	tests := []struct {
		name    string
		mutate  func(doc *Doc)
		missing []MissingLine
	}{
		{
			name:    "dropped list item",
			mutate:  func(doc *Doc) { doc.Content[1].Content = doc.Content[1].Content[:1] },
			missing: []MissingLine{{Line: 4, Text: "second"}},
		},
		{
			name: "dropped table cell",
			mutate: func(doc *Doc) {
				row := doc.Content[2].Content[1]
				row.Content = row.Content[:1]
			},
			missing: []MissingLine{{Line: 8, Text: "d"}},
		},
		{
			name: "reordered blocks",
			mutate: func(doc *Doc) {
				doc.Content[0], doc.Content[1] = doc.Content[1], doc.Content[0]
			},
			missing: []MissingLine{{Line: 3, Text: "first"}, {Line: 4, Text: "second"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc, err := Convert(md)
			if err != nil {
				t.Fatal(err)
			}

			tt.mutate(doc)

			var le *LossError
			if !errors.As(Verify(md, doc), &le) {
				t.Fatal("want *LossError")
			}

			if len(le.Missing) != len(tt.missing) {
				t.Fatalf("missing = %+v, want %+v", le.Missing, tt.missing)
			}

			for i := range tt.missing {
				if le.Missing[i] != tt.missing[i] {
					t.Errorf("missing[%d] = %+v, want %+v", i, le.Missing[i], tt.missing[i])
				}
			}
		})
	}
}

func TestVerifyIgnoresLinkTargets(t *testing.T) {
	t.Parallel()

	md := "[label](https://x.test/very/long) and <br> **bold**"

	doc, err := Convert(md)
	if err != nil {
		t.Fatal(err)
	}

	if err := Verify(md, doc); err != nil {
		t.Errorf("Verify: %v", err)
	}
}
