package bencode

import (
	"fmt"
	"reflect"
	"strings"
)

// Marshal encodes a Go value (typically a struct) into bencoded bytes.
func Marshal(v interface{}) ([]byte, error) {
	return EncodeBytes(v)
}

// Unmarshal decodes bencoded bytes into a Go value pointed to by v.
// v must be a non-nil pointer.
func Unmarshal(data []byte, v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("bencode: Unmarshal requires a non-nil pointer")
	}

	// Decode using raw mode to preserve binary data as []byte
	decoded, err := DecodeBytesRaw(data)
	if err != nil {
		return err
	}

	return unmarshalValue(decoded, rv.Elem())
}

func unmarshalValue(src interface{}, dst reflect.Value) error {
	// Handle pointer types
	if dst.Kind() == reflect.Ptr {
		if src == nil {
			dst.Set(reflect.Zero(dst.Type()))
			return nil
		}
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		return unmarshalValue(src, dst.Elem())
	}

	if src == nil {
		return nil
	}

	switch v := src.(type) {
	case int64:
		return unmarshalInt(v, dst)
	case []byte:
		return unmarshalBytes(v, dst)
	case string:
		return unmarshalString(v, dst)
	case []interface{}:
		return unmarshalList(v, dst)
	case map[string]interface{}:
		return unmarshalDict(v, dst)
	default:
		return fmt.Errorf("bencode: unexpected decoded type %T", src)
	}
}

func unmarshalInt(v int64, dst reflect.Value) error {
	switch dst.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		dst.SetInt(v)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v < 0 {
			return fmt.Errorf("bencode: cannot assign negative integer to unsigned type")
		}
		dst.SetUint(uint64(v))
	case reflect.Interface:
		dst.Set(reflect.ValueOf(v))
	default:
		return fmt.Errorf("bencode: cannot assign integer to %s", dst.Type())
	}
	return nil
}

func unmarshalBytes(v []byte, dst reflect.Value) error {
	switch dst.Kind() {
	case reflect.String:
		dst.SetString(string(v))
	case reflect.Slice:
		if dst.Type().Elem().Kind() == reflect.Uint8 {
			dst.SetBytes(append([]byte(nil), v...))
		} else {
			return fmt.Errorf("bencode: cannot assign bytes to %s", dst.Type())
		}
	case reflect.Interface:
		dst.Set(reflect.ValueOf(v))
	default:
		return fmt.Errorf("bencode: cannot assign string/bytes to %s", dst.Type())
	}
	return nil
}

func unmarshalString(v string, dst reflect.Value) error {
	switch dst.Kind() {
	case reflect.String:
		dst.SetString(v)
	case reflect.Slice:
		if dst.Type().Elem().Kind() == reflect.Uint8 {
			dst.SetBytes([]byte(v))
		} else {
			return fmt.Errorf("bencode: cannot assign string to %s", dst.Type())
		}
	case reflect.Interface:
		dst.Set(reflect.ValueOf(v))
	default:
		return fmt.Errorf("bencode: cannot assign string to %s", dst.Type())
	}
	return nil
}

func unmarshalList(v []interface{}, dst reflect.Value) error {
	switch dst.Kind() {
	case reflect.Slice:
		slice := reflect.MakeSlice(dst.Type(), len(v), len(v))
		for i, item := range v {
			if err := unmarshalValue(item, slice.Index(i)); err != nil {
				return fmt.Errorf("bencode: error unmarshaling list element %d: %w", i, err)
			}
		}
		dst.Set(slice)
	case reflect.Interface:
		dst.Set(reflect.ValueOf(v))
	default:
		return fmt.Errorf("bencode: cannot assign list to %s", dst.Type())
	}
	return nil
}

func unmarshalDict(v map[string]interface{}, dst reflect.Value) error {
	switch dst.Kind() {
	case reflect.Struct:
		return unmarshalStruct(v, dst)
	case reflect.Map:
		if dst.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("bencode: map key must be string, got %s", dst.Type().Key())
		}
		if dst.IsNil() {
			dst.Set(reflect.MakeMap(dst.Type()))
		}
		for key, val := range v {
			mapVal := reflect.New(dst.Type().Elem()).Elem()
			if err := unmarshalValue(val, mapVal); err != nil {
				return fmt.Errorf("bencode: error unmarshaling map value for key %q: %w", key, err)
			}
			dst.SetMapIndex(reflect.ValueOf(key), mapVal)
		}
	case reflect.Interface:
		dst.Set(reflect.ValueOf(v))
	default:
		return fmt.Errorf("bencode: cannot assign dict to %s", dst.Type())
	}
	return nil
}

func unmarshalStruct(v map[string]interface{}, dst reflect.Value) error {
	rt := dst.Type()

	// Build a map from bencode field name -> struct field index
	fieldMap := make(map[string]int)
	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		if !sf.IsExported() {
			continue
		}
		tag := sf.Tag.Get("bencode")
		if tag == "-" {
			continue
		}
		name, _ := parseTag(tag)
		if name == "" {
			name = sf.Name
		}
		fieldMap[name] = i
	}

	for key, val := range v {
		idx, ok := fieldMap[key]
		if !ok {
			// Skip unknown fields
			continue
		}
		if err := unmarshalValue(val, dst.Field(idx)); err != nil {
			return fmt.Errorf("bencode: error unmarshaling field %q: %w", key, err)
		}
	}

	return nil
}

// tagOptions is a string that follows a comma in a struct field's bencode tag.
type tagOptions string

// parseTag splits a struct field's bencode tag into its name and options.
func parseTag(tag string) (string, tagOptions) {
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx], tagOptions(tag[idx+1:])
	}
	return tag, ""
}

// Contains reports whether opts contains the given option name.
func (o tagOptions) Contains(name string) bool {
	if len(o) == 0 {
		return false
	}
	s := string(o)
	for s != "" {
		var opt string
		if idx := strings.Index(s, ","); idx >= 0 {
			opt, s = s[:idx], s[idx+1:]
		} else {
			opt, s = s, ""
		}
		if opt == name {
			return true
		}
	}
	return false
}
