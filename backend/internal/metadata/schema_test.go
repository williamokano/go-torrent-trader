package metadata

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func ptrInt(i int) *int           { return &i }
func ptrFloat(f float64) *float64 { return &f }
func selectField(k string, o ...string) FieldDef {
	return FieldDef{Key: k, Label: k, Type: TypeSelect, Options: o}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantLen int
		wantErr bool
	}{
		{"empty string", "", 0, false},
		{"whitespace", "   ", 0, false},
		{"null literal", "null", 0, false},
		{"empty array", "[]", 0, false},
		{"one field", `[{"key":"year","label":"Year","type":"number"}]`, 1, false},
		{"malformed", `[{`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.raw))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse() err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && len(got) != tt.wantLen {
				t.Fatalf("Parse() len = %d, want %d", len(got), tt.wantLen)
			}
			if tt.wantErr && !errors.Is(err, ErrInvalidSchema) {
				t.Fatalf("Parse() err not ErrInvalidSchema: %v", err)
			}
		})
	}
}

func TestValidateSchema_Valid(t *testing.T) {
	fields := []FieldDef{
		{Key: "title", Label: "Title", Type: TypeText, MaxLength: ptrInt(200), Pattern: "^[A-Za-z ]+$"},
		{Key: "year", Label: "Year", Type: TypeNumber, Min: ptrFloat(1900), Max: ptrFloat(2100), Integer: true},
		{Key: "codec", Label: "Codec", Type: TypeSelect, Options: []string{"x264", "x265"}},
		{Key: "audio", Label: "Audio", Type: TypeMultiselect, Options: []string{"FLAC", "AC3"}, MaxItems: ptrInt(2)},
		{Key: "hdr", Label: "HDR", Type: TypeBoolean, Required: true},
	}
	if err := ValidateSchema(fields); err != nil {
		t.Fatalf("ValidateSchema() unexpected err = %v", err)
	}
}

func TestValidateSchema_Invalid(t *testing.T) {
	tests := []struct {
		name   string
		fields []FieldDef
	}{
		{"bad key uppercase", []FieldDef{{Key: "Year", Label: "Year", Type: TypeNumber}}},
		{"bad key leading digit", []FieldDef{{Key: "1year", Label: "Year", Type: TypeNumber}}},
		{"bad key symbol", []FieldDef{{Key: "ye-ar", Label: "Year", Type: TypeNumber}}},
		{"empty key", []FieldDef{{Key: "", Label: "Year", Type: TypeNumber}}},
		{"duplicate key", []FieldDef{
			{Key: "year", Label: "Year", Type: TypeNumber},
			{Key: "year", Label: "Year2", Type: TypeNumber},
		}},
		{"empty label", []FieldDef{{Key: "year", Label: "  ", Type: TypeNumber}}},
		{"unknown type", []FieldDef{{Key: "year", Label: "Year", Type: Type("date")}}},
		{"select no options", []FieldDef{{Key: "codec", Label: "Codec", Type: TypeSelect}}},
		{"select empty option", []FieldDef{{Key: "codec", Label: "Codec", Type: TypeSelect, Options: []string{"x264", " "}}}},
		{"select dup option", []FieldDef{{Key: "codec", Label: "Codec", Type: TypeSelect, Options: []string{"x264", "x264"}}}},
		{"multiselect maxitems zero", []FieldDef{{Key: "a", Label: "A", Type: TypeMultiselect, Options: []string{"x"}, MaxItems: ptrInt(0)}}},
		{"number min>max", []FieldDef{{Key: "year", Label: "Year", Type: TypeNumber, Min: ptrFloat(10), Max: ptrFloat(5)}}},
		{"text maxlength zero", []FieldDef{{Key: "t", Label: "T", Type: TypeText, MaxLength: ptrInt(0)}}},
		{"text bad pattern", []FieldDef{{Key: "t", Label: "T", Type: TypeText, Pattern: "("}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSchema(tt.fields)
			if err == nil {
				t.Fatalf("ValidateSchema() expected error, got nil")
			}
			if !errors.Is(err, ErrInvalidSchema) {
				t.Fatalf("ValidateSchema() err not ErrInvalidSchema: %v", err)
			}
		})
	}
}

func TestValidateSchema_LabelTooLong(t *testing.T) {
	long := make([]byte, maxLabelLength+1)
	for i := range long {
		long[i] = 'a'
	}
	err := ValidateSchema([]FieldDef{{Key: "k", Label: string(long), Type: TypeText}})
	if !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("expected ErrInvalidSchema, got %v", err)
	}
}

func TestMerge(t *testing.T) {
	parent := []FieldDef{
		{Key: "codec", Label: "Codec", Type: TypeSelect, Options: []string{"x264", "x265"}},
		{Key: "quality", Label: "Quality", Type: TypeSelect, Options: []string{"1080p", "720p"}},
	}
	movies := []FieldDef{
		{Key: "year", Label: "Year", Type: TypeNumber},
	}
	got := Merge([][]FieldDef{parent, movies})
	wantKeys := []string{"codec", "quality", "year"}
	if len(got) != len(wantKeys) {
		t.Fatalf("Merge() len = %d, want %d", len(got), len(wantKeys))
	}
	for i, k := range wantKeys {
		if got[i].Key != k {
			t.Errorf("Merge()[%d].Key = %q, want %q", i, got[i].Key, k)
		}
	}
}

func TestMerge_ChildOverridesInPlace(t *testing.T) {
	parent := []FieldDef{
		{Key: "codec", Label: "Codec", Type: TypeSelect, Options: []string{"x264"}},
		{Key: "quality", Label: "Quality", Type: TypeSelect, Options: []string{"1080p"}},
	}
	child := []FieldDef{
		{Key: "codec", Label: "Video Codec", Type: TypeSelect, Options: []string{"x264", "x265", "AV1"}},
	}
	got := Merge([][]FieldDef{parent, child})
	if len(got) != 2 {
		t.Fatalf("Merge() len = %d, want 2", len(got))
	}
	// codec stays in position 0 but adopts the child's definition.
	if got[0].Key != "codec" || got[0].Label != "Video Codec" || len(got[0].Options) != 3 {
		t.Errorf("override in place failed: %+v", got[0])
	}
	if got[1].Key != "quality" {
		t.Errorf("Merge()[1].Key = %q, want quality", got[1].Key)
	}
}

func TestMerge_Empty(t *testing.T) {
	if got := Merge(nil); len(got) != 0 {
		t.Fatalf("Merge(nil) len = %d, want 0", len(got))
	}
}

func TestValidateValues_UnknownKey(t *testing.T) {
	schema := []FieldDef{{Key: "year", Label: "Year", Type: TypeNumber}}
	_, err := ValidateValues(schema, map[string]any{"bogus": 1})
	if !errors.Is(err, ErrInvalidValues) {
		t.Fatalf("expected ErrInvalidValues, got %v", err)
	}
}

func TestValidateValues_Required(t *testing.T) {
	schema := []FieldDef{{Key: "year", Label: "Year", Type: TypeNumber, Required: true}}
	for _, values := range []map[string]any{
		{},            // absent
		{"year": nil}, // nil
		{"year": ""},  // empty string
	} {
		if _, err := ValidateValues(schema, values); !errors.Is(err, ErrInvalidValues) {
			t.Fatalf("values %v: expected ErrInvalidValues, got %v", values, err)
		}
	}
}

func TestValidateValues_OptionalEmptyOmitted(t *testing.T) {
	schema := []FieldDef{
		{Key: "year", Label: "Year", Type: TypeNumber},
		{Key: "note", Label: "Note", Type: TypeText},
	}
	out, err := ValidateValues(schema, map[string]any{"year": nil, "note": "  "})
	if err != nil {
		t.Fatalf("unexpected err = %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty canonical map, got %v", out)
	}
}

func TestValidateValues_Number(t *testing.T) {
	schema := []FieldDef{{Key: "year", Label: "Year", Type: TypeNumber, Min: ptrFloat(1900), Max: ptrFloat(2100), Integer: true}}
	tests := []struct {
		name    string
		val     any
		wantErr bool
	}{
		{"valid float", float64(2024), false},
		{"valid numeric string", "2024", false},
		{"below min", float64(1800), true},
		{"above max", float64(2200), true},
		{"non integer", float64(2024.5), true},
		{"not a number", "abc", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateValues(schema, map[string]any{"year": tt.val})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateValues_NumberFromJSON(t *testing.T) {
	// Values coming through json.Unmarshal are float64; make sure whole numbers
	// round-trip back to JSON without a decimal point.
	schema := []FieldDef{{Key: "year", Label: "Year", Type: TypeNumber, Integer: true}}
	var values map[string]any
	if err := json.Unmarshal([]byte(`{"year":2024}`), &values); err != nil {
		t.Fatal(err)
	}
	out, err := ValidateValues(schema, values)
	if err != nil {
		t.Fatalf("unexpected err = %v", err)
	}
	encoded, _ := json.Marshal(out)
	if string(encoded) != `{"year":2024}` {
		t.Fatalf("canonical JSON = %s, want {\"year\":2024}", encoded)
	}
}

func TestValidateValues_Text(t *testing.T) {
	schema := []FieldDef{{Key: "t", Label: "T", Type: TypeText, MaxLength: ptrInt(5), Pattern: "^[a-z]+$"}}
	tests := []struct {
		name    string
		val     any
		wantErr bool
	}{
		{"valid", "abc", false},
		{"too long", "abcdef", true},
		{"pattern mismatch", "ABC", true},
		{"not a string", 123, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateValues(schema, map[string]any{"t": tt.val})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateValues_TextTrimmed(t *testing.T) {
	schema := []FieldDef{{Key: "t", Label: "T", Type: TypeText}}
	out, err := ValidateValues(schema, map[string]any{"t": "  hi  "})
	if err != nil {
		t.Fatalf("unexpected err = %v", err)
	}
	if out["t"] != "hi" {
		t.Fatalf("text not trimmed: %q", out["t"])
	}
}

func TestValidateValues_Select(t *testing.T) {
	schema := []FieldDef{selectField("codec", "x264", "x265")}
	if _, err := ValidateValues(schema, map[string]any{"codec": "x265"}); err != nil {
		t.Fatalf("valid select err = %v", err)
	}
	if _, err := ValidateValues(schema, map[string]any{"codec": "vp9"}); !errors.Is(err, ErrInvalidValues) {
		t.Fatalf("invalid option: expected ErrInvalidValues, got %v", err)
	}
	if _, err := ValidateValues(schema, map[string]any{"codec": 5}); !errors.Is(err, ErrInvalidValues) {
		t.Fatalf("wrong type: expected ErrInvalidValues, got %v", err)
	}
}

func TestValidateValues_Multiselect(t *testing.T) {
	schema := []FieldDef{{Key: "audio", Label: "Audio", Type: TypeMultiselect, Options: []string{"FLAC", "AC3", "DTS"}, MaxItems: ptrInt(2)}}

	out, err := ValidateValues(schema, map[string]any{"audio": []any{"FLAC", "AC3"}})
	if err != nil {
		t.Fatalf("valid multiselect err = %v", err)
	}
	if !reflect.DeepEqual(out["audio"], []string{"FLAC", "AC3"}) {
		t.Fatalf("audio = %v, want [FLAC AC3]", out["audio"])
	}

	// dedupe collapses to one, which is under the limit
	out, err = ValidateValues(schema, map[string]any{"audio": []any{"FLAC", "FLAC"}})
	if err != nil {
		t.Fatalf("dedupe err = %v", err)
	}
	if !reflect.DeepEqual(out["audio"], []string{"FLAC"}) {
		t.Fatalf("dedupe result = %v, want [FLAC]", out["audio"])
	}

	if _, err := ValidateValues(schema, map[string]any{"audio": []any{"FLAC", "AC3", "DTS"}}); !errors.Is(err, ErrInvalidValues) {
		t.Fatalf("over max_items: expected ErrInvalidValues, got %v", err)
	}
	if _, err := ValidateValues(schema, map[string]any{"audio": []any{"MP3"}}); !errors.Is(err, ErrInvalidValues) {
		t.Fatalf("invalid option: expected ErrInvalidValues, got %v", err)
	}
	if _, err := ValidateValues(schema, map[string]any{"audio": []any{1, 2}}); !errors.Is(err, ErrInvalidValues) {
		t.Fatalf("non-string elems: expected ErrInvalidValues, got %v", err)
	}
	if _, err := ValidateValues(schema, map[string]any{"audio": "FLAC"}); !errors.Is(err, ErrInvalidValues) {
		t.Fatalf("non-array: expected ErrInvalidValues, got %v", err)
	}
}

func TestValidateValues_Boolean(t *testing.T) {
	schema := []FieldDef{{Key: "hdr", Label: "HDR", Type: TypeBoolean}}
	tests := []struct {
		name    string
		val     any
		want    any
		wantErr bool
	}{
		{"true bool", true, true, false},
		{"false bool", false, false, false},
		{"true string", "true", true, false},
		{"false string", "FALSE", false, false},
		{"invalid string", "yes", nil, true},
		{"number", 1, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := ValidateValues(schema, map[string]any{"hdr": tt.val})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && out["hdr"] != tt.want {
				t.Fatalf("hdr = %v, want %v", out["hdr"], tt.want)
			}
		})
	}
}

func TestValidateValues_EmptySchemaEmptyValues(t *testing.T) {
	out, err := ValidateValues(nil, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected err = %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty map, got %v", out)
	}
}

func TestValidateValues_EmptySchemaRejectsValues(t *testing.T) {
	if _, err := ValidateValues(nil, map[string]any{"year": 2024}); !errors.Is(err, ErrInvalidValues) {
		t.Fatalf("expected ErrInvalidValues for values against empty schema, got %v", err)
	}
}
