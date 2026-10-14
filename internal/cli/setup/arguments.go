package setup

import "strconv"

// Arguments is the parsed input the setup commands consume.
type Arguments interface {
	Get(string) string
	ValuesOf(string) []string
}

// ExitError ends a setup command whose output already said why (pm doctor's differences): pm exits with Code and
// prints nothing more.
type ExitError struct{ Code int }

func (e ExitError) Error() string { return "exit " + strconv.Itoa(e.Code) }
