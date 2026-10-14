package parse

import (
	"errors"
	"io"
	"reflect"
	"testing"
)

func TestArgumentChecks(t *testing.T) {
	for _, tt := range []struct {
		name   string
		args   []string
		values map[string][]string
		err    string
		help   bool
	}{
		{name: "values", args: []string{"--number", " +01 ", "--pick", "left", "--pair", "label", "text", "-3", "tail"}, values: map[string][]string{"number": {"1"}, "pick": {"left"}, "pair": {"label\x00text"}, "first": {"-3"}, "rest": {"tail"}}},
		{name: "abbreviation", args: []string{"--num=2", "--pick", "right", "x"}, err: "unrecognized arguments: --num=2"},
		{name: "repeat", args: []string{"--pick", "left", "--pair", "a", "b", "--pair", "c", "d", "x"}, values: map[string][]string{"pick": {"left"}, "pair": {"a\x00b", "c\x00d"}, "first": {"x"}, "rest": nil}},
		{name: "group conflict", args: []string{"--pick", "left", "--other", "x"}, err: "argument --other: not allowed with argument --pick"},
		{name: "required positional", args: []string{"--pick", "left"}, err: "the following arguments are required: first"},
		{name: "required group", args: []string{"x"}, err: "one of the arguments --pick --other is required"},
		{name: "invalid integer", args: []string{"--number", "no", "--pick", "left", "x"}, err: "argument --number: invalid int value: 'no'"},
		{name: "invalid choice", args: []string{"--pick", "up", "x"}, err: "argument --pick: invalid choice: 'up' (choose from 'left', 'right')"},
		{name: "missing pair", args: []string{"--pair", "a"}, err: "argument --pair: expected 2 arguments"},
		{name: "missing flag value", args: []string{"--number"}, err: "argument --number: expected one argument"},
		{name: "unknown", args: []string{"--unknown", "--pick", "left", "x"}, err: "unrecognized arguments: --unknown"},
		{name: "prefix of two", args: []string{"--p", "x"}, err: "unrecognized arguments: --p"},
		{name: "unknown before required", args: []string{"--pik", "left"}, err: "unrecognized arguments: --pik"},
		{name: "help", args: []string{"--help"}, help: true},
		{name: "option terminator", args: []string{"--pick", "left", "--", "--number"}, values: map[string][]string{"pick": {"left"}, "first": {"--number"}, "rest": nil}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := &Command[bool]{Name: "pm", Groups: []bool{true}, Args: []Arg{
				{Flags: []string{"--number"}, Dest: "number", IsInt: true},
				{Flags: []string{"--pick"}, Dest: "pick", Choices: []string{"left", "right"}, Group: 1},
				{Flags: []string{"--other"}, Dest: "other", Kind: SwitchOn, Group: 1},
				{Flags: []string{"--pair"}, Dest: "pair", Arity: Two, Kind: Repeated},
				{Dest: "first"}, {Dest: "rest", Arity: Many},
			}}
			var parsed *Parsed[bool]
			root := Build(c, func(p *Parsed[bool]) error { parsed = p; return nil }, nil, io.Discard)
			root.SetArgs(tt.args)
			err := root.Execute()
			if tt.err != "" {
				var ue *UsageError[bool]
				if !errors.As(err, &ue) || ue.Msg != tt.err {
					t.Fatalf("want %q, got %v", tt.err, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.help {
				if parsed != nil {
					t.Fatal("help dispatched command")
				}
				return
			}
			if parsed == nil || !reflect.DeepEqual(parsed.Values, tt.values) {
				t.Fatalf("want %#v, got %#v", tt.values, parsed)
			}
		})
	}
}
