package work

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
)

// The pm service rereads the items only when the fingerprint moved: a read must leave it as it was, every write must
// move it, and a store made anew in its place must move it too.
func TestFingerprintMovesOnAWriteAndNotOnARead(t *testing.T) {
	d, o := newStore(t)
	seed(t, d)
	if err := d.Shutdown(); err != nil {
		t.Fatal(err)
	}
	before := must(Fingerprint(o.Dir))
	for range 2 {
		r := must(OpenStore(o))
		must(r.Items())
		if err := r.Shutdown(); err != nil {
			t.Fatal(err)
		}
		if after := must(Fingerprint(o.Dir)); after != before {
			t.Fatalf("a read moved the fingerprint:\n%q\n%q", before, after)
		}
	}
	w := must(OpenStore(o))
	must(w.Create(New{Type: Project, Title: "Another"}))
	if err := w.Shutdown(); err != nil {
		t.Fatal(err)
	}
	written := must(Fingerprint(o.Dir))
	if written == before {
		t.Fatal("a write left the fingerprint as it was")
	}
	if err := os.RemoveAll(o.Dir); err != nil {
		t.Fatal(err)
	}
	if _, err := Fingerprint(o.Dir); err == nil {
		t.Fatal("no store, yet a fingerprint")
	}
	n := must(CreateStore(o))
	seed(t, n)
	must(n.Create(New{Type: Project, Title: "Another"}))
	if err := n.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if must(Fingerprint(o.Dir)) == written {
		t.Fatal("a store made anew kept the old one's fingerprint")
	}
}

func TestGCKeepsEveryItem(t *testing.T) {
	d, o := newStore(t)
	seed(t, d)
	before := must(d.Items())
	if err := d.GC(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := d.Shutdown(); err != nil {
		t.Fatal(err)
	}
	r := must(OpenStore(o))
	defer r.Shutdown()
	if after := must(r.Items()); !reflect.DeepEqual(after, before) {
		t.Fatalf("gc changed the items:\n%v\n%v", before, after)
	}
}

// The pm service bounds its sync, which holds the gate: a sync whose context is done stops, pushes nothing and leaves
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
