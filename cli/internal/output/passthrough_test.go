package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// The whole point of Passthrough: json shows what the API sent, including fields
// no Go struct in this repo names. A script reading such a field would otherwise
// get null, which is indistinguishable from false.
func TestPassthroughJSONEmitsEveryFieldIncludingUnknownOnes(t *testing.T) {
	raw := json.RawMessage(`{"user":{"username":"alice","can_upload":false,"bonus_points":42,"a_field_added_next_year":"x"}}`)

	var buf bytes.Buffer
	err := New(&buf, FormatJSON).Print(Passthrough{
		Raw:     raw,
		Summary: Table{Headers: []string{"username"}, Rows: [][]string{{"alice"}}},
	})
	if err != nil {
		t.Fatalf("Print() error = %v", err)
	}

	got := buf.String()
	for _, want := range []string{"can_upload", "bonus_points", "a_field_added_next_year"} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q is missing %q", got, want)
		}
	}
	// It must still be valid, indented JSON.
	var round any
	if err := json.Unmarshal([]byte(got), &round); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}
	if !strings.Contains(got, "\n  ") {
		t.Errorf("output %q is not indented", got)
	}
}

// A false boolean must survive as false, not vanish. Distinguishing "false" from
// "absent" is the reason the passthrough exists at all.
func TestPassthroughPreservesFalseAndNull(t *testing.T) {
	raw := json.RawMessage(`{"warned":false,"last_login":null}`)

	var buf bytes.Buffer
	if err := New(&buf, FormatJSON).Print(Passthrough{Raw: raw}); err != nil {
		t.Fatalf("Print() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	v, ok := got["warned"]
	if !ok {
		t.Fatal("warned is absent, want false")
	}
	if v != false {
		t.Errorf("warned = %v, want false", v)
	}
	if v, ok := got["last_login"]; !ok || v != nil {
		t.Errorf("last_login = %v (present %v), want an explicit null", v, ok)
	}
}

// Byte counts and IDs are int64. Decoding through float64 for YAML would round
// them, turning a precise number into a subtly wrong one.
func TestPassthroughYAMLPreservesLargeIntegers(t *testing.T) {
	raw := json.RawMessage(`{"uploaded":9007199254740993,"id":1234567890123456789}`)

	var buf bytes.Buffer
	if err := New(&buf, FormatYAML).Print(Passthrough{Raw: raw}); err != nil {
		t.Fatalf("Print() error = %v", err)
	}

	got := buf.String()
	for _, want := range []string{"9007199254740993", "1234567890123456789"} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q lost precision, want %q verbatim", got, want)
		}
	}
	// A float64 round-trip renders these in scientific notation or off by one.
	if strings.Contains(got, "e+") {
		t.Errorf("output %q used scientific notation, want exact integers", got)
	}
}

func TestPassthroughYAMLKeepsFloats(t *testing.T) {
	var buf bytes.Buffer
	if err := New(&buf, FormatYAML).Print(Passthrough{Raw: json.RawMessage(`{"ratio":2.5}`)}); err != nil {
		t.Fatalf("Print() error = %v", err)
	}
	if !strings.Contains(buf.String(), "2.5") {
		t.Errorf("output %q, want the float preserved", buf.String())
	}
}

func TestPassthroughYAMLHandlesNestedStructures(t *testing.T) {
	raw := json.RawMessage(`{"user":{"tags":["a","b"],"counts":{"seeding":3}}}`)

	var buf bytes.Buffer
	if err := New(&buf, FormatYAML).Print(Passthrough{Raw: raw}); err != nil {
		t.Fatalf("Print() error = %v", err)
	}
	for _, want := range []string{"tags", "seeding", "3"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("output %q is missing %q", buf.String(), want)
		}
	}
}

// Table output still comes from the typed summary, which is a summary by design.
func TestPassthroughTableUsesTheSummary(t *testing.T) {
	var buf bytes.Buffer
	err := New(&buf, FormatTable).Print(Passthrough{
		Raw:     json.RawMessage(`{"user":{"username":"alice","passkey":"secret"}}`),
		Summary: Table{Headers: []string{"username"}, Rows: [][]string{{"alice"}}},
	})
	if err != nil {
		t.Fatalf("Print() error = %v", err)
	}
	if !strings.Contains(buf.String(), "alice") {
		t.Errorf("output %q, want the summary row", buf.String())
	}
	// The summary is what limits the table; raw fields must not leak into it.
	if strings.Contains(buf.String(), "secret") {
		t.Error("table output included a field the summary did not name")
	}
}

func TestPassthroughReportsMalformedJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := New(&buf, FormatJSON).Print(Passthrough{Raw: json.RawMessage(`{"broken"`)}); err == nil {
		t.Fatal("Print() succeeded on malformed JSON, want an error")
	}
	if err := New(&buf, FormatYAML).Print(Passthrough{Raw: json.RawMessage(`{"broken"`)}); err == nil {
		t.Fatal("Print() succeeded on malformed JSON for yaml, want an error")
	}
}

// An empty Raw means the value was never populated; fall back to marshalling the
// struct rather than emitting nothing.
func TestPassthroughWithNoRawFallsBackToMarshalling(t *testing.T) {
	var buf bytes.Buffer
	if err := New(&buf, FormatJSON).Print(Passthrough{Summary: Table{Headers: []string{"a"}}}); err != nil {
		t.Fatalf("Print() error = %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Print() wrote nothing for an empty passthrough")
	}
}
