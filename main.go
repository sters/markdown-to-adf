package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/sters/markdown-to-adf/adf"
)

//nolint:gochecknoglobals
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func getVersion() string {
	if version != "dev" {
		return version
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
	}

	return version
}

func getCommit() string {
	if commit != "none" {
		return commit
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				if len(setting.Value) > 7 {
					return setting.Value[:7]
				}

				return setting.Value
			}
		}
	}

	return commit
}

func getDate() string {
	if date != "unknown" {
		return date
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.time" {
				if t, err := time.Parse(time.RFC3339, setting.Value); err == nil {
					return t.UTC().Format("2006-01-02T15:04:05Z")
				}

				return setting.Value
			}
		}
	}

	return date
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

const usage = `Usage: markdown-to-adf [-o output.json] [input.md]

Converts Markdown to Atlassian Document Format (ADF) JSON, for acli's
--description-file / --body-file. Reads stdin when input is omitted or "-".

Unsupported Markdown is an error, and the output is checked to contain the text
of every source line; nothing is written unless both pass.

Flags:
`

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("markdown-to-adf", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}

	var (
		output      string
		showVersion bool
	)

	fs.StringVar(&output, "o", "", "write the ADF JSON to this file instead of stdout")
	fs.BoolVar(&showVersion, "version", false, "show version information")
	fs.BoolVar(&showVersion, "v", false, "show version information (short)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}

		return 2
	}

	if showVersion {
		fmt.Fprintf(stdout, "Version:    %s\n", getVersion())
		fmt.Fprintf(stdout, "Commit:     %s\n", getCommit())
		fmt.Fprintf(stdout, "Built:      %s\n", getDate())
		fmt.Fprintf(stdout, "Go version: %s\n", runtime.Version())
		fmt.Fprintf(stdout, "OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)

		return 0
	}

	if fs.NArg() > 1 {
		fs.Usage()

		return 2
	}

	if err := convert(fs.Arg(0), output, stdin, stdout); err != nil {
		fmt.Fprintf(stderr, "markdown-to-adf: %v\n", err)

		return 1
	}

	return 0
}

func convert(input, output string, stdin io.Reader, stdout io.Writer) error {
	var (
		src []byte
		err error
	)

	if input == "" || input == "-" {
		src, err = io.ReadAll(stdin)
	} else {
		src, err = os.ReadFile(input)
	}

	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	doc, err := adf.Convert(string(src))
	if err != nil {
		return err //nolint:wrapcheck // SyntaxError already names the line.
	}

	if err := adf.Verify(string(src), doc); err != nil {
		return err //nolint:wrapcheck // LossError already lists the lines.
	}

	out, err := adf.Marshal(doc)
	if err != nil {
		return err //nolint:wrapcheck
	}

	if output == "" {
		if _, err := stdout.Write(out); err != nil {
			return fmt.Errorf("write output: %w", err)
		}

		return nil
	}

	if err := os.WriteFile(output, out, 0o600); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	return nil
}
