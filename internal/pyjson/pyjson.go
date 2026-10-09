// Package pyjson reads JSON as Python's json.loads does and writes it as json.dumps does with its default separators
// (", " and ": "), so pm's JSON output is byte-identical to Python pm's. Objects keep their keys in input order, as
// a Python dict does.
package pyjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
)

// Object is a JSON object with its keys in input order.
type Object struct {
	Keys   []string
	Values map[string]any
}

// Get is dict.get(key): the value, or nil when the key is absent (as for JSON null).
func (o *Object) Get(key string) any { return o.Values[key] }

// Loads parses one JSON document: nil, bool, json.Number, string, []any or *Object. Trailing data is an error.
func Loads(text string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	v, err := decode(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("extra data after the JSON document")
	}
	return v, nil
}

func decode(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			o := &Object{Values: map[string]any{}}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k := kt.(string)
				v, err := decode(dec)
				if err != nil {
					return nil, err
				}
				if _, seen := o.Values[k]; !seen {
					o.Keys = append(o.Keys, k) // a repeated key keeps its first place and its last value, as in a dict
				}
				o.Values[k] = v
			}
			_, err := dec.Token()
			return o, err
		case '[':
			list := []any{}
			for dec.More() {
				v, err := decode(dec)
				if err != nil {
					return nil, err
				}
				list = append(list, v)
			}
			_, err := dec.Token()
			return list, err
		}
		return nil, fmt.Errorf("unexpected %v", t)
	default:
		return t, nil
	}
}

// Dumps is json.dumps(v, ensure_ascii=ascii).
func Dumps(v any, ascii bool) string {
	var b bytes.Buffer
	write(&b, v, ascii)
	return b.String()
}

func write(b *bytes.Buffer, v any, ascii bool) {
	switch t := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if t {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case json.Number:
		b.WriteString(Number(t))
	case int:
		b.WriteString(strconv.Itoa(t))
	case string:
		b.WriteString(String(t, ascii))
	case []any:
		b.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				b.WriteString(", ")
			}
			write(b, e, ascii)
		}
		b.WriteByte(']')
	case *Object:
		b.WriteByte('{')
		for i, k := range t.Keys {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(String(k, ascii))
			b.WriteString(": ")
			write(b, t.Values[k], ascii)
		}
		b.WriteByte('}')
	case [][2]any: // an ordered object built in Go: pairs of key and value
		b.WriteByte('{')
		for i, kv := range t {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(String(kv[0].(string), ascii))
			b.WriteString(": ")
			write(b, kv[1], ascii)
		}
		b.WriteByte('}')
	default:
		panic(fmt.Sprintf("pyjson: cannot write a %T", v))
	}
}

// DumpsIndent is json.dumps(v, indent=indent, ensure_ascii=ascii): one item a line, nested by indent spaces, "," at
// each line's end and ": " after a key; an empty list or object stays [] or {}.
func DumpsIndent(v any, indent int, ascii bool) string {
	var b bytes.Buffer
	writeIndent(&b, v, strings.Repeat(" ", indent), "\n", ascii)
	return b.String()
}

func writeIndent(b *bytes.Buffer, v any, step, nl string, ascii bool) {
	inner := nl + step
	switch t := v.(type) {
	case []any:
		if len(t) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(inner)
			writeIndent(b, e, step, inner, ascii)
		}
		b.WriteString(nl)
		b.WriteByte(']')
	case *Object:
		if len(t.Keys) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteByte('{')
		for i, k := range t.Keys {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(inner)
			b.WriteString(String(k, ascii))
			b.WriteString(": ")
			writeIndent(b, t.Values[k], step, inner, ascii)
		}
		b.WriteString(nl)
		b.WriteByte('}')
	default:
		write(b, v, ascii)
	}
}

// NewObject is an empty object.
func NewObject() *Object { return &Object{Values: map[string]any{}} }

// Has is `key in d`.
func (o *Object) Has(key string) bool {
	_, ok := o.Values[key]
	return ok
}

// Set is d[key] = value: a new key goes last, an existing one keeps its place.
func (o *Object) Set(key string, value any) {
	if !o.Has(key) {
		o.Keys = append(o.Keys, key)
	}
	o.Values[key] = value
}

// Delete is del d[key]; an absent key is left alone.
func (o *Object) Delete(key string) {
	if !o.Has(key) {
		return
	}
	delete(o.Values, key)
	for i, k := range o.Keys {
		if k == key {
			o.Keys = append(o.Keys[:i:i], o.Keys[i+1:]...)
			break
		}
	}
}

// Number is how json.dumps writes a number json.loads read from this text: an int keeps its digits, a float is
// Python's repr of it.
func Number(n json.Number) string {
	s := string(n)
	if !strings.ContainsAny(s, ".eE") {
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return strconv.FormatInt(i, 10) // -0 is 0, as Python's int
		}
		return s
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil { // out of range: Python reads inf
		if strings.HasPrefix(s, "-") {
			return "-Infinity"
		}
		return "Infinity"
	}
	return FloatRepr(f)
}

// FloatRepr is Python's repr() of a float: the shortest digits that round-trip, in exponent form below 1e-4 and from
// 1e16 up.
func FloatRepr(f float64) string {
	e := strconv.FormatFloat(f, 'e', -1, 64) // d.ddde±XX
	mant, exp, _ := strings.Cut(e, "e")
	x, _ := strconv.Atoi(exp)
	if x < -4 || x >= 16 {
		sign := "+"
		if x < 0 {
			sign, x = "-", -x
		}
		return fmt.Sprintf("%se%s%02d", mant, sign, x)
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// String is json.dumps(s, ensure_ascii=ascii) for a string.
func String(s string, ascii bool) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			switch {
			case r < 0x20:
				fmt.Fprintf(&b, `\u%04x`, r)
			case ascii && r > 0x7e:
				if r > 0xffff {
					r -= 0x10000
					fmt.Fprintf(&b, `\u%04x\u%04x`, 0xd800+(r>>10), 0xdc00+(r&0x3ff))
				} else {
					fmt.Fprintf(&b, `\u%04x`, r)
				}
			default:
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Truthy is Python's bool() of a value json.loads gave.
func Truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case json.Number:
		f, err := strconv.ParseFloat(string(t), 64)
		return err != nil || f != 0
	case string:
		return t != ""
	case []any:
		return len(t) > 0
	case *Object:
		return len(t.Keys) > 0
	}
	return true
}

// Str is Python's str() of a value json.loads gave.
func Str(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case bool:
		if t {
			return "True"
		}
		return "False"
	case json.Number:
		return Number(t)
	case string:
		return t
	}
	return Repr(v)
}

// Repr is Python's repr() of a value json.loads gave.
func Repr(v any) string {
	switch t := v.(type) {
	case string:
		return StrRepr(t)
	case []any:
		parts := make([]string, len(t))
		for i, e := range t {
			parts[i] = Repr(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *Object:
		parts := make([]string, len(t.Keys))
		for i, k := range t.Keys {
			parts[i] = StrRepr(k) + ": " + Repr(t.Values[k])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return Str(v)
}

// TypeName is the Python type name of a value json.loads gave.
func TypeName(v any) string {
	switch t := v.(type) {
	case nil:
		return "NoneType"
	case bool:
		return "bool"
	case json.Number:
		if strings.ContainsAny(string(t), ".eE") {
			return "float"
		}
		return "int"
	case string:
		return "str"
	case []any:
		return "list"
	case *Object:
		return "dict"
	}
	return fmt.Sprintf("%T", v)
}

// pyPrintable is Python's str.isprintable() for one character: not a control, format, surrogate, private-use,
// unassigned or separator character, the space excepted.
func pyPrintable(r rune) bool {
	if r == ' ' {
		return true
	}
	if unicode.In(r, unicode.Cc, unicode.Cf, unicode.Cs, unicode.Co, unicode.Zl, unicode.Zp, unicode.Zs) {
		return false
	}
	return unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S)
}

// StrRepr is Python's repr() of a str.
func StrRepr(s string) string {
	quote := '\''
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		quote = '"'
	}
	var b strings.Builder
	b.WriteRune(quote)
	for _, r := range s {
		switch {
		case r == quote || r == '\\':
			b.WriteRune('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || (r >= 0x7f && r <= 0xff && !pyPrintable(r)):
			fmt.Fprintf(&b, `\x%02x`, r)
		case r > 0xff && !pyPrintable(r):
			if r > 0xffff {
				fmt.Fprintf(&b, `\U%08x`, r)
			} else {
				fmt.Fprintf(&b, `\u%04x`, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	b.WriteRune(quote)
	return b.String()
}
