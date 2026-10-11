package pyjson

import "testing"

func TestStrReprQuotesAsPython(t *testing.T) {
	for in, want := range map[string]string{"bd": "'bd'", "it's": `"it's"`, `a'b"c`: `'a\'b"c'`, "x\ny": `'x\ny'`,
		"a\u00a0b\u00adc\u00e9": `'a\xa0b\xadcé'`, "\u2028": `'\u2028'`} {
		if got := StrRepr(in); got != want {
			t.Errorf("repr(%q): got %s, want %s", in, got, want)
		}
	}
}
