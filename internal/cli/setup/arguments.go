package setup

// Arguments is the parsed input the setup commands consume.
type Arguments interface {
	Get(string) string
	ValuesOf(string) []string
}

// ExitError is a hook failure already printed to stderr.
type ExitError struct{ Code int }

func (e ExitError) Error() string { return "post-checkout setup failed" }
func (e ExitError) ExitCode() int { return e.Code }
