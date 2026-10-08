package records

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/Yeeef/yeeef-agents/pm/internal/pyjson"
)

// A header value's Python type, as pyyaml's safe_load (YAML 1.1) gives it. go.yaml.in/yaml/v3 reads YAML 1.2, where
// yes, on, 0755 and 1:20 are strings; a record header is read with the 1.1 rules, so `title: yes` is True as in Python.
const (
	KindStr      = "str"
	KindInt      = "int"
	KindFloat    = "float"
	KindBool     = "bool"
	KindNull     = "NoneType"
	KindDate     = "date"
	KindDatetime = "datetime"
	KindList     = "list"
	KindDict     = "dict"
)

// Value is one header value: its Python type, its str() and its truth.
type Value struct {
	Kind   string
	Str    string
	Truthy bool
	repr   string // repr() where it differs from str()
}

// Repr is Python's repr() of the value.
func (v Value) Repr() string {
	switch {
	case v.Kind == KindStr:
		return pyjson.StrRepr(v.Str)
	case v.repr != "":
		return v.repr
	}
	return v.Str
}

// Meta is a record's header by key.
type Meta map[string]Value

func parseHeader(text, rel string) (Meta, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, errorf("%s: the YAML header does not parse: %v", rel, err)
	}
	meta := Meta{}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return meta, nil // an empty header: safe_load gives None, read as {}
	}
	root := doc.Content[0]
	if root.Kind == yaml.ScalarNode {
		if v, err := scalar(root); err == nil && v.Kind == KindNull {
			return meta, nil
		}
	}
	if root.Kind != yaml.MappingNode {
		return nil, errorf("%s: the YAML header is not a mapping of fields", rel)
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, err := value(root.Content[i])
		if err != nil {
			return nil, errorf("%s: %v", rel, err)
		}
		v, err := value(root.Content[i+1])
		if err != nil {
			return nil, errorf("%s: %v", rel, err)
		}
		meta[k.Str] = v
	}
	return meta, nil
}

func value(n *yaml.Node) (Value, error) {
	switch n.Kind {
	case yaml.ScalarNode:
		return scalar(n)
	case yaml.AliasNode:
		return value(n.Alias)
	case yaml.SequenceNode:
		parts := make([]string, len(n.Content))
		for i, c := range n.Content {
			v, err := value(c)
			if err != nil {
				return Value{}, err
			}
			parts[i] = v.Repr()
		}
		s := "[" + strings.Join(parts, ", ") + "]"
		return Value{Kind: KindList, Str: s, Truthy: len(parts) > 0}, nil
	case yaml.MappingNode:
		parts := make([]string, 0, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, err := value(n.Content[i])
			if err != nil {
				return Value{}, err
			}
			v, err := value(n.Content[i+1])
			if err != nil {
				return Value{}, err
			}
			parts = append(parts, k.Repr()+": "+v.Repr())
		}
		s := "{" + strings.Join(parts, ", ") + "}"
		return Value{Kind: KindDict, Str: s, Truthy: len(parts) > 0}, nil
	}
	return Value{}, fmt.Errorf("unsupported YAML node in the header")
}

func str(s string) Value { return Value{Kind: KindStr, Str: s, Truthy: s != ""} }

// scalar resolves a scalar as pyyaml's SafeLoader does: a quoted or block scalar, or one tagged !!str, is a string; a
// plain one is typed by YAML 1.1's implicit resolvers.
func scalar(n *yaml.Node) (Value, error) {
	if n.Style&yaml.TaggedStyle != 0 && n.Tag != "!!str" {
		return Value{}, fmt.Errorf("the header value %s has the tag %s, which pm does not read", pyjson.StrRepr(n.Value), n.Tag)
	}
	if n.Style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle|yaml.TaggedStyle) != 0 {
		return str(n.Value), nil
	}
	if strings.ContainsRune(n.Value, '\t') {
		return Value{}, fmt.Errorf("the header value %s holds a tab, which pyyaml refuses", pyjson.StrRepr(n.Value))
	}
	return Resolve(n.Value)
}

// The implicit resolvers of pyyaml's SafeLoader (resolver.py), YAML 1.1.
var (
	boolRE  = regexp.MustCompile(`^(?:yes|Yes|YES|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF)$`)
	floatRE = regexp.MustCompile(`^(?:[-+]?(?:[0-9][0-9_]*)\.[0-9_]*(?:[eE][-+][0-9]+)?|\.[0-9][0-9_]*(?:[eE][-+][0-9]+)?` +
		`|[-+]?[0-9][0-9_]*(?::[0-5]?[0-9])+\.[0-9_]*|[-+]?\.(?:inf|Inf|INF)|\.(?:nan|NaN|NAN))$`)
	intRE = regexp.MustCompile(`^(?:[-+]?0b[0-1_]+|[-+]?0[0-7_]+|[-+]?(?:0|[1-9][0-9_]*)|[-+]?0x[0-9a-fA-F_]+` +
		`|[-+]?[1-9][0-9_]*(?::[0-5]?[0-9])+)$`)
	nullRE      = regexp.MustCompile(`^(?:~|null|Null|NULL|)$`)
	timestampRE = regexp.MustCompile(`^(?:[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]` +
		`|[0-9][0-9][0-9][0-9]-[0-9][0-9]?-[0-9][0-9]?(?:[Tt]|[ \t]+)[0-9][0-9]?:[0-9][0-9]:[0-9][0-9](?:\.[0-9]*)?` +
		`(?:[ \t]*(?:Z|[-+][0-9][0-9]?(?::[0-9][0-9])?))?)$`)
	timestampParts = regexp.MustCompile(`^([0-9][0-9][0-9][0-9])-([0-9][0-9]?)-([0-9][0-9]?)` +
		`(?:(?:[Tt]|[ \t]+)([0-9][0-9]?):([0-9][0-9]):([0-9][0-9])(?:\.([0-9]*))?` +
		`(?:[ \t]*(Z|([-+])([0-9][0-9]?)(?::([0-9][0-9]))?))?)?$`)
)

// Resolve is a plain scalar's value under YAML 1.1, as pyyaml's SafeLoader constructs it.
func Resolve(s string) (Value, error) {
	switch {
	case s != "" && strings.ContainsRune("yYnNtTfFoO", rune(s[0])) && boolRE.MatchString(s):
		b := strings.Contains("yes true on", strings.ToLower(s))
		return Value{Kind: KindBool, Str: map[bool]string{true: "True", false: "False"}[b], Truthy: b}, nil
	case s == "=" || s == "<<": // YAML 1.1's value and merge keys, which SafeLoader cannot construct
		return Value{}, fmt.Errorf("the header value %s is a YAML 1.1 key, which pyyaml refuses", pyjson.StrRepr(s))
	case nullRE.MatchString(s):
		return Value{Kind: KindNull, Str: "None"}, nil
	case intRE.MatchString(s):
		return resolveInt(s), nil
	case floatRE.MatchString(s):
		return resolveFloat(s), nil
	case timestampRE.MatchString(s):
		return resolveTimestamp(s)
	}
	return str(s), nil
}

func resolveInt(s string) Value {
	s = strings.ReplaceAll(s, "_", "")
	sign := 1
	if s[0] == '-' || s[0] == '+' {
		if s[0] == '-' {
			sign = -1
		}
		s = s[1:]
	}
	n := new(big.Int)
	switch {
	case s == "0":
	case strings.HasPrefix(s, "0b"):
		n.SetString(s[2:], 2)
	case strings.HasPrefix(s, "0x"):
		n.SetString(s[2:], 16)
	case s[0] == '0':
		n.SetString(s, 8)
	case strings.Contains(s, ":"):
		base := big.NewInt(1)
		parts := strings.Split(s, ":")
		for i := len(parts) - 1; i >= 0; i-- {
			d, _ := new(big.Int).SetString(parts[i], 10)
			n.Add(n, d.Mul(d, base))
			base = new(big.Int).Mul(base, big.NewInt(60))
		}
	default:
		n.SetString(s, 10)
	}
	if sign < 0 {
		n.Neg(n)
	}
	return Value{Kind: KindInt, Str: n.String(), Truthy: n.Sign() != 0}
}

func resolveFloat(s string) Value {
	s = strings.ToLower(strings.ReplaceAll(s, "_", ""))
	sign := 1.0
	if s[0] == '-' || s[0] == '+' {
		if s[0] == '-' {
			sign = -1
		}
		s = s[1:]
	}
	var f float64
	switch {
	case s == ".inf":
		f = math.Inf(1)
	case s == ".nan":
		f = math.NaN()
	case strings.Contains(s, ":"):
		base := 1.0
		parts := strings.Split(s, ":")
		for i := len(parts) - 1; i >= 0; i-- {
			d, _ := strconv.ParseFloat(parts[i], 64)
			f += d * base
			base *= 60
		}
	default:
		f, _ = strconv.ParseFloat(s, 64)
	}
	f *= sign
	repr := pyjson.FloatRepr(f)
	switch {
	case math.IsInf(f, 1):
		repr = "inf"
	case math.IsInf(f, -1):
		repr = "-inf"
	case math.IsNaN(f):
		repr = "nan"
	}
	return Value{Kind: KindFloat, Str: repr, Truthy: f != 0}
}

func resolveTimestamp(s string) (Value, error) {
	m := timestampParts.FindStringSubmatch(s)
	atoi := func(x string) int { n, _ := strconv.Atoi(x); return n }
	y, mo, d := atoi(m[1]), atoi(m[2]), atoi(m[3])
	date := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	if date.Year() != y || int(date.Month()) != mo || date.Day() != d {
		return Value{}, fmt.Errorf("the header value %s is no valid date", pyjson.StrRepr(s))
	}
	if m[4] == "" {
		iso := fmt.Sprintf("%04d-%02d-%02d", y, mo, d)
		return Value{Kind: KindDate, Str: iso, Truthy: true, repr: fmt.Sprintf("datetime.date(%d, %d, %d)", y, mo, d)}, nil
	}
	h, mi, sec := atoi(m[4]), atoi(m[5]), atoi(m[6])
	if h > 23 || mi > 59 || sec > 59 {
		return Value{}, fmt.Errorf("the header value %s is no valid time", pyjson.StrRepr(s))
	}
	out := fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d", y, mo, d, h, mi, sec)
	if frac := m[7]; frac != "" {
		frac = (frac + "000000")[:6]
		if us := atoi(frac); us != 0 {
			out += fmt.Sprintf(".%06d", us)
		}
	}
	switch {
	case m[8] == "Z":
		out += "+00:00"
	case m[8] != "":
		tzh, tzm := atoi(m[10]), atoi(m[11])
		out += fmt.Sprintf("%s%02d:%02d", m[9], tzh, tzm)
	}
	return Value{Kind: KindDatetime, Str: out, Truthy: true}, nil
}

// YAMLStr is a header value as a template writes it: plain when YAML 1.1 reads `k: <s>` back as the same string,
// otherwise double-quoted JSON (Python's yaml_str).
func YAMLStr(s string) string {
	if plainRoundTrips(s) {
		return s
	}
	return pyjson.String(s, false)
}

// pyyaml's reader rejects these characters anywhere in a stream.
var nonPrintable = regexp.MustCompile(`[^\x09\x0A\x0D\x20-\x7E\x{85}\x{A0}-\x{D7FF}\x{E000}-\x{FFFD}\x{10000}-\x{10FFFF}]`)

func plainRoundTrips(s string) bool {
	if nonPrintable.MatchString(s) {
		return false
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte("k: "+s), &doc); err != nil || len(doc.Content) != 1 {
		return false
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode || len(root.Content) != 2 || root.Content[0].Value != "k" {
		return false
	}
	n := root.Content[1]
	if n.Kind != yaml.ScalarNode || n.Style&yaml.TaggedStyle != 0 && n.Tag != "!!str" {
		return false
	}
	v, err := scalar(n)
	return err == nil && v.Kind == KindStr && v.Str == s
}
