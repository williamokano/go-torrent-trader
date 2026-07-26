package output

import (
	"bytes"
	"strings"
	"testing"
)

type sample struct {
	Name  string `json:"name" yaml:"name"`
	Count int    `json:"count" yaml:"count"`
}

func (s sample) Table() Table {
	return Table{Headers: []string{"name", "count"}, Rows: [][]string{{s.Name, "1"}}}
}

// notTabular deliberately has no Table method.
type notTabular struct {
	Name string `json:"name" yaml:"name"`
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		in      string
		want    Format
		wantErr bool
	}{
		{in: "table", want: FormatTable},
		{in: "json", want: FormatJSON},
		{in: "yaml", want: FormatYAML},
		{in: "JSON", want: FormatJSON},
		{in: "  yaml  ", want: FormatYAML},
		{in: "xml", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseFormat(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseFormat(%q) succeeded, want an error", tc.in)
				}
				// The message must list the valid choices, or the user has to read
				// the source to find them.
				for _, f := range Formats() {
					if !strings.Contains(err.Error(), f) {
						t.Errorf("error %q does not list %q", err, f)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFormat(%q) error = %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ParseFormat(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPrintTable(t *testing.T) {
	var buf bytes.Buffer
	if err := New(&buf, FormatTable).Print(sample{Name: "alice", Count: 1}); err != nil {
		t.Fatalf("Print() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "NAME") || !strings.Contains(got, "COUNT") {
		t.Errorf("output %q, want upper-case headers", got)
	}
	if !strings.Contains(got, "alice") {
		t.Errorf("output %q, want the row value", got)
	}
}

// An empty result set must say so rather than printing a bare header row that
// looks like output got cut off.
func TestPrintTableWithNoRows(t *testing.T) {
	var buf bytes.Buffer
	empty := Table{Headers: []string{"name"}, Rows: nil}
	if err := writeTable(&buf, empty); err != nil {
		t.Fatalf("writeTable() error = %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "No results." {
		t.Errorf("output = %q, want \"No results.\"", got)
	}
}

func TestPrintJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := New(&buf, FormatJSON).Print(sample{Name: "alice", Count: 2}); err != nil {
		t.Fatalf("Print() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, `"name": "alice"`) {
		t.Errorf("output %q, want indented JSON with the name field", got)
	}
	// Count is 2 in the value but the table renderer hardcodes 1; JSON must show
	// the real value, which is the point of having both.
	if !strings.Contains(got, `"count": 2`) {
		t.Errorf("output %q, want the real count", got)
	}
}

func TestPrintYAML(t *testing.T) {
	var buf bytes.Buffer
	if err := New(&buf, FormatYAML).Print(sample{Name: "alice", Count: 3}); err != nil {
		t.Fatalf("Print() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "name: alice") {
		t.Errorf("output %q, want YAML with the name field", got)
	}
	if !strings.Contains(got, "count: 3") {
		t.Errorf("output %q, want the count field", got)
	}
}

// Asking for a table of something that has no table form must be an error rather
// than silently printing something misleading.
func TestPrintTableRejectsANonTabularValue(t *testing.T) {
	var buf bytes.Buffer
	err := New(&buf, FormatTable).Print(notTabular{Name: "alice"})
	if err == nil {
		t.Fatal("Print() succeeded for a non-tabular value, want an error")
	}
	if !strings.Contains(err.Error(), "--output json") {
		t.Errorf("error %q, want it to suggest another format", err)
	}
}

// json and yaml must work for a value with no table form, so a command is never
// blocked from being scriptable by a missing Table method.
func TestPrintJSONWorksForANonTabularValue(t *testing.T) {
	var buf bytes.Buffer
	if err := New(&buf, FormatJSON).Print(notTabular{Name: "alice"}); err != nil {
		t.Fatalf("Print() error = %v", err)
	}
	if !strings.Contains(buf.String(), "alice") {
		t.Errorf("output %q, want the value", buf.String())
	}
}

func TestPrintRejectsAnUnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	if err := New(&buf, Format("xml")).Print(sample{}); err == nil {
		t.Fatal("Print() succeeded with an unknown format, want an error")
	}
}
