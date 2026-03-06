package bencode

import (
	"bytes"
	"errors"
	"testing"
)

func TestDecodeInteger(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr error
	}{
		{"positive", "i42e", 42, nil},
		{"negative", "i-1e", -1, nil},
		{"zero", "i0e", 0, nil},
		{"large number", "i9999999999e", 9999999999, nil},
		{"large negative", "i-9999999999e", -9999999999, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeBytes([]byte(tt.input))
			if tt.wantErr != nil {
				if err == nil || !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.(int64) != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDecodeIntegerErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"leading zero", "i03e", ErrLeadingZero},
		{"negative zero", "i-0e", ErrNegativeZero},
		{"negative leading zero", "i-03e", ErrLeadingZero},
		{"empty integer", "ie", ErrInvalidFormat},
		{"invalid char", "i1x2e", ErrInvalidIntChar},
		{"unexpected eof", "i42", ErrUnexpectedEOF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeBytes([]byte(tt.input))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestDecodeString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "4:spam", "spam"},
		{"empty", "0:", ""},
		{"with spaces", "11:hello world", "hello world"},
		{"with colon", "5:a:b:c", "a:b:c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeBytes([]byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.(string) != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecodeStringBinary(t *testing.T) {
	// Binary data with null bytes
	input := []byte("4:\x00\x01\x02\x03")
	got, err := DecodeBytesRaw(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotBytes, ok := got.([]byte)
	if !ok {
		t.Fatalf("expected []byte, got %T", got)
	}
	want := []byte{0x00, 0x01, 0x02, 0x03}
	if !bytes.Equal(gotBytes, want) {
		t.Errorf("got %v, want %v", gotBytes, want)
	}
}

func TestDecodeStringErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"missing colon", "4spam"},
		{"truncated data", "10:spam"},
		{"empty input", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeBytes([]byte(tt.input))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestDecodeList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, v interface{})
	}{
		{
			"simple list",
			"l4:spam4:eggse",
			func(t *testing.T, v interface{}) {
				list := v.([]interface{})
				if len(list) != 2 {
					t.Fatalf("expected 2 elements, got %d", len(list))
				}
				if list[0].(string) != "spam" {
					t.Errorf("element 0: got %q, want %q", list[0], "spam")
				}
				if list[1].(string) != "eggs" {
					t.Errorf("element 1: got %q, want %q", list[1], "eggs")
				}
			},
		},
		{
			"empty list",
			"le",
			func(t *testing.T, v interface{}) {
				list := v.([]interface{})
				if list != nil {
					t.Errorf("expected nil slice for empty list, got %v", list)
				}
			},
		},
		{
			"mixed types",
			"l4:spami42ee",
			func(t *testing.T, v interface{}) {
				list := v.([]interface{})
				if len(list) != 2 {
					t.Fatalf("expected 2 elements, got %d", len(list))
				}
				if list[0].(string) != "spam" {
					t.Errorf("element 0: got %q, want %q", list[0], "spam")
				}
				if list[1].(int64) != 42 {
					t.Errorf("element 1: got %d, want %d", list[1], 42)
				}
			},
		},
		{
			"nested list",
			"ll4:spamei42ee",
			func(t *testing.T, v interface{}) {
				list := v.([]interface{})
				if len(list) != 2 {
					t.Fatalf("expected 2 elements, got %d", len(list))
				}
				inner := list[0].([]interface{})
				if len(inner) != 1 {
					t.Fatalf("inner list: expected 1 element, got %d", len(inner))
				}
				if inner[0].(string) != "spam" {
					t.Errorf("inner element: got %q, want %q", inner[0], "spam")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeBytes([]byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.check(t, got)
		})
	}
}

func TestDecodeDict(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, v interface{})
	}{
		{
			"simple dict",
			"d3:cow3:moo4:spam4:eggse",
			func(t *testing.T, v interface{}) {
				dict := v.(map[string]interface{})
				if dict["cow"].(string) != "moo" {
					t.Errorf("cow: got %q, want %q", dict["cow"], "moo")
				}
				if dict["spam"].(string) != "eggs" {
					t.Errorf("spam: got %q, want %q", dict["spam"], "eggs")
				}
			},
		},
		{
			"empty dict",
			"de",
			func(t *testing.T, v interface{}) {
				dict := v.(map[string]interface{})
				if len(dict) != 0 {
					t.Errorf("expected empty dict, got %v", dict)
				}
			},
		},
		{
			"nested dict",
			"d4:infod4:name4:testee",
			func(t *testing.T, v interface{}) {
				dict := v.(map[string]interface{})
				inner := dict["info"].(map[string]interface{})
				if inner["name"].(string) != "test" {
					t.Errorf("info.name: got %q, want %q", inner["name"], "test")
				}
			},
		},
		{
			"dict with list value",
			"d4:listl4:spam4:eggsee",
			func(t *testing.T, v interface{}) {
				dict := v.(map[string]interface{})
				list := dict["list"].([]interface{})
				if len(list) != 2 {
					t.Fatalf("expected 2 list elements, got %d", len(list))
				}
			},
		},
		{
			"dict with integer value",
			"d3:numi42ee",
			func(t *testing.T, v interface{}) {
				dict := v.(map[string]interface{})
				if dict["num"].(int64) != 42 {
					t.Errorf("num: got %d, want %d", dict["num"], 42)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeBytes([]byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.check(t, got)
		})
	}
}

func TestDecodeFromReader(t *testing.T) {
	r := bytes.NewReader([]byte("i42e"))
	got, err := Decode(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.(int64) != 42 {
		t.Errorf("got %d, want %d", got, 42)
	}
}

func TestDecodeUnexpectedToken(t *testing.T) {
	_, err := DecodeBytes([]byte("x"))
	if err == nil {
		t.Fatal("expected error for unexpected token")
	}
	if !errors.Is(err, ErrUnexpectedToken) {
		t.Errorf("expected ErrUnexpectedToken, got %v", err)
	}
}

func TestDecodeEmptyInput(t *testing.T) {
	_, err := DecodeBytes([]byte{})
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestDecodeListUnterminatedError(t *testing.T) {
	_, err := DecodeBytes([]byte("l4:spam"))
	if err == nil {
		t.Fatal("expected error for unterminated list")
	}
}

func TestDecodeDictUnterminatedError(t *testing.T) {
	_, err := DecodeBytes([]byte("d3:cow3:moo"))
	if err == nil {
		t.Fatal("expected error for unterminated dict")
	}
}
