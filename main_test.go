package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunStdinToStdout(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	code := run(nil, strings.NewReader("hello **world**"), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}

	want := `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[` +
		`{"type":"text","text":"hello "},{"type":"text","text":"world","marks":[{"type":"strong"}]}]}]}` + "\n"
	if stdout.String() != want {
		t.Errorf("stdout = %s", stdout.String())
	}
}

func TestRunFileToFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	in := filepath.Join(dir, "in.md")
	out := filepath.Join(dir, "out.json")

	if err := os.WriteFile(in, []byte("# hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-o", out, in}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(got, []byte(`"type":"heading"`)) || stdout.Len() != 0 {
		t.Errorf("out = %s, stdout = %s", got, stdout.String())
	}
}

func TestRunRejectsWithoutWriting(t *testing.T) {
	t.Parallel()

	out := filepath.Join(t.TempDir(), "out.json")

	var stdout, stderr bytes.Buffer

	code := run([]string{"-o", out}, strings.NewReader("ok\n![img](https://x.test/a.png)"), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}

	if !strings.Contains(stderr.String(), "line 2: images are not supported") {
		t.Errorf("stderr = %s", stderr.String())
	}

	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("output was written: %v", err)
	}
}
