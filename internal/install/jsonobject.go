package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// object is a JSON object that keeps its keys in the order read or set, so that pm rewrites a settings file a user
// edits (.claude/settings.json, .codex/hooks.json, .claude/settings.local.json) with every key and value it does not
// touch in its place.
type object struct {
	Keys   []string
	values map[string]any
}

func newObject() *object { return &object{values: map[string]any{}} }

// Get is the key's value, nil when it is absent (as for JSON null).
func (o *object) Get(key string) any { return o.values[key] }

// Has is whether the object holds the key.
func (o *object) Has(key string) bool {
	_, ok := o.values[key]
	return ok
}

// Set puts a new key last and keeps an existing one in its place.
func (o *object) Set(key string, value any) {
	if !o.Has(key) {
		o.Keys = append(o.Keys, key)
	}
	o.values[key] = value
}

// Delete takes the key out; an absent key is left alone.
func (o *object) Delete(key string) {
	if !o.Has(key) {
		return
	}
	delete(o.values, key)
	for i, k := range o.Keys {
		if k == key {
			o.Keys = append(o.Keys[:i:i], o.Keys[i+1:]...)
			break
		}
	}
}

// MarshalJSON writes the keys in order.
func (o *object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.Keys {
		if i > 0 {
			b.WriteByte(',')
		}
		key, err := marshal(k)
		if err != nil {
			return nil, err
		}
		value, err := marshal(o.values[k])
		if err != nil {
			return nil, err
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(value)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// marshal is v as compact JSON with <, > and & kept as they are.
func marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// parseJSON reads one JSON document as nil, bool, json.Number (its text kept), string, []any or *object; trailing data
// is an error. A repeated key keeps its first place and its last value.
func parseJSON(text string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("extra data after the JSON document")
	}
	return v, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch tok {
	case json.Delim('{'):
		o := newObject()
		for dec.More() {
			k, err := dec.Token()
			if err != nil {
				return nil, err
			}
			v, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			key, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("an object key that is a %T", k) // the decoder hands keys as strings
			}
			o.Set(key, v)
		}
		_, err := dec.Token()
		return o, err
	case json.Delim('['):
		list := []any{}
		for dec.More() {
			v, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
		_, err := dec.Token()
		return list, err
	}
	return tok, nil
}

// DumpJSON is how pm writes a JSON settings file: two-space indent, one key per line, non-ASCII and <, > and & kept as
// they are, and a final newline.
func DumpJSON(data any) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		panic(err) // data is what parseJSON read or pm built: always encodable
	}
	return b.String()
}

// canonical is v as JSON with every object's keys sorted, so that two values compare equal however their keys are
// ordered.
func canonical(v any) string {
	b, err := marshal(plain(v))
	if err != nil {
		panic(err) // as for DumpJSON
	}
	return string(b)
}

// plain is v with each *object a map, which encoding/json writes with its keys sorted.
func plain(v any) any {
	switch t := v.(type) {
	case *object:
		m := make(map[string]any, len(t.Keys))
		for _, k := range t.Keys {
			m[k] = plain(t.values[k])
		}
		return m
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = plain(e)
		}
		return out
	}
	return v
}
