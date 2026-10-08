package pyjson

import "testing"

// Expected values are Python 3's output for the same input.
func TestStringEscapesAsJSONDumps(t *testing.T) {
	in := "a\"b\\c\n\t\x01\x7fé—\U0001F600"
	for _, c := range []struct {
		ascii bool
		want  string
	}{
		{true, `"a\"b\\c\n\t\u0001\u007f\u00e9\u2014\ud83d\ude00"`}, // json.dumps(s)
		{false, "\"a\\\"b\\\\c\\n\\t\\u0001\x7fé—\U0001F600\""},     // json.dumps(s, ensure_ascii=False)
	} {
		if got := String(in, c.ascii); got != c.want {
			t.Errorf("ascii=%v: got %s, want %s", c.ascii, got, c.want)
		}
	}
}

func TestDumpsKeepsKeyOrderAndPythonSeparators(t *testing.T) {
	v, err := Loads(`{"z": [1, 2.50, true, null], "a": {"k": "v"}, "z2": 1e2}`)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"z": [1, 2.5, true, null], "a": {"k": "v"}, "z2": 100.0}`
	if got := Dumps(v, true); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestFloatRepr(t *testing.T) {
	for f, want := range map[float64]string{20: "20.0", 19.5: "19.5", 1e16: "1e+16", 1e-5: "1e-05", 0.0001: "0.0001",
		123456789012345.6: "123456789012345.6"} {
		if got := FloatRepr(f); got != want {
			t.Errorf("repr(%v): got %s, want %s", f, got, want)
		}
	}
}

func TestStrReprQuotesAsPython(t *testing.T) {
	for in, want := range map[string]string{"bd": "'bd'", "it's": `"it's"`, `a'b"c`: `'a\'b"c'`, "x\ny": `'x\ny'`,
		"a\u00a0b\u00adc\u00e9": `'a\xa0b\xadcé'`, "\u2028": `'\u2028'`} {
		if got := StrRepr(in); got != want {
			t.Errorf("repr(%q): got %s, want %s", in, got, want)
		}
	}
}
