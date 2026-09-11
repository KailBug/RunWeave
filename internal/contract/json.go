// Package contract defines the versioned, transport-independent business contract.
package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// strictDecode additionally rejects ambiguities accepted by encoding/json v1.
// optional tags describe wire presence; omitempty only controls encoding.
func strictDecode(data []byte, dst any) error {
	if !utf8.Valid(data) || !json.Valid(data) {
		return fmt.Errorf("invalid JSON or UTF-8")
	}
	if err := checkSurrogates(data); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	value, err := readValue(d, 0)
	if err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("expected one JSON value")
	}
	if err := checkShape(value, reflect.TypeOf(dst).Elem()); err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

func readValue(d *json.Decoder, depth int) (any, error) {
	if depth > 32 {
		return nil, fmt.Errorf("JSON nesting exceeds 32")
	}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	if token == nil {
		return nil, fmt.Errorf("null is not allowed")
	}
	switch token {
	case json.Delim('{'):
		object := map[string]any{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("expected object key")
			}
			if _, exists := object[name]; exists {
				return nil, fmt.Errorf("duplicate field")
			}
			value, err := readValue(d, depth+1)
			if err != nil {
				return nil, err
			}
			object[name] = value
		}
		_, err := d.Token()
		return object, err
	case json.Delim('['):
		array := []any{}
		for d.More() {
			value, err := readValue(d, depth+1)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		_, err := d.Token()
		return array, err
	default:
		return token, nil
	}
}

func checkShape(value any, typ reflect.Type) error {
	if typ.Kind() == reflect.Pointer {
		return checkShape(value, typ.Elem())
	}
	switch typ.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object")
		}
		fields := make(map[string]reflect.StructField, typ.NumField())
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			fields[name] = field
			if _, exists := object[name]; !exists && field.Tag.Get("optional") != "true" {
				return fmt.Errorf("missing field %s", name)
			}
		}
		for name, item := range object {
			field, ok := fields[name]
			if !ok {
				return fmt.Errorf("unknown field")
			}
			if err := checkShape(item, field.Type); err != nil {
				return err
			}
		}
	case reflect.Slice:
		array, ok := value.([]any)
		if !ok {
			return fmt.Errorf("expected array")
		}
		for _, item := range array {
			if err := checkShape(item, typ.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkSurrogates(data []byte) error {
	// json.Valid has already checked escape syntax. Skip escaped backslashes,
	// and require every UTF-16 surrogate escape to belong to a valid pair.
	for i := 0; i < len(data); i++ {
		if data[i] != '\\' {
			continue
		}
		i++
		if data[i] != 'u' {
			continue
		}
		value, _ := strconv.ParseUint(string(data[i+1:i+5]), 16, 16)
		i += 4
		if value >= 0xdc00 && value <= 0xdfff {
			return fmt.Errorf("unpaired surrogate")
		}
		if value < 0xd800 || value > 0xdbff {
			continue
		}
		if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
			return fmt.Errorf("unpaired surrogate")
		}
		low, err := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
		if err != nil || low < 0xdc00 || low > 0xdfff {
			return fmt.Errorf("unpaired surrogate")
		}
		i += 6
	}
	return nil
}

// Marshal would silently replace invalid UTF-8 in Go strings. Reject it before
// encoding values constructed by callers, not only values decoded from JSON.
func validGoStrings(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Pointer:
		return value.IsNil() || validGoStrings(value.Elem())
	case reflect.String:
		return utf8.ValidString(value.String())
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if !validGoStrings(value.Field(i)) {
				return false
			}
		}
	case reflect.Slice:
		for i := 0; i < value.Len(); i++ {
			if !validGoStrings(value.Index(i)) {
				return false
			}
		}
	}
	return true
}
