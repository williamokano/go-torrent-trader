package bencode

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
)

// Encode writes the bencoded representation of v to w.
// Supported types: int, int8-int64, uint, uint8-uint64, string, []byte,
// slices, arrays, maps with string keys, and structs with bencode tags.
func Encode(w io.Writer, v interface{}) error {
	data, err := encodeValue(v)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// EncodeBytes returns the bencoded representation of v as a byte slice.
func EncodeBytes(v interface{}) ([]byte, error) {
	return encodeValue(v)
}

func encodeValue(v interface{}) ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("bencode: cannot encode nil")
	}
	return encodeReflect(reflect.ValueOf(v))
}

func encodeReflect(rv reflect.Value) ([]byte, error) {
	// Handle interface and pointer indirection
	for rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil, fmt.Errorf("bencode: cannot encode nil")
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return encodeInt(rv.Int()), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return encodeUint(rv.Uint()), nil

	case reflect.String:
		return encodeString(rv.String()), nil

	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			// []byte -> bencode string
			return encodeByteSlice(rv.Bytes()), nil
		}
		return encodeSlice(rv)

	case reflect.Array:
		return encodeSlice(rv)

	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("bencode: map keys must be strings, got %s", rv.Type().Key().Kind())
		}
		return encodeMap(rv)

	case reflect.Struct:
		return encodeStruct(rv)

	default:
		return nil, fmt.Errorf("bencode: unsupported type: %s", rv.Type())
	}
}

func encodeInt(n int64) []byte {
	var buf bytes.Buffer
	buf.WriteByte('i')
	buf.WriteString(strconv.FormatInt(n, 10))
	buf.WriteByte('e')
	return buf.Bytes()
}

func encodeUint(n uint64) []byte {
	var buf bytes.Buffer
	buf.WriteByte('i')
	buf.WriteString(strconv.FormatUint(n, 10))
	buf.WriteByte('e')
	return buf.Bytes()
}

func encodeString(s string) []byte {
	var buf bytes.Buffer
	buf.WriteString(strconv.Itoa(len(s)))
	buf.WriteByte(':')
	buf.WriteString(s)
	return buf.Bytes()
}

func encodeByteSlice(b []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString(strconv.Itoa(len(b)))
	buf.WriteByte(':')
	buf.Write(b)
	return buf.Bytes()
}

func encodeSlice(rv reflect.Value) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('l')
	for i := 0; i < rv.Len(); i++ {
		encoded, err := encodeReflect(rv.Index(i))
		if err != nil {
			return nil, fmt.Errorf("bencode: error encoding list element %d: %w", i, err)
		}
		buf.Write(encoded)
	}
	buf.WriteByte('e')
	return buf.Bytes(), nil
}

func encodeMap(rv reflect.Value) ([]byte, error) {
	// Per BitTorrent spec, dict keys must be sorted
	keys := make([]string, 0, rv.Len())
	for _, k := range rv.MapKeys() {
		keys = append(keys, k.String())
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.WriteByte('d')
	for _, key := range keys {
		buf.Write(encodeString(key))
		val := rv.MapIndex(reflect.ValueOf(key))
		encoded, err := encodeReflect(val)
		if err != nil {
			return nil, fmt.Errorf("bencode: error encoding dict value for key %q: %w", key, err)
		}
		buf.Write(encoded)
	}
	buf.WriteByte('e')
	return buf.Bytes(), nil
}

func encodeStruct(rv reflect.Value) ([]byte, error) {
	rt := rv.Type()

	type field struct {
		name  string
		value reflect.Value
	}

	var fields []field

	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		fv := rv.Field(i)

		// Skip unexported fields
		if !sf.IsExported() {
			continue
		}

		tag := sf.Tag.Get("bencode")
		if tag == "-" {
			continue
		}

		name, opts := parseTag(tag)
		if name == "" {
			name = sf.Name
		}

		// Handle omitempty
		if opts.Contains("omitempty") && isZero(fv) {
			continue
		}

		fields = append(fields, field{name: name, value: fv})
	}

	// Sort fields by key name per spec
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].name < fields[j].name
	})

	var buf bytes.Buffer
	buf.WriteByte('d')
	for _, f := range fields {
		buf.Write(encodeString(f.name))
		encoded, err := encodeReflect(f.value)
		if err != nil {
			return nil, fmt.Errorf("bencode: error encoding struct field %q: %w", f.name, err)
		}
		buf.Write(encoded)
	}
	buf.WriteByte('e')
	return buf.Bytes(), nil
}

func isZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return false
}
