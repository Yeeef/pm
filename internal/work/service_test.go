package work

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// The pm service rereads the items only when the fingerprint moved: a read must leave it as it was, every write must
// move it, and a store made anew in its place must move it too.
func TestTheMarkMovesOnAWriteAndNotOnARead(t *testing.T) {
	d, _ := newStore(t)
	seed(t, d)
	before := must(d.Mark())
	must(d.Items())
	if after := must(d.Mark()); after != before {
		t.Fatalf("a read moved the mark: %s, was %s", after, before)
	}
	must(d.Create(New{Type: Project, Title: "Q"}))
	if after := must(d.Mark()); after == before {
		t.Fatal("a write left the mark")
	}
}

func TestGCKeepsEveryItem(t *testing.T) {
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
}

// The host's GC runs while commands write (GC takes no slot and no write lock). Dolt before dolthub/dolt#11312
// (merged as b130ee82ebe9, 2026-07-17) let the PruneTableFiles that ends a collection delete a chunk journal a
// concurrent write had just made: the writes since landed in an unlinked file, lost on the next open, and the next
// collection panicked the process ("remove …/noms/vvvv…: no such file or directory: error dropping journal writer
// during UpdateGCGen", pm CI run 38024394987). The race is inside Dolt, so pm cannot avoid it; the Dolt it links must
// hold the fix. The test reads it from go.mod: a test binary's build info lists no dependencies.
func TestDoltHoldsTheJournalPruneFix(t *testing.T) {
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

// The pm service bounds its sync: a sync whose context is done stops, pushes nothing and leaves
// the store's items as they were; the next sync with time left pushes what it did not. (A context done before Dolt
// first reached the remote would kill the git init of its remote cache, which no later sync repairs: the service
// checks its context before it starts a sync.)
func TestSyncContextStopsOnceItsContextIsDone(t *testing.T) {
	d, _ := newStore(t)
	seed(t, d)
	defer d.Shutdown()
	bare := bareRemote(t)
	if err := d.AddRemote(bare); err != nil {
		t.Fatal(err)
	}
	must(d.SyncContext(context.Background()))
	pushed := gitRun(t, bare, "rev-parse", RemoteRef)
	must(d.Create(New{Type: Project, Title: "Later"}))
	before := must(d.Items())
	c, cancel := context.WithCancel(context.Background())
	cancel()
	// Dolt kills the git it runs under the query's context
	if _, err := d.SyncContext(c); err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("a sync under a done context: %v", err)
	}
	if now := gitRun(t, bare, "rev-parse", RemoteRef); now != pushed {
		t.Fatalf("pushed under a done context: %s, was %s", now, pushed)
	}
	if after := must(d.Items()); !reflect.DeepEqual(after, before) {
		t.Fatal("the items moved")
	}
	if r := must(d.SyncContext(context.Background())); r.Pushed != 1 {
		t.Fatalf("the next sync did not push the later commit: %+v", r)
	}
}
