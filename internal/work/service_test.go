package work

import (
	"context"
	"os"
	"reflect"
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
