package work

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
)

// FromExport reads what Export writes, one item per line, back into items, ids kept: the input of pm init --import,
// which moves a project's items from one clone's work store into another's. A line that is not one JSON object of
// Item's fields (an unknown key among them) fails with its number, and the items must pass Check.
func FromExport(r io.Reader) ([]Item, error) {
	var items []Item
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for n := 1; sc.Scan(); n++ {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var it Item
		if err := strictly(line, &it); err != nil {
			return nil, fmt.Errorf("import: line %d: %w", n, err)
		}
		items = append(items, it)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("import: %w", err)
	}
	if err := Check(items); err != nil {
		return nil, fmt.Errorf("import: %w", err)
	}
	return items, nil
}
