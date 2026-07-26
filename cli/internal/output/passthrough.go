package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Passthrough pairs the API's own JSON with a table summary.
//
// It exists because a hand-written response struct silently drops every field it
// does not name, and `--output json` promising "what the API returned" while
// quietly filtering it is worse than not offering json at all: a script reading
// a field the struct forgot gets null, which is indistinguishable from false.
// Decoding into a typed struct is still fine for the table, which is a summary
// by definition — but json and yaml emit the payload itself.
type Passthrough struct {
	// Raw is the response body exactly as the API sent it.
	Raw json.RawMessage
	// Summary is the table rendering.
	Summary Table
}

// Table implements Tabular.
func (p Passthrough) Table() Table { return p.Summary }

// writeJSON re-indents the original bytes rather than round-tripping through a
// map, so key order and numeric precision survive untouched.
func (p Passthrough) writeJSON(out io.Writer) error {
	var buf bytes.Buffer
	if err := json.Indent(&buf, p.Raw, "", "  "); err != nil {
		return fmt.Errorf("formatting json output: %w", err)
	}
	buf.WriteByte('\n')
	if _, err := out.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("writing json output: %w", err)
	}
	return nil
}

// yamlValue decodes the raw JSON into plain Go values for YAML encoding.
//
// json.Number is preserved through the decode and converted here, because the
// default float64 decoding silently mangles large integers — and this API sends
// byte counts and IDs as int64, where a rounded value would be wrong rather than
// merely imprecise.
func (p Passthrough) yamlValue() (any, error) {
	dec := json.NewDecoder(bytes.NewReader(p.Raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("decoding response for yaml output: %w", err)
	}
	return denumber(v), nil
}

func denumber(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, inner := range t {
			t[k] = denumber(inner)
		}
		return t
	case []any:
		for i, inner := range t {
			t[i] = denumber(inner)
		}
		return t
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		// Beyond both — keep the digits rather than losing them.
		return t.String()
	default:
		return v
	}
}
