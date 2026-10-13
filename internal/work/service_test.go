package work

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// The host's GC runs while commands write (GC takes no slot and no write lock). Dolt before dolthub/dolt#11312
// (merged as b130ee82ebe9, 2026-07-17) let the PruneTableFiles that ends a collection delete a chunk journal a
// concurrent write had just made: the writes since landed in an unlinked file, lost on the next open, and the next
// collection panicked the process ("remove …/noms/vvvv…: no such file or directory: error dropping journal writer
// during UpdateGCGen", pm CI run 38024394987). The race is inside Dolt, so pm cannot avoid it; the Dolt it links must
// hold the fix. The test reads it from go.mod: a test binary's build info lists no dependencies.
func TestDoltHoldsTheJournalPruneFix(t *testing.T) {
	t.Run("GCKeepsEveryItem", func(t *testing.T) {
		d, o := newStore(t)
		seed(t, d)
		before := must(d.Items())
		if err := o.h.GC(context.Background()); err != nil {
			t.Fatal(err)
		}
		r := o.dial(t)
		if after := must(r.Items()); !reflect.DeepEqual(after, before) {
			t.Fatalf("gc changed the items:\n%v\n%v", before, after)
		}
	})

	const fixed = "2026-07-17T20:48:48Z" // b130ee82ebe9's commit time
	data, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := modfile.ParseLax("go.mod", data, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range f.Require {
		if r.Mod.Path != "github.com/dolthub/dolt/go" {
			continue
		}
		v := r.Mod.Version
		at, err := module.PseudoVersionTime(v)
		if err != nil { // a tagged release: every one after the pin's base v0.40.5 is later than the fix
			if semver.Compare(v, "v0.40.5") <= 0 {
				t.Fatalf("Dolt %s predates the journal prune fix (dolthub/dolt#11312)", v)
			}
			return
		}
		if want, _ := time.Parse(time.RFC3339, fixed); at.Before(want) {
			t.Fatalf("Dolt %s (%s) predates the journal prune fix (dolthub/dolt#11312, %s)", v, at.Format(time.RFC3339), fixed)
		}
		return
	}
	t.Fatal("go.mod does not require github.com/dolthub/dolt/go")
}

// A collection returns only once each of its visits to the other sessions has ended (host_gc.go): Dolt's ends a
// visit after the collection's wait, so the next collection could begin a visit of a session whose last one had not
// ended, and both closed one channel ("panic: close of closed channel"). Idle sessions are what a collection visits.
func TestACollectionRacingItsOwnVisitsReturnsOnceEachEnds(t *testing.T) {
	_, o := newStore(t)
	for range 32 {
		must(o.dial(t).Mark())
	}
	for i := range 30 {
		if err := o.h.GC(context.Background()); err != nil {
			t.Fatal(err)
		}
		for _, s := range o.h.visits.states() {
			if s.HasOutstandingVisitCall() {
				t.Fatalf("collection %d returned while a session's visit had not ended", i)
			}
		}
	}
}
