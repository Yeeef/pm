// Package worktest is a read-only work.Store over a fixed list of items, for tests of the code that reads items (the
// records, the site, pm check) before the embedded Dolt store exists. The items come as pm export gives them; the
// parity corpus feeds them from bd issues through the neutral-test mapper (tests/work_items.py).
package worktest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/Yeeef/pm/internal/work"
)

// Store holds the items; every write fails.
type Store struct{ items []work.Item }

var _ work.Store = (*Store)(nil)

// New is a store holding items, in that order.
func New(items []work.Item) *Store { return &Store{items} }

// FromJSON is a store holding the items in a JSON array file.
func FromJSON(path string) (*Store, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var items []work.Item
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return New(items), nil
}

var errReadOnly = errors.New("worktest.Store is read-only")

func (s *Store) Items() ([]work.Item, error) { return append([]work.Item{}, s.items...), nil }

func (s *Store) Get(ids ...string) ([]work.Item, error) {
	var out []work.Item
	for _, id := range ids {
		found := false
		for _, it := range s.items {
			if it.ID == id {
				out, found = append(out, it), true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("no item %s", id)
		}
	}
	return out, nil
}

func (s *Store) Needs(session string) ([]work.Item, error) {
	var out []work.Item
	for _, it := range s.items {
		if it.Need != nil && it.Need.RaisedBy != nil && it.Need.RaisedBy.Session == session {
			out = append(out, it)
		}
	}
	return out, nil
}

func (s *Store) Create(work.New) (work.Item, error)                  { return work.Item{}, errReadOnly }
func (s *Store) Edit(string, *string, *string) error                 { return errReadOnly }
func (s *Store) Close(string, string, work.Resolution, string) error { return errReadOnly }
func (s *Store) SetResolution(string, work.Resolution) error         { return errReadOnly }
func (s *Store) Move(string, string) error                           { return errReadOnly }
func (s *Store) Claim(string, work.Holder, func(string) bool) error  { return errReadOnly }
func (s *Store) Release(string, string) error                        { return errReadOnly }
func (s *Store) DepAdd(string, string) error                         { return errReadOnly }
func (s *Store) DepRemove(string, string) error                      { return errReadOnly }
func (s *Store) Answer(string, string) error                         { return errReadOnly }
func (s *Store) UpdateNeed(string, work.NeedUpdate) error            { return errReadOnly }
func (s *Store) Shutdown() error                                     { return nil }
func (s *Store) Comment(string, work.CommentKind, string, string) (work.Comment, error) {
	return work.Comment{}, errReadOnly
}
