package proc

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// GHTimeout bounds one gh call.
const GHTimeout = 60 * time.Second

// GH runs gh in dir and returns what it prints; a gh that fails or runs past GHTimeout is an error naming why.
func GH(dir string, args ...string) (string, error) {
	cmd := "gh " + strings.Join(args, " ")
	res, err := Run(append([]string{"gh"}, args...), Options{Cwd: &dir, Timeout: GHTimeout,
		TimeoutText: strconv.Itoa(int(GHTimeout.Seconds()))})
	switch {
	case err != nil:
		return "", fmt.Errorf("%s did not run: %v", cmd, err)
	case res.Code != 0:
		why := strings.TrimSpace(res.Stderr)
		if why == "" {
			why = fmt.Sprintf("exit %d", res.Code)
		}
		return "", fmt.Errorf("%s failed: %s", cmd, why)
	}
	return res.Stdout, nil
}

// GHJSON runs gh in dir and decodes the JSON it prints into v; a gh that fails, runs past GHTimeout or prints no JSON
// is an error naming why.
func GHJSON(dir string, v any, args ...string) error {
	out, err := GH(dir, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(out), v); err != nil {
		return fmt.Errorf("gh %s printed no JSON: %v", strings.Join(args, " "), err)
	}
	return nil
}
