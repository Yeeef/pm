package cli

import (
	"fmt"
	"io"

	"github.com/Yeeef/pm/internal/cli/where"
	"github.com/Yeeef/pm/internal/store"
)

// cmdWhere is pm where: with records, the store's path alone, for scripts; without, every location with its state.
func cmdWhere(p *Parsed, here string, stdout io.Writer) error {
	if p.Get("what") == "" {
		out, err := where.All(here)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, out)
		return err
	}
	records, err := store.Find(here)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, records)
	return err
}
