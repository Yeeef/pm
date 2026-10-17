package needform

import (
	"reflect"
	"testing"

	"github.com/Yeeef/pm/internal/cli/parse"
)

// A refused option label is one broken rule: its --cost, a --default naming it and the count of options add none.
func TestARefusedLabelIsOneRule(t *testing.T) {
	pair := func(a, b string) string { return a + parse.PairSep + b }
	base := map[string][]string{"question": {"Which?"}, "fact": {"A fact."}}
	cases := map[string]struct {
		option, cost, def []string
		want              string
	}{
		"not letters and digits": {
			[]string{pair("a)", "A."), pair("b", "B.")}, []string{pair("a)", "c."), pair("b", "c.")},
			[]string{pair("b", "why.")}, `the option label "a)" is not letters and digits only, e.g. a, b or keep`,
		},
		"duplicate": {
			[]string{pair("a", "A."), pair("a", "B.")}, []string{pair("a", "c."), pair("a", "d.")},
			[]string{pair("a", "why.")}, "two options have the label a; give each option its own label",
		},
	}
	for name, c := range cases {
		values := map[string][]string{"option": c.option, "cost": c.cost, "default": c.def}
		for k, val := range base {
			values[k] = val
		}
		var v Violations
		if Markdown(values, &v) != "" || !reflect.DeepEqual(v.List, []string{c.want}) {
			t.Errorf("%s: got %q, want only %q", name, v.List, c.want)
		}
	}
}
