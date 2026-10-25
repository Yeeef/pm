package cli

import (
	"github.com/Yeeef/pm/internal/cli/where"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/service"
	"github.com/Yeeef/pm/internal/store"
)

// showPort is Python's port(): $PORT for one run, else the installed service's port, else the config's.
func showPort(here, main string) (int, error) {
	cfg, err := config.Load(here)
	if err != nil {
		return 0, err
	}
	return service.PortFor(main, int(cfg.Port))
}

// siteURL is the site's URL for this store, as pm show links a record.
func (e *env) siteURL() (string, error) { return where.SiteURL(e.here, store.MainOf(e.records)) }
