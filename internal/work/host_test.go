package work

import (
	"os"
	"testing"

	"github.com/Yeeef/pm/internal/buildinfo"
)

// served is a store a test host serves: its host, and the options its clients take.
type served struct {
	h    *Host
	main string
	o    Options
}

// shortMain is a new clone directory under a short temp root, removed at the test's end: t.TempDir's paths can be
// too long for the socket on macOS.
func shortMain(t testing.TB) string {
	t.Helper()
	main, err := os.MkdirTemp("", "pm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(main) })
	return main
}

// host starts a host on main with ops, closed at the test's end.
func host(t testing.TB, main string, o Options, ops Ops) *served {
	t.Helper()
	h, err := NewHost(HostOptions{Main: main, Version: buildinfo.Version, Now: o.Now, Ops: ops})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	return &served{h, main, o}
}

// dial is a client of the store, shut down at the test's end.
func (s *served) dial(t testing.TB) *Dolt {
	t.Helper()
	d, err := dial(s.h.sock, dialConfig{prefix: s.o.Prefix, now: s.o.Now, withDB: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Shutdown() })
	return d
}

// serve starts a host on a new clone directory, makes an empty store there and returns a client of it.
func serve(t testing.TB, o Options) (*Dolt, *served) {
	t.Helper()
	s := host(t, shortMain(t), o, Ops{})
	d, err := dial(s.h.sock, dialConfig{prefix: o.Prefix, now: o.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Shutdown() })
	if err := d.CreateStore(); err != nil {
		t.Fatal(err)
	}
	return d, s
}
