// Package metadata implements the category-driven torrent metadata schema:
// per-category field definitions, inheritance across the category hierarchy,
// and validation of both the schema itself and the values submitted against it.
//
// It is a leaf package with no dependencies on other internal packages, so it
// stays trivially unit-testable and import-cycle-free.
package metadata

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Type enumerates the supported metadata field types.
type Type string

const (
	TypeText        Type = "text"
	TypeNumber      Type = "number"
	TypeSelect      Type = "select"
	TypeMultiselect Type = "multiselect"
	TypeBoolean     Type = "boolean"
)

// Sentinel errors so callers can map failures to HTTP status codes.
var (
	// ErrInvalidSchema is returned when a category's field definitions are malformed.
	ErrInvalidSchema = errors.New("invalid metadata schema")
	// ErrInvalidValues is returned when submitted values don't satisfy the schema.
	ErrInvalidValues = errors.New("invalid metadata values")
)

// maxKeyLength / maxLabelLength bound the schema so a category definition can't grow unbounded.
const (
	maxKeyLength   = 64
	maxLabelLength = 128
)

var keyRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// FieldDef defines a single metadata field on a category. Type-specific
// constraints are only meaningful for their matching Type and are ignored
// otherwise.
type FieldDef struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Type     Type   `json:"type"`
	Required bool   `json:"required,omitempty"`
	Help     string `json:"help,omitempty"`

	// select / multiselect
	Options  []string `json:"options,omitempty"`
	MaxItems *int     `json:"max_items,omitempty"` // multiselect only

	// number
	Min     *float64 `json:"min,omitempty"`
	Max     *float64 `json:"max,omitempty"`
	Integer bool     `json:"integer,omitempty"`

	// text
	MaxLength *int   `json:"max_length,omitempty"`
	Pattern   string `json:"pattern,omitempty"`
}

// Parse decodes a category's own schema from raw JSONB. An empty or nil input
// (including the JSON literals "null", "" and "[]") yields a nil slice.
func Parse(raw []byte) ([]FieldDef, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var fields []FieldDef
	if err := json.Unmarshal([]byte(trimmed), &fields); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSchema, err)
	}
	return fields, nil
}

// ValidateSchema checks that a category's own field definitions are well-formed.
// It is called on category create/update, before persisting the schema.
func ValidateSchema(fields []FieldDef) error {
	seen := make(map[string]bool, len(fields))
	for i := range fields {
		f := fields[i]
		if !keyRe.MatchString(f.Key) || len(f.Key) > maxKeyLength {
			return fmt.Errorf("%w: field key %q must match ^[a-z][a-z0-9_]*$ and be at most %d chars", ErrInvalidSchema, f.Key, maxKeyLength)
		}
		if seen[f.Key] {
			return fmt.Errorf("%w: duplicate field key %q", ErrInvalidSchema, f.Key)
		}
		seen[f.Key] = true

		if strings.TrimSpace(f.Label) == "" {
			return fmt.Errorf("%w: field %q: label is required", ErrInvalidSchema, f.Key)
		}
		if utf8.RuneCountInString(f.Label) > maxLabelLength {
			return fmt.Errorf("%w: field %q: label too long (max %d)", ErrInvalidSchema, f.Key, maxLabelLength)
		}

		if err := validateFieldConstraints(f); err != nil {
			return err
		}
	}
	return nil
}

func validateFieldConstraints(f FieldDef) error {
	switch f.Type {
	case TypeSelect, TypeMultiselect:
		if len(f.Options) == 0 {
			return fmt.Errorf("%w: field %q: %s requires at least one option", ErrInvalidSchema, f.Key, f.Type)
		}
		optSeen := make(map[string]bool, len(f.Options))
		for _, opt := range f.Options {
			if strings.TrimSpace(opt) == "" {
				return fmt.Errorf("%w: field %q: options cannot be empty", ErrInvalidSchema, f.Key)
			}
			if optSeen[opt] {
				return fmt.Errorf("%w: field %q: duplicate option %q", ErrInvalidSchema, f.Key, opt)
			}
			optSeen[opt] = true
		}
		if f.Type == TypeMultiselect && f.MaxItems != nil && *f.MaxItems < 1 {
			return fmt.Errorf("%w: field %q: max_items must be >= 1", ErrInvalidSchema, f.Key)
		}
	case TypeNumber:
		if f.Min != nil && f.Max != nil && *f.Min > *f.Max {
			return fmt.Errorf("%w: field %q: min must be <= max", ErrInvalidSchema, f.Key)
		}
	case TypeText:
		if f.MaxLength != nil && *f.MaxLength < 1 {
			return fmt.Errorf("%w: field %q: max_length must be >= 1", ErrInvalidSchema, f.Key)
		}
		if f.Pattern != "" {
			if _, err := regexp.Compile(f.Pattern); err != nil {
				return fmt.Errorf("%w: field %q: invalid pattern: %v", ErrInvalidSchema, f.Key, err)
			}
		}
	case TypeBoolean:
		// no extra constraints
	default:
		return fmt.Errorf("%w: field %q: unknown type %q", ErrInvalidSchema, f.Key, f.Type)
	}
	return nil
}

// Merge combines the own-schemas of a category's ancestor chain (root first,
// leaf last) into the category's effective schema. Fields are ordered by first
// appearance; a descendant redefining an ancestor's key replaces the definition
// in place, keeping the ancestor's position for stable UI ordering.
func Merge(chainRootToLeaf [][]FieldDef) []FieldDef {
	idx := make(map[string]int)
	out := make([]FieldDef, 0)
	for _, level := range chainRootToLeaf {
		for _, f := range level {
			if i, ok := idx[f.Key]; ok {
				out[i] = f
				continue
			}
			idx[f.Key] = len(out)
			out = append(out, f)
		}
	}
	return out
}

// ValidateValues validates submitted values against an effective schema and
// returns a canonical map suitable for persisting as JSONB. Unknown keys are
// rejected, required fields must be present and non-empty, and every value is
// type- and constraint-checked. Absent or empty optional fields are omitted
// from the result rather than stored as null.
func ValidateValues(effective []FieldDef, values map[string]any) (map[string]any, error) {
	byKey := make(map[string]FieldDef, len(effective))
	for _, f := range effective {
		byKey[f.Key] = f
	}

	// Reject keys the schema doesn't define — a strong signal the client is
	// out of sync with the category rather than silently dropping data.
	for k := range values {
		if _, ok := byKey[k]; !ok {
			return nil, fmt.Errorf("%w: unknown field %q", ErrInvalidValues, k)
		}
	}

	out := make(map[string]any)
	for _, f := range effective {
		raw, present := values[f.Key]
		if !present || isEmpty(raw) {
			if f.Required {
				return nil, fmt.Errorf("%w: field %q is required", ErrInvalidValues, f.Key)
			}
			continue
		}
		canonical, err := coerceValue(f, raw)
		if err != nil {
			return nil, err
		}
		out[f.Key] = canonical
	}
	return out, nil
}

// CoerceFilterValue coerces a raw query-string filter value into the field's
// underlying JSON type so a JSONB predicate compares like with like. Unlike
// ValidateValues it does NOT enforce required/option/length constraints — an
// out-of-range or non-option filter value is allowed and simply matches no
// rows. Its only job is to guarantee the value's JSON type matches the field
// (numbers as numbers, booleans as booleans) so containment and numeric range
// comparisons behave correctly.
//
// For text/select/multiselect it returns the trimmed string; multiselect
// callers wrap it in a single-element array for array containment. An empty
// value is rejected so callers can treat "present but empty" as no filter.
func CoerceFilterValue(f FieldDef, raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("%w: field %q filter value is empty", ErrInvalidValues, f.Key)
	}
	switch f.Type {
	case TypeNumber:
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: field %q filter must be a number", ErrInvalidValues, f.Key)
		}
		return n, nil
	case TypeBoolean:
		switch strings.ToLower(raw) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		return nil, fmt.Errorf("%w: field %q filter must be true or false", ErrInvalidValues, f.Key)
	case TypeText, TypeSelect, TypeMultiselect:
		return raw, nil
	default:
		return nil, fmt.Errorf("%w: field %q: unknown type %q", ErrInvalidValues, f.Key, f.Type)
	}
}

// isEmpty reports whether a submitted value should be treated as "not provided".
func isEmpty(v any) bool {
	switch val := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(val) == ""
	case []any:
		return len(val) == 0
	case []string:
		return len(val) == 0
	default:
		return false
	}
}

func coerceValue(f FieldDef, raw any) (any, error) {
	switch f.Type {
	case TypeText:
		return coerceText(f, raw)
	case TypeNumber:
		return coerceNumber(f, raw)
	case TypeSelect:
		return coerceSelect(f, raw)
	case TypeMultiselect:
		return coerceMultiselect(f, raw)
	case TypeBoolean:
		return coerceBoolean(f, raw)
	default:
		return nil, fmt.Errorf("%w: field %q: unknown type %q", ErrInvalidValues, f.Key, f.Type)
	}
}

func coerceText(f FieldDef, raw any) (any, error) {
	s, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("%w: field %q must be text", ErrInvalidValues, f.Key)
	}
	s = strings.TrimSpace(s)
	if f.MaxLength != nil && utf8.RuneCountInString(s) > *f.MaxLength {
		return nil, fmt.Errorf("%w: field %q exceeds max length %d", ErrInvalidValues, f.Key, *f.MaxLength)
	}
	if f.Pattern != "" {
		re, err := regexp.Compile(f.Pattern)
		if err != nil {
			return nil, fmt.Errorf("%w: field %q has an invalid pattern", ErrInvalidValues, f.Key)
		}
		if !re.MatchString(s) {
			return nil, fmt.Errorf("%w: field %q does not match the required pattern", ErrInvalidValues, f.Key)
		}
	}
	return s, nil
}

func coerceNumber(f FieldDef, raw any) (any, error) {
	n, ok := toFloat(raw)
	if !ok {
		return nil, fmt.Errorf("%w: field %q must be a number", ErrInvalidValues, f.Key)
	}
	if f.Integer && n != math.Trunc(n) {
		return nil, fmt.Errorf("%w: field %q must be a whole number", ErrInvalidValues, f.Key)
	}
	if f.Min != nil && n < *f.Min {
		return nil, fmt.Errorf("%w: field %q must be >= %v", ErrInvalidValues, f.Key, *f.Min)
	}
	if f.Max != nil && n > *f.Max {
		return nil, fmt.Errorf("%w: field %q must be <= %v", ErrInvalidValues, f.Key, *f.Max)
	}
	return n, nil
}

func coerceSelect(f FieldDef, raw any) (any, error) {
	s, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("%w: field %q must be one of the allowed options", ErrInvalidValues, f.Key)
	}
	if !containsString(f.Options, s) {
		return nil, fmt.Errorf("%w: field %q has an invalid option %q", ErrInvalidValues, f.Key, s)
	}
	return s, nil
}

func coerceMultiselect(f FieldDef, raw any) (any, error) {
	items, err := toStringSlice(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: field %q must be a list of options", ErrInvalidValues, f.Key)
	}
	out := make([]string, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if !containsString(f.Options, item) {
			return nil, fmt.Errorf("%w: field %q has an invalid option %q", ErrInvalidValues, f.Key, item)
		}
		if seen[item] {
			continue // dedupe, preserving first-seen order
		}
		seen[item] = true
		out = append(out, item)
	}
	if f.MaxItems != nil && len(out) > *f.MaxItems {
		return nil, fmt.Errorf("%w: field %q allows at most %d items", ErrInvalidValues, f.Key, *f.MaxItems)
	}
	return out, nil
}

func coerceBoolean(f FieldDef, raw any) (any, error) {
	switch v := raw.(type) {
	case bool:
		return v, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
	}
	return nil, fmt.Errorf("%w: field %q must be true or false", ErrInvalidValues, f.Key)
}

// toFloat accepts the numeric shapes JSON decoding can produce (float64,
// json.Number) plus numeric strings from lenient clients.
func toFloat(raw any) (float64, bool) {
	switch v := raw.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// toStringSlice normalizes an array value to []string, rejecting non-string elements.
func toStringSlice(raw any) ([]string, error) {
	switch v := raw.(type) {
	case []string:
		return v, nil
	case []any:
		out := make([]string, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("element %d is not a string", i)
			}
			out[i] = s
		}
		return out, nil
	default:
		return nil, fmt.Errorf("not a list")
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
