package output

import (
	"bytes"
	"strings"
	"testing"
)

// A table with rows but no headers is legitimate; it must print the rows rather
// than a stray blank line.
func TestWriteTableWithoutHeaders(t *testing.T) {
	var buf bytes.Buffer
	err := writeTable(&buf, Table{Rows: [][]string{{"a", "b"}}})
	if err != nil {
		t.Fatalf("writeTable() error = %v", err)
	}

	got := strings.TrimSpace(buf.String())
	if !strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Errorf("output = %q, want both cells", got)
	}
	if strings.HasPrefix(buf.String(), "\n") {
		t.Errorf("output = %q, want no leading blank line", buf.String())
	}
}

func TestWriteTableAlignsColumns(t *testing.T) {
	var buf bytes.Buffer
	err := writeTable(&buf, Table{
		Headers: []string{"name", "value"},
		Rows: [][]string{
			{"short", "1"},
			{"a-much-longer-name", "2"},
		},
	})
	if err != nil {
		t.Fatalf("writeTable() error = %v", err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3 (header plus two rows): %q", len(lines), buf.String())
	}
	// tabwriter pads to a common width, so the value column starts at the same
	// offset on both data rows.
	if strings.Index(lines[1], "1") != strings.Index(lines[2], "2") {
		t.Errorf("columns are not aligned:\n%s", buf.String())
	}
}

func TestFormatsListsEverySupportedFormat(t *testing.T) {
	got := Formats()
	if len(got) != 3 {
		t.Fatalf("Formats() = %v, want three entries", got)
	}
	// Every advertised format must actually parse, or help text promises
	// something that errors.
	for _, f := range got {
		if _, err := ParseFormat(f); err != nil {
			t.Errorf("ParseFormat(%q) error = %v, but Formats() advertises it", f, err)
		}
	}
}
