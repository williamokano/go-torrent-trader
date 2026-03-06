package bencode

import (
	"bytes"
	"testing"
)

type TorrentInfo struct {
	PieceLength int64  `bencode:"piece length"`
	Pieces      []byte `bencode:"pieces"`
	Name        string `bencode:"name"`
	Length      int64  `bencode:"length"`
}

type Torrent struct {
	Announce string      `bencode:"announce"`
	Info     TorrentInfo `bencode:"info"`
}

func TestMarshalUnmarshalTorrent(t *testing.T) {
	original := Torrent{
		Announce: "http://tracker.example.com/announce",
		Info: TorrentInfo{
			PieceLength: 262144,
			Pieces:      []byte{0xDE, 0xAD, 0xBE, 0xEF},
			Name:        "example.txt",
			Length:      1048576,
		},
	}

	data, err := Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Torrent
	err = Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Announce != original.Announce {
		t.Errorf("Announce: got %q, want %q", decoded.Announce, original.Announce)
	}
	if decoded.Info.Name != original.Info.Name {
		t.Errorf("Info.Name: got %q, want %q", decoded.Info.Name, original.Info.Name)
	}
	if decoded.Info.Length != original.Info.Length {
		t.Errorf("Info.Length: got %d, want %d", decoded.Info.Length, original.Info.Length)
	}
	if decoded.Info.PieceLength != original.Info.PieceLength {
		t.Errorf("Info.PieceLength: got %d, want %d", decoded.Info.PieceLength, original.Info.PieceLength)
	}
	if !bytes.Equal(decoded.Info.Pieces, original.Info.Pieces) {
		t.Errorf("Info.Pieces: got %v, want %v", decoded.Info.Pieces, original.Info.Pieces)
	}
}

type SkipFields struct {
	Included string `bencode:"included"`
	Skipped  string `bencode:"-"`
	Also     string `bencode:"also"`
}

func TestMarshalSkipField(t *testing.T) {
	v := SkipFields{
		Included: "yes",
		Skipped:  "should not appear",
		Also:     "present",
	}

	data, err := Marshal(v)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// Decode as raw dict to verify "Skipped" is not present
	decoded, err := DecodeBytes(data)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	dict := decoded.(map[string]interface{})
	if _, ok := dict["Skipped"]; ok {
		t.Error("field with bencode:\"-\" should not be encoded")
	}
	if dict["included"].(string) != "yes" {
		t.Errorf("included: got %q, want %q", dict["included"], "yes")
	}
	if dict["also"].(string) != "present" {
		t.Errorf("also: got %q, want %q", dict["also"], "present")
	}
}

func TestUnmarshalSkipField(t *testing.T) {
	data := []byte("d4:also7:present8:included3:yes7:skipped6:hiddene")

	var v SkipFields
	err := Unmarshal(data, &v)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if v.Included != "yes" {
		t.Errorf("Included: got %q, want %q", v.Included, "yes")
	}
	if v.Skipped != "" {
		t.Errorf("Skipped should be empty, got %q", v.Skipped)
	}
	if v.Also != "present" {
		t.Errorf("Also: got %q, want %q", v.Also, "present")
	}
}

type OmitEmpty struct {
	Name  string `bencode:"name"`
	Value int64  `bencode:"value,omitempty"`
	Data  []byte `bencode:"data,omitempty"`
}

func TestMarshalOmitempty(t *testing.T) {
	v := OmitEmpty{
		Name:  "test",
		Value: 0,    // zero value, should be omitted
		Data:  nil,  // zero value, should be omitted
	}

	data, err := Marshal(v)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	decoded, err := DecodeBytes(data)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	dict := decoded.(map[string]interface{})
	if _, ok := dict["value"]; ok {
		t.Error("omitempty zero int should not be encoded")
	}
	if _, ok := dict["data"]; ok {
		t.Error("omitempty nil slice should not be encoded")
	}
	if dict["name"].(string) != "test" {
		t.Errorf("name: got %q, want %q", dict["name"], "test")
	}
}

func TestMarshalOmitemptyNonZero(t *testing.T) {
	v := OmitEmpty{
		Name:  "test",
		Value: 42,
		Data:  []byte{1, 2, 3},
	}

	data, err := Marshal(v)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	decoded, err := DecodeBytes(data)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	dict := decoded.(map[string]interface{})
	if _, ok := dict["value"]; !ok {
		t.Error("non-zero omitempty int should be encoded")
	}
	if _, ok := dict["data"]; !ok {
		t.Error("non-empty omitempty slice should be encoded")
	}
}

type Nested struct {
	Inner InnerStruct `bencode:"inner"`
	Value string      `bencode:"value"`
}

type InnerStruct struct {
	Field1 string `bencode:"field1"`
	Field2 int64  `bencode:"field2"`
}

func TestMarshalUnmarshalNested(t *testing.T) {
	original := Nested{
		Inner: InnerStruct{
			Field1: "hello",
			Field2: 99,
		},
		Value: "world",
	}

	data, err := Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Nested
	err = Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Value != original.Value {
		t.Errorf("Value: got %q, want %q", decoded.Value, original.Value)
	}
	if decoded.Inner.Field1 != original.Inner.Field1 {
		t.Errorf("Inner.Field1: got %q, want %q", decoded.Inner.Field1, original.Inner.Field1)
	}
	if decoded.Inner.Field2 != original.Inner.Field2 {
		t.Errorf("Inner.Field2: got %d, want %d", decoded.Inner.Field2, original.Inner.Field2)
	}
}

func TestUnmarshalNonPointerError(t *testing.T) {
	var v Torrent
	err := Unmarshal([]byte("de"), v) // not a pointer
	if err == nil {
		t.Fatal("expected error for non-pointer argument")
	}
}

func TestUnmarshalNilPointerError(t *testing.T) {
	err := Unmarshal([]byte("de"), (*Torrent)(nil))
	if err == nil {
		t.Fatal("expected error for nil pointer argument")
	}
}

func TestUnmarshalUnknownFieldsIgnored(t *testing.T) {
	// Dict with extra fields not in the struct
	data := []byte("d7:unknown5:value4:name4:teste")

	type Simple struct {
		Name string `bencode:"name"`
	}

	var v Simple
	err := Unmarshal(data, &v)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if v.Name != "test" {
		t.Errorf("Name: got %q, want %q", v.Name, "test")
	}
}

type UnexportedField struct {
	Public  string `bencode:"public"`
	private string `bencode:"private"` //nolint:unused
}

func TestMarshalUnexportedFieldSkipped(t *testing.T) {
	v := UnexportedField{
		Public: "visible",
	}

	data, err := Marshal(v)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	decoded, err := DecodeBytes(data)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	dict := decoded.(map[string]interface{})
	if _, ok := dict["private"]; ok {
		t.Error("unexported field should not be encoded")
	}
	if dict["public"].(string) != "visible" {
		t.Errorf("public: got %q, want %q", dict["public"], "visible")
	}
}

func TestMarshalStructSortedKeys(t *testing.T) {
	type Alpha struct {
		Zebra string `bencode:"zebra"`
		Apple string `bencode:"apple"`
		Mango string `bencode:"mango"`
	}

	v := Alpha{
		Zebra: "z",
		Apple: "a",
		Mango: "m",
	}

	data, err := Marshal(v)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// Keys should be sorted: apple, mango, zebra
	want := "d5:apple1:a5:mango1:m5:zebra1:ze"
	if string(data) != want {
		t.Errorf("got %q, want %q", string(data), want)
	}
}

func TestUnmarshalIntoMap(t *testing.T) {
	data := []byte("d3:foo3:bar3:numi42ee")

	var m map[string]interface{}
	err := Unmarshal(data, &m)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	fooBytes, ok := m["foo"].([]byte)
	if !ok {
		t.Fatalf("foo: expected []byte, got %T", m["foo"])
	}
	if string(fooBytes) != "bar" {
		t.Errorf("foo: got %q, want %q", string(fooBytes), "bar")
	}

	num, ok := m["num"].(int64)
	if !ok {
		t.Fatalf("num: expected int64, got %T", m["num"])
	}
	if num != 42 {
		t.Errorf("num: got %d, want %d", num, 42)
	}
}

func TestMarshalUnmarshalWithPointerField(t *testing.T) {
	type WithPtr struct {
		Name  string `bencode:"name"`
		Count *int64 `bencode:"count"`
	}

	count := int64(7)
	original := WithPtr{Name: "test", Count: &count}

	data, err := Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded WithPtr
	err = Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Name != original.Name {
		t.Errorf("Name: got %q, want %q", decoded.Name, original.Name)
	}
	if decoded.Count == nil || *decoded.Count != *original.Count {
		t.Errorf("Count: got %v, want %v", decoded.Count, original.Count)
	}
}
