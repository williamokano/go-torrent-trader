package bencode

import (
	"bytes"
	"testing"
)

func TestEncodeInteger(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  string
	}{
		{"positive int", 42, "i42e"},
		{"negative int", -1, "i-1e"},
		{"zero", 0, "i0e"},
		{"int64", int64(9999999999), "i9999999999e"},
		{"int8", int8(7), "i7e"},
		{"uint", uint(100), "i100e"},
		{"uint64", uint64(18446744073709551615), "i18446744073709551615e"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EncodeBytes(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("got %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestEncodeString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "spam", "4:spam"},
		{"empty", "", "0:"},
		{"with spaces", "hello world", "11:hello world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EncodeBytes(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("got %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestEncodeByteSlice(t *testing.T) {
	input := []byte{0x00, 0x01, 0x02, 0x03}
	got, err := EncodeBytes(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []byte("4:\x00\x01\x02\x03")
	if !bytes.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestEncodeList(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  string
	}{
		{
			"string list",
			[]string{"spam", "eggs"},
			"l4:spam4:eggse",
		},
		{
			"interface list",
			[]interface{}{"spam", int64(42)},
			"l4:spami42ee",
		},
		{
			"empty list",
			[]interface{}{},
			"le",
		},
		{
			"nested list",
			[]interface{}{[]interface{}{"spam"}, int64(42)},
			"ll4:spamei42ee",
		},
		{
			"int list",
			[]int{1, 2, 3},
			"li1ei2ei3ee",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EncodeBytes(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("got %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestEncodeDictSortedKeys(t *testing.T) {
	input := map[string]interface{}{
		"spam": "eggs",
		"cow":  "moo",
	}
	got, err := EncodeBytes(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Keys must be sorted: cow before spam
	want := "d3:cow3:moo4:spam4:eggse"
	if string(got) != want {
		t.Errorf("got %q, want %q", string(got), want)
	}
}

func TestEncodeDictEmpty(t *testing.T) {
	input := map[string]interface{}{}
	got, err := EncodeBytes(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "de" {
		t.Errorf("got %q, want %q", string(got), "de")
	}
}

func TestEncodeDictNested(t *testing.T) {
	input := map[string]interface{}{
		"info": map[string]interface{}{
			"name": "test",
		},
	}
	got, err := EncodeBytes(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "d4:infod4:name4:testee"
	if string(got) != want {
		t.Errorf("got %q, want %q", string(got), want)
	}
}

func TestEncodeToWriter(t *testing.T) {
	var buf bytes.Buffer
	err := Encode(&buf, "spam")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.String() != "4:spam" {
		t.Errorf("got %q, want %q", buf.String(), "4:spam")
	}
}

func TestEncodeNilError(t *testing.T) {
	_, err := EncodeBytes(nil)
	if err == nil {
		t.Fatal("expected error for nil value")
	}
}

func TestEncodeRoundtrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"integer", "i42e"},
		{"string", "4:spam"},
		{"list", "l4:spam4:eggse"},
		{"dict", "d3:cow3:moo4:spam4:eggse"},
		{"nested", "d4:infod4:name4:testee"},
		{"complex", "d8:announcei1e4:infod6:lengthi100e4:name4:testee"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Decode
			decoded, err := DecodeBytes([]byte(tt.input))
			if err != nil {
				t.Fatalf("decode error: %v", err)
			}
			// Re-encode
			encoded, err := EncodeBytes(decoded)
			if err != nil {
				t.Fatalf("encode error: %v", err)
			}
			if string(encoded) != tt.input {
				t.Errorf("roundtrip failed: got %q, want %q", string(encoded), tt.input)
			}
		})
	}
}

func TestEncodeBinaryRoundtrip(t *testing.T) {
	// Test that binary data survives a roundtrip
	original := []byte{0x00, 0xFF, 0x01, 0xFE}
	encoded, err := EncodeBytes(original)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}

	decoded, err := DecodeBytesRaw(encoded)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	decodedBytes, ok := decoded.([]byte)
	if !ok {
		t.Fatalf("expected []byte, got %T", decoded)
	}

	if !bytes.Equal(decodedBytes, original) {
		t.Errorf("binary roundtrip failed: got %v, want %v", decodedBytes, original)
	}
}

func TestEncodeUnsupportedType(t *testing.T) {
	_, err := EncodeBytes(3.14)
	if err == nil {
		t.Fatal("expected error for unsupported type float64")
	}
}

func TestEncodeMapNonStringKey(t *testing.T) {
	_, err := EncodeBytes(map[int]string{1: "one"})
	if err == nil {
		t.Fatal("expected error for non-string map keys")
	}
}
