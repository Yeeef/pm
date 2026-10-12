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

// GHJSON runs gh in dir and decodes the JSON it prints into v; a gh that fails, runs past GHTimeout or prints no JSON
// is an error naming why.
func GHJSON(dir string, v any, args ...string) error {
	cmd := "gh " + strings.Join(args, " ")
	res, err := Run(append([]string{"gh"}, args...), Options{Cwd: &dir, Timeout: GHTimeout,
		TimeoutText: strconv.Itoa(int(GHTimeout.Seconds()))})
	switch {
	case err != nil:
		return fmt.Errorf("%s did not run: %v", cmd, err)
	case res.Code != 0:
		why := strings.TrimSpace(res.Stderr)
		if why == "" {
			why = fmt.Sprintf("exit %d", res.Code)
		}
		return fmt.Errorf("%s failed: %s", cmd, why)
	}
	if err := json.Unmarshal([]byte(res.Stdout), v); err != nil {
		return fmt.Errorf("%s printed no JSON: %v", cmd, err)
	}
	return nil
}
