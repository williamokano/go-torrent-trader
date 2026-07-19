package metadata

import (
	"errors"
	"reflect"
	"testing"
)

func TestCoerceFilterValue(t *testing.T) {
	tests := []struct {
		name    string
		field   FieldDef
		raw     string
		want    any
		wantErr bool
	}{
		{"number", FieldDef{Key: "year", Type: TypeNumber}, "2024", float64(2024), false},
		{"number with spaces", FieldDef{Key: "year", Type: TypeNumber}, " 2024 ", float64(2024), false},
		{"number float", FieldDef{Key: "rating", Type: TypeNumber}, "8.5", 8.5, false},
		{"number invalid", FieldDef{Key: "year", Type: TypeNumber}, "abc", nil, true},
		{"boolean true", FieldDef{Key: "hdr", Type: TypeBoolean}, "true", true, false},
		{"boolean TRUE", FieldDef{Key: "hdr", Type: TypeBoolean}, "TRUE", true, false},
		{"boolean false", FieldDef{Key: "hdr", Type: TypeBoolean}, "false", false, false},
		{"boolean invalid", FieldDef{Key: "hdr", Type: TypeBoolean}, "yes", nil, true},
		{"text", FieldDef{Key: "title", Type: TypeText}, " The Matrix ", "The Matrix", false},
		{"select", FieldDef{Key: "codec", Type: TypeSelect}, "x265", "x265", false},
		{"multiselect returns string", FieldDef{Key: "genres", Type: TypeMultiselect}, "Action", "Action", false},
		{"empty rejected", FieldDef{Key: "title", Type: TypeText}, "   ", nil, true},
		{"unknown type rejected", FieldDef{Key: "x", Type: Type("mystery")}, "v", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CoerceFilterValue(tt.field, tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got value %#v", got)
				}
				if !errors.Is(err, ErrInvalidValues) {
					t.Errorf("error = %v, want it to wrap ErrInvalidValues", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v (%T), want %#v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}
