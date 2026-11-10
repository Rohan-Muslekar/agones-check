// Package report renders check results as text, JSON, GitHub Actions
// annotations, or a Markdown environment matrix.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Rohan-Muslekar/agones-check/internal/check"
)

// Format names accepted by Write.
var Formats = []string{"text", "json", "github", "matrix"}

// Write renders r in the given format.
func Write(w io.Writer, format string, r *check.Result, min check.Severity) error {
	switch format {
	case "text", "":
		return Text(w, r, min)
	case "json":
		return JSON(w, r, min)
	case "github":
		return GitHub(w, r, min)
	case "matrix":
		return Matrix(w, r)
	}
	return fmt.Errorf("unknown output format %q (want one of %s)", format, strings.Join(Formats, ", "))
}

func filter(r *check.Result, min check.Severity) []check.Finding {
	var out []check.Finding
	for _, f := range r.Findings {
		if f.Severity.AtLeast(min) {
			out = append(out, f)
		}
	}
	return out
}

func envs(f check.Finding) string {
	if len(f.Profiles) == 0 {
		return "all"
	}
	return strings.Join(f.Profiles, ",")
}

// Text is the human-readable default.
func Text(w io.Writer, r *check.Result, min check.Severity) error {
	fs := filter(r, min)
	for _, f := range fs {
		loc := f.Source.String()
		if f.Field != "" {
			loc += " " + f.Field
		}
		fmt.Fprintf(w, "%s: %s %s [%s] (env: %s)\n    %s\n", loc, strings.ToUpper(string(f.Severity)), f.Rule, f.Object, envs(f), f.Message)
	}
	if len(fs) > 0 {
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "%d objects from %d files (%d non-Agones documents skipped), %d environments (%s): %d errors, %d warnings in %s\n",
		len(r.Objects), r.Files, r.Skipped, len(r.Profiles), strings.Join(r.Profiles, ", "),
		r.Count(check.Error), r.Count(check.Warning)-r.Count(check.Error), r.Elapsed.Round(1e5))
	return nil
}

// JSON writes the full result for tools.
func JSON(w io.Writer, r *check.Result, min check.Severity) error {
	out := *r
	out.Findings = filter(r, min)
	if out.Findings == nil {
		out.Findings = []check.Finding{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// GitHub writes workflow commands so findings show up as annotations on
// the pull request diff.
func GitHub(w io.Writer, r *check.Result, min check.Severity) error {
	for _, f := range filter(r, min) {
		level := "warning"
		switch f.Severity {
		case check.Error:
			level = "error"
		case check.Info:
			level = "notice"
		}
		title := fmt.Sprintf("%s %s (env: %s)", f.Rule, f.Object, envs(f))
		fmt.Fprintf(w, "::%s file=%s,line=%d,title=%s::%s\n", level, f.Source.File, f.Source.Line, escProp(title), escData(f.Message))
	}
	return Text(w, r, min)
}

func escData(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	return strings.ReplaceAll(s, "\n", "%0A")
}

func escProp(s string) string {
	s = escData(s)
	s = strings.ReplaceAll(s, ":", "%3A")
	return strings.ReplaceAll(s, ",", "%2C")
}

// Matrix prints the object x environment table as Markdown.
func Matrix(w io.Writer, r *check.Result) error {
	fmt.Fprintf(w, "| Object | %s |\n", strings.Join(r.Profiles, " | "))
	fmt.Fprintf(w, "|---|%s\n", strings.Repeat("---|", len(r.Profiles)))
	for _, id := range r.Objects {
		row := r.Matrix[id]
		cells := make([]string, len(row))
		for i, s := range row {
			cells[i] = symbol(s)
		}
		fmt.Fprintf(w, "| %s | %s |\n", id, strings.Join(cells, " | "))
	}
	_, err := fmt.Fprintln(w, "\nok = passes, warn = warning only, broken = accepted but fails at runtime, REJECTED = refused at apply")
	return err
}

func symbol(s check.Status) string {
	switch s {
	case check.StatusRejected:
		return "REJECTED"
	case check.StatusBroken:
		return "broken"
	case check.StatusWarning:
		return "warn"
	}
	return "ok"
}
