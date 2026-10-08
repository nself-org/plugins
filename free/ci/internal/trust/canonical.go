package trust

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// Canonical encodes JSON data with sorted object keys and integer numbers.
func Canonical(v any) ([]byte, error) {
	var b bytes.Buffer
	if err := canonicalValue(&b, reflect.ValueOf(v)); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
func writeString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				_, _ = fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}
func canonicalValue(b *bytes.Buffer, v reflect.Value) error {
	if !v.IsValid() {
		b.WriteString("null")
		return nil
	}
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			b.WriteString("null")
			return nil
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case reflect.String:
		if v.Type() == reflect.TypeOf(json.Number("")) {
			s := v.String()
			if strings.ContainsAny(s, ".eE") || !json.Valid([]byte(s)) {
				return fmt.Errorf("float or invalid number is not canonical JSON")
			}
			b.WriteString(s)
		} else {
			writeString(b, v.String())
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b.WriteString(strconv.FormatInt(v.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		b.WriteString(strconv.FormatUint(v.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		return fmt.Errorf("float is not canonical JSON")
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("object keys must be strings")
		}
		keys := v.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			writeString(b, k.String())
			b.WriteByte(':')
			if err := canonicalValue(b, v.MapIndex(k)); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 && v.Kind() == reflect.Slice {
			writeString(b, base64.StdEncoding.EncodeToString(v.Bytes()))
			return nil
		}
		b.WriteByte('[')
		for i := 0; i < v.Len(); i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := canonicalValue(b, v.Index(i)); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case reflect.Struct:
		type field struct {
			name  string
			value reflect.Value
		}
		fields := []field{}
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if !f.IsExported() {
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			fields = append(fields, field{name, v.Field(i)})
		}
		sort.Slice(fields, func(i, j int) bool { return fields[i].name < fields[j].name })
		b.WriteByte('{')
		for i, f := range fields {
			if i > 0 {
				b.WriteByte(',')
			}
			writeString(b, f.name)
			b.WriteByte(':')
			if err := canonicalValue(b, f.value); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	default:
		return fmt.Errorf("unsupported JSON type %s", v.Kind())
	}
	return nil
}
