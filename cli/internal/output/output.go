// Package output renders command results in the format the caller asked for.
//
// Every command prints through a Printer rather than calling fmt directly, so
// that --output json works everywhere without each command re-implementing it.
// That matters more for a CLI meant for cron jobs and pipelines than the
// duplication it saves: a command that only knows how to print a table is a
// command nobody can script against.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

// Format is a rendering style.
type Format string

// Supported formats.
const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
)

// Formats lists the supported formats, for help text and shell completion.
func Formats() []string {
	return []string{string(FormatTable), string(FormatJSON), string(FormatYAML)}
}

// ParseFormat validates a --output value.
func ParseFormat(s string) (Format, error) {
	switch f := Format(strings.ToLower(strings.TrimSpace(s))); f {
	case FormatTable, FormatJSON, FormatYAML:
		return f, nil
	default:
		return "", fmt.Errorf("unknown output format %q: want one of %s", s, strings.Join(Formats(), ", "))
	}
}

// Table is a rendered column layout.
type Table struct {
	Headers []string
	Rows    [][]string
}

// Tabular is implemented by results that have a sensible table representation.
//
// A result that cannot be a table simply does not implement this, and asking for
// --output table reports that rather than printing something misleading.
type Tabular interface {
	Table() Table
}

// Printer writes results in one format.
type Printer struct {
	Out    io.Writer
	Format Format
}

// New builds a Printer.
func New(out io.Writer, format Format) Printer {
	return Printer{Out: out, Format: format}
}

// Print renders v.
//
// For json and yaml the whole value is marshalled, so scripts see every field the
// API returned. For table only the columns the type chose are shown, which is a
// summary and is documented as one.
func (p Printer) Print(v any) error {
	// A Passthrough carries the API's own bytes; emit those rather than
	// re-marshalling a struct that may not name every field.
	if pt, ok := v.(Passthrough); ok && len(pt.Raw) > 0 {
		switch p.Format {
		case FormatJSON:
			return pt.writeJSON(p.Out)
		case FormatYAML:
			value, err := pt.yamlValue()
			if err != nil {
				return err
			}
			v = value
		}
	}

	switch p.Format {
	case FormatJSON:
		enc := json.NewEncoder(p.Out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(v); err != nil {
			return fmt.Errorf("encoding json output: %w", err)
		}
		return nil

	case FormatYAML:
		enc := yaml.NewEncoder(p.Out)
		enc.SetIndent(2)
		if err := enc.Encode(v); err != nil {
			return fmt.Errorf("encoding yaml output: %w", err)
		}
		if err := enc.Close(); err != nil {
			return fmt.Errorf("closing yaml output: %w", err)
		}
		return nil

	case FormatTable:
		t, ok := v.(Tabular)
		if !ok {
			return fmt.Errorf("%T has no table representation: use --output json or --output yaml", v)
		}
		return writeTable(p.Out, t.Table())

	default:
		return fmt.Errorf("unknown output format %q", p.Format)
	}
}

func writeTable(out io.Writer, t Table) error {
	if len(t.Rows) == 0 {
		if _, err := fmt.Fprintln(out, "No results."); err != nil {
			return fmt.Errorf("writing table output: %w", err)
		}
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 4, 3, ' ', 0)
	if len(t.Headers) > 0 {
		if _, err := fmt.Fprintln(w, strings.Join(upper(t.Headers), "\t")); err != nil {
			return fmt.Errorf("writing table header: %w", err)
		}
	}
	for _, row := range t.Rows {
		if _, err := fmt.Fprintln(w, strings.Join(row, "\t")); err != nil {
			return fmt.Errorf("writing table row: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flushing table output: %w", err)
	}
	return nil
}

func upper(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToUpper(s)
	}
	return out
}
