package cli

import (
	"fmt"

	"github.com/Yeeef/yeeef-agents/pm/internal/config"
	"github.com/Yeeef/yeeef-agents/pm/internal/service"
)

// showPort is Python's port(): $PORT for one run, else the installed service's port, else the config's.
func showPort(here, main string) (int, error) {
	cfg, err := config.Load(here)
	if err != nil {
		return 0, err
	}
	return service.PortFor(main, int(cfg.Port))
}

// showSiteURL is Python's site_url(): the repo's site_url, else http://localhost:<port>.
func showSiteURL(here, main string) (string, error) {
	cfg, err := config.Load(here)
	if err != nil {
		return "", err
	}
	if cfg.SiteURL != "" {
		return cfg.SiteURL, nil
	}
	port, err := service.PortFor(main, int(cfg.Port))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://localhost:%d", port), nil
}
