// Package worktest holds the work store's test doubles: ServeAt, a real store served by a host on a clone directory
// the caller gives, for tests that need one; and Store, a read-only work.Store over a fixed list of items, for tests
// of the code that reads items (the records, the site, pm check). The items come as pm export gives them.
package worktest

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/work"
)

// ServeAt starts a host on the clone directory main, with ops, makes an empty store there and dials one client; the
// test's end stops both. The caller removes main, which it keeps short: the host's socket must fit the kernel's limit.
func ServeAt(t testing.TB, main string, ops work.Ops) (*work.Host, *work.Dolt) {
	t.Helper()
	h, err := work.NewHost(work.HostOptions{Main: main, Version: buildinfo.Version, Ops: ops})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	d, err := work.DialSetup(main)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Shutdown() })
	if err := d.CreateStore(); err != nil {
		t.Fatal(err)
	}
	return h, d
}

// Store holds the items; every write fails.
type Store struct{ items []work.Item }

var _ work.Store = (*Store)(nil)

// New is a store holding items, in that order.
func New(items []work.Item) *Store { return &Store{items} }

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
func (s *Store) MoveSprint(string, string, string) (work.Item, error) {
	return work.Item{}, errReadOnly
}
func (s *Store) Claim(string, work.Holder, func(string) bool) error { return errReadOnly }
func (s *Store) Release(string, string) error                       { return errReadOnly }
func (s *Store) DepAdd(string, string) error                        { return errReadOnly }
func (s *Store) DepRemove(string, string) error                     { return errReadOnly }
func (s *Store) Answer(string, string) error                        { return errReadOnly }
func (s *Store) UpdateNeed(string, work.NeedUpdate) error           { return errReadOnly }
func (s *Store) Shutdown() error                                    { return nil }
func (s *Store) Comment(string, work.CommentKind, string, string) (work.Comment, error) {
	return work.Comment{}, errReadOnly
}
