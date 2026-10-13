package install

import (
	"testing"

	"github.com/BurntSushi/toml"
)

// A settings file laid out as pm writes JSON comes back byte for byte: keys in the file's order, numbers as written,
// non-ASCII and <, > and & as they are, empty containers inline.
func TestDumpJSONKeepsASettingsFileByteForByte(t *testing.T) {
	text := "{\n" +
		"  \"zeta\": 1.50,\n" +
		"  \"alpha\": {\n" +
		"    \"b\": [],\n" +
		"    \"a\": {},\n" +
		"    \"n\": [\n" +
		"      -0,\n" +
		"      1e2,\n" +
		"      true,\n" +
		"      null\n" +
		"    ]\n" +
		"  },\n" +
		"  \"model\": \"café <a&b> 😀 \\\"q\\\" \\\\ \\n\\t\\u0001\",\n" +
		"  \"last\": 2\n" +
		"}\n"
	v, err := parseJSON(text)
	if err != nil {
		t.Fatal(err)
	}
	if got := DumpJSON(v); got != text {
		t.Errorf("DumpJSON(parseJSON(text)) =\n%s\nwant\n%s", got, text)
	}
	o, ok := v.(*object)
	if !ok {
		t.Fatalf("parseJSON gave a %T", v)
	}
	o.Set("alpha", "x")
	o.Set("new", "y")
	o.Delete("zeta")
	if got, want := DumpJSON(o), "{\n  \"alpha\": \"x\",\n  \"model\": \"café <a&b> 😀 \\\"q\\\" \\\\ \\n\\t\\u0001\",\n"+
		"  \"last\": 2,\n  \"new\": \"y\"\n}\n"; got != want {
		t.Errorf("after Set and Delete:\n%s\nwant\n%s", got, want)
	}
	if _, err := parseJSON(`{"a": 1} x`); err == nil {
		t.Error("trailing data parsed")
	}
	if got, want := canonical(o), `{"alpha":"x","last":2,"model":"café <a&b> 😀 \"q\" \\ \n\t\u0001","new":"y"}`; got != want {
		t.Errorf("canonical = %s, want %s", got, want)
	}
}

// tomlString's output is a TOML basic string that reads back as the string, for every character TOML escapes or
// refuses raw.
func TestTOMLStringReadsBack(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"main", `"main"`},
		{`a"b\c`, `"a\"b\\c"`},
		{"\b\t\n\f\r", `"\b\t\n\f\r"`},
		{"\x00\x01\x07\x0b\x1b\x1f\x7f", `"\u0000\u0001\u0007\u000b\u001b\u001f\u007f"`},
		{"café 😀 \u2028", "\"café 😀 \u2028\""},
	} {
		got := tomlString(c.in)
		if got != c.want {
			t.Errorf("tomlString(%q) = %s, want %s", c.in, got, c.want)
		}
		var back struct{ K string }
		if _, err := toml.Decode("K = "+got, &back); err != nil || back.K != c.in {
			t.Errorf("tomlString(%q) = %s reads back as %q (%v)", c.in, got, back.K, err)
		}
	}
}
