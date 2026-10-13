package install

// The launcher keeps each pin's release binary at <data>/pm/pins/<X>/pm, about 113 MB, and never needs one that no
// checkout pins. A checkout on this machine is a worktree of a clone pm knows: this one, and each clone with a pm
// service unit installed (pm init installs one per clone, whatever pm version set it up). pm upgrade prunes every
// other pin's binary and pm doctor names them. A pruned pin keeps its sha256 record, so a later download of it is
// checked against the first one's. Whatever cannot be read (a unit, a clone's worktree list, a worktree's config)
// prunes nothing: a pin is kept unless pm knows that no checkout pins it.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/launch"
	"github.com/Yeeef/pm/internal/service"
)

// keptRecord is the file a pruned pin keeps: the sha256 of its first download (launch.Download).
const keptRecord = "sha256"

// PrunePins removes the binary of every pin that no checkout on this machine pins, keeping its sha256; pm upgrade's
// lines on what it pruned, or on why it pruned none, and none when there was nothing to prune.
func PrunePins(main string) []string {
	unused, size, err := unusedPins(main)
	if err != nil {
		return []string{fmt.Sprintf("pins: pruned none under %s: %v", launch.Pins(), err)}
	}
	if len(unused) == 0 {
		return nil
	}
	for _, v := range unused {
		if err := prunePin(v); err != nil {
			return []string{fmt.Sprintf("pins: pruning pm %s under %s failed: %v", v, launch.Pins(), err)}
		}
	}
	return []string{fmt.Sprintf("pins: pruned pm %s under %s (%d MB), which no checkout on this machine pins; each "+
		"keeps its %s, so a later download of it is checked against it", strings.Join(unused, ", "), launch.Pins(),
		size/1e6, keptRecord)}
}

// UnusedPins is pm doctor's line on the pins whose binary no checkout on this machine needs; none when there are none.
func UnusedPins(main string) []string {
	unused, size, err := unusedPins(main)
	if err != nil {
		return []string{fmt.Sprintf("pins: cannot tell which pins under %s are unused: %v", launch.Pins(), err)}
	}
	if len(unused) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("pins: no checkout on this machine pins pm %s, whose binaries under %s hold %d MB; "+
		"pm upgrade --to %s prunes them", strings.Join(unused, ", "), launch.Pins(), size/1e6, buildinfo.Version)}
}

// unusedPins is each pin under launch.Pins() that holds more than its sha256 and that no checkout pins, oldest first,
// and the bytes they hold.
func unusedPins(main string) ([]string, int64, error) {
	entries, err := os.ReadDir(launch.Pins())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var keep map[string]bool // read only when there is a pin to prune
	var unused []string
	var size int64
	for _, e := range entries {
		n, err := pinBytes(e.Name())
		if err != nil {
			return nil, 0, err
		}
		if n == 0 {
			continue
		}
		if keep == nil {
			if keep, err = pinsInUse(main); err != nil {
				return nil, 0, err
			}
		}
		if !keep[e.Name()] {
			unused = append(unused, e.Name())
			size += n
		}
	}
	sort.Slice(unused, func(i, j int) bool {
		a, _ := launch.Key(unused[i])
		b, _ := launch.Key(unused[j])
		return launch.Less(a, b)
	})
	return unused, size, nil
}

// pinBytes is the bytes a pin's dir holds besides its sha256; 0 for a pruned pin, or an entry that is no dir.
func pinBytes(version string) (int64, error) {
	dir := launch.PinDir(version)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return 0, err
	}
	var n int64
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || p == filepath.Join(dir, keptRecord) {
			return err
		}
		info, err := d.Info()
		if err == nil {
			n += max(info.Size(), 1) // an empty file is still something to prune
		}
		return err
	})
	return n, err
}

// prunePin removes everything in a pin's dir but its sha256.
func prunePin(version string) error {
	dir := launch.PinDir(version)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() != keptRecord {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// pinsInUse is every version a checkout on this machine pins: each worktree of this clone (main) and of each clone
// with a pm service unit, and the running pm's own. A clone whose main checkout is gone pins nothing.
func pinsInUse(main string) (map[string]bool, error) {
	clones, err := service.Clones()
	if err != nil {
		return nil, fmt.Errorf("cannot read the pm service units, which name the clones on this machine: %w", err)
	}
	keep := map[string]bool{buildinfo.Version: true}
	for _, clone := range append([]string{main}, clones...) {
		if _, err := os.Stat(clone); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		out, err := Git(clone, "worktree", "list", "--porcelain")
		if err != nil {
			return nil, fmt.Errorf("cannot list the worktrees of %s: %w", clone, err)
		}
		for _, line := range strings.Split(out, "\n") {
			wt, ok := strings.CutPrefix(line, "worktree ")
			if !ok {
				continue
			}
			v, err := pinOf(wt)
			if err != nil {
				return nil, err
			}
			if v != "" {
				keep[v] = true
			}
		}
	}
	return keep, nil
}

// pinOf is the version a checkout's .pm/config.toml pins; "" when the checkout or its config is gone.
func pinOf(checkout string) (string, error) {
	path := filepath.Join(checkout, config.Rel)
	data := map[string]any{}
	_, err := toml.DecodeFile(path, &data)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("cannot read the pin of %s: %w", checkout, err)
	}
	v, ok := data["version"].(string)
	if !ok {
		return "", fmt.Errorf("cannot read the pin of %s: %s sets no version", checkout, config.Rel)
	}
	return v, nil
}
