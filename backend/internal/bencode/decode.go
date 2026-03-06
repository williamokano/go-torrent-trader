package bencode

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
)

var (
	ErrUnexpectedEOF   = errors.New("bencode: unexpected end of input")
	ErrInvalidFormat    = errors.New("bencode: invalid format")
	ErrLeadingZero     = errors.New("bencode: integer has leading zeros")
	ErrNegativeZero    = errors.New("bencode: negative zero is not allowed")
	ErrInvalidIntChar  = errors.New("bencode: invalid character in integer")
	ErrInvalidStrLen   = errors.New("bencode: invalid string length")
	ErrNegativeStrLen  = errors.New("bencode: negative string length")
	ErrUnexpectedToken = errors.New("bencode: unexpected token")
)

// decoder reads bencoded data from a byte slice.
type decoder struct {
	data []byte
	pos  int
}

// Decode reads a bencoded value from the given reader and returns a Go value.
// Integers are returned as int64, strings as string, lists as []interface{},
// and dicts as map[string]interface{}. Binary data in strings is preserved.
func Decode(r io.Reader) (interface{}, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("bencode: read error: %w", err)
	}
	return DecodeBytes(data)
}

// DecodeBytes decodes a bencoded byte slice into a Go value.
func DecodeBytes(data []byte) (interface{}, error) {
	d := &decoder{data: data}
	val, err := d.decodeValue()
	if err != nil {
		return nil, err
	}
	return val, nil
}

// DecodeBytesRaw is like DecodeBytes but returns strings as []byte to preserve
// binary data. Useful when decoding torrent files where pieces/info_hash are raw bytes.
func DecodeBytesRaw(data []byte) (interface{}, error) {
	d := &decoder{data: data, pos: 0}
	val, err := d.decodeValueRaw()
	if err != nil {
		return nil, err
	}
	return val, nil
}

func (d *decoder) peek() (byte, error) {
	if d.pos >= len(d.data) {
		return 0, ErrUnexpectedEOF
	}
	return d.data[d.pos], nil
}

func (d *decoder) readByte() (byte, error) {
	if d.pos >= len(d.data) {
		return 0, ErrUnexpectedEOF
	}
	b := d.data[d.pos]
	d.pos++
	return b, nil
}

func (d *decoder) decodeValue() (interface{}, error) {
	b, err := d.peek()
	if err != nil {
		return nil, err
	}

	switch {
	case b == 'i':
		return d.decodeInt()
	case b == 'l':
		return d.decodeList()
	case b == 'd':
		return d.decodeDict()
	case b >= '0' && b <= '9':
		return d.decodeString()
	default:
		return nil, fmt.Errorf("%w: unexpected byte '%c' at position %d", ErrUnexpectedToken, b, d.pos)
	}
}

func (d *decoder) decodeValueRaw() (interface{}, error) {
	b, err := d.peek()
	if err != nil {
		return nil, err
	}

	switch {
	case b == 'i':
		return d.decodeInt()
	case b == 'l':
		return d.decodeListRaw()
	case b == 'd':
		return d.decodeDictRaw()
	case b >= '0' && b <= '9':
		return d.decodeStringBytes()
	default:
		return nil, fmt.Errorf("%w: unexpected byte '%c' at position %d", ErrUnexpectedToken, b, d.pos)
	}
}

func (d *decoder) decodeInt() (int64, error) {
	// consume 'i'
	if _, err := d.readByte(); err != nil {
		return 0, err
	}

	start := d.pos
	for {
		b, err := d.peek()
		if err != nil {
			return 0, ErrUnexpectedEOF
		}
		if b == 'e' {
			break
		}
		d.pos++
	}

	numStr := string(d.data[start:d.pos])

	// consume 'e'
	d.pos++

	if len(numStr) == 0 {
		return 0, fmt.Errorf("%w: empty integer", ErrInvalidFormat)
	}

	// Check for negative zero: i-0e
	if numStr == "-0" {
		return 0, ErrNegativeZero
	}

	// Check for leading zeros: i03e, i-03e
	if len(numStr) > 1 && numStr[0] == '0' {
		return 0, ErrLeadingZero
	}
	if len(numStr) > 2 && numStr[0] == '-' && numStr[1] == '0' {
		return 0, ErrLeadingZero
	}

	// Validate characters
	for i, c := range numStr {
		if c == '-' && i == 0 {
			continue
		}
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("%w: '%c'", ErrInvalidIntChar, c)
		}
	}

	val, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %s", ErrInvalidFormat, err)
	}

	return val, nil
}

func (d *decoder) decodeString() (string, error) {
	raw, err := d.decodeStringBytes()
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (d *decoder) decodeStringBytes() ([]byte, error) {
	start := d.pos
	for {
		b, err := d.peek()
		if err != nil {
			return nil, ErrUnexpectedEOF
		}
		if b == ':' {
			break
		}
		if b < '0' || b > '9' {
			return nil, fmt.Errorf("%w: invalid character in string length", ErrInvalidFormat)
		}
		d.pos++
	}

	lenStr := string(d.data[start:d.pos])
	if len(lenStr) == 0 {
		return nil, fmt.Errorf("%w: missing string length", ErrInvalidFormat)
	}

	length, err := strconv.ParseInt(lenStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidStrLen, err)
	}
	if length < 0 {
		return nil, ErrNegativeStrLen
	}

	// consume ':'
	d.pos++

	if int64(d.pos)+length > int64(len(d.data)) {
		return nil, ErrUnexpectedEOF
	}

	result := make([]byte, length)
	copy(result, d.data[d.pos:d.pos+int(length)])
	d.pos += int(length)

	return result, nil
}

func (d *decoder) decodeList() ([]interface{}, error) {
	// consume 'l'
	if _, err := d.readByte(); err != nil {
		return nil, err
	}

	var list []interface{}
	for {
		b, err := d.peek()
		if err != nil {
			return nil, ErrUnexpectedEOF
		}
		if b == 'e' {
			d.pos++
			return list, nil
		}
		val, err := d.decodeValue()
		if err != nil {
			return nil, err
		}
		list = append(list, val)
	}
}

func (d *decoder) decodeListRaw() ([]interface{}, error) {
	// consume 'l'
	if _, err := d.readByte(); err != nil {
		return nil, err
	}

	var list []interface{}
	for {
		b, err := d.peek()
		if err != nil {
			return nil, ErrUnexpectedEOF
		}
		if b == 'e' {
			d.pos++
			return list, nil
		}
		val, err := d.decodeValueRaw()
		if err != nil {
			return nil, err
		}
		list = append(list, val)
	}
}

func (d *decoder) decodeDict() (map[string]interface{}, error) {
	// consume 'd'
	if _, err := d.readByte(); err != nil {
		return nil, err
	}

	dict := make(map[string]interface{})
	for {
		b, err := d.peek()
		if err != nil {
			return nil, ErrUnexpectedEOF
		}
		if b == 'e' {
			d.pos++
			return dict, nil
		}

		key, err := d.decodeString()
		if err != nil {
			return nil, fmt.Errorf("bencode: error decoding dict key: %w", err)
		}

		val, err := d.decodeValue()
		if err != nil {
			return nil, fmt.Errorf("bencode: error decoding dict value for key %q: %w", key, err)
		}

		dict[key] = val
	}
}

func (d *decoder) decodeDictRaw() (map[string]interface{}, error) {
	// consume 'd'
	if _, err := d.readByte(); err != nil {
		return nil, err
	}

	dict := make(map[string]interface{})
	for {
		b, err := d.peek()
		if err != nil {
			return nil, ErrUnexpectedEOF
		}
		if b == 'e' {
			d.pos++
			return dict, nil
		}

		// Keys are always strings
		keyBytes, err := d.decodeStringBytes()
		if err != nil {
			return nil, fmt.Errorf("bencode: error decoding dict key: %w", err)
		}
		key := string(keyBytes)

		val, err := d.decodeValueRaw()
		if err != nil {
			return nil, fmt.Errorf("bencode: error decoding dict value for key %q: %w", key, err)
		}

		dict[key] = val
	}
}

// DecodeAll decodes all bencoded values from a reader.
func DecodeAll(r io.Reader) ([]interface{}, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("bencode: read error: %w", err)
	}

	d := &decoder{data: data}
	var results []interface{}
	for d.pos < len(d.data) {
		val, err := d.decodeValue()
		if err != nil {
			return nil, err
		}
		results = append(results, val)
	}
	return results, nil
}

// DecodeFrom decodes a bencoded value from a byte buffer, returning the value
// and the number of bytes consumed.
func DecodeFrom(data []byte) (interface{}, int, error) {
	d := &decoder{data: data}
	val, err := d.decodeValue()
	if err != nil {
		return nil, 0, err
	}
	return val, d.pos, nil
}

// DecodeReader is a convenience alias that decodes from a bytes.Reader.
func DecodeReader(data []byte) (interface{}, error) {
	return Decode(bytes.NewReader(data))
}
