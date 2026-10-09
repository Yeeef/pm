package work

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

// The id rules of the work-store page's Ids section.

func TestCheckIDTakesThePagesFormat(t *testing.T) {
	for _, id := range []string{"yeeef-agents-9va", "yeeef-agents-9va.41.2", "demo-abc", "demo-abcd1234", "a_b-x9z.10"} {
		if err := CheckID(id); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
	for _, id := range []string{"9va", "-9va", "demo-ab", "demo-abcdefghi", "demo-9VA", "demo-9va.0", "demo-9va.01",
		"demo-9va.", "demo-9va..1", "demo-9va.x", "demo-9va.-1", "de mo-9va", "my.repo-9va"} {
		if err := CheckID(id); err == nil {
			t.Errorf("%s passed", id)
		}
	}
}

func TestNextChildIsOneAboveTheHighestChildNumberEverMinted(t *testing.T) {
	p := "demo-9va"
	ids := []string{p, p + ".1", p + ".2", p + ".10", p + ".2.5", "demo-abc.11", p + ".1.10.1"}
	if got := NextChild(ids, p); got != p+".11" {
		t.Errorf("%s", got)
	}
	// A moved-away child keeps its id, so its number counts wherever it sits now: the ids are all that count.
	if got := NextChild(append(ids, p+".12"), p); got != p+".13" {
		t.Errorf("%s", got)
	}
	// p.1's children are p.1.<n>: p.10 and p.10.1 are not among them.
	if got := NextChild(append(ids, p+".10.1"), p+".1"); got != p+".1.1" {
		t.Errorf("%s", got)
	}
	if got := NextChild(ids, p+".2"); got != p+".2.6" {
		t.Errorf("%s", got)
	}
}

func TestNewRootMintsFourBase36CharactersAndALongerOneOnAHit(t *testing.T) {
	random := bytes.NewReader(bytes.Repeat([]byte{0, 1, 35, 36, 71}, 20))
	first := ""
	id, err := NewRoot("demo", func(id string) bool {
		if first == "" {
			first = id
			return true
		}
		return false
	}, random)
	if err != nil {
		t.Fatal(err)
	}
	if first != "demo-01z0" || id != "demo-z01z0" { // bytes mod 36: 0 1 35 36 → 0 1 z 0; then 71 0 1 35 36
		t.Errorf("first %s, then %s", first, id)
	}
	if _, err := NewRoot("demo", func(string) bool { return true }, random); err == nil ||
		!strings.Contains(err.Error(), "taken") {
		t.Errorf("every id taken: %v", err)
	}
	if _, err := NewRoot("", func(string) bool { return false }, random); err == nil {
		t.Error("minted without a prefix")
	}
}

func TestIDsOrderByTheirNumbers(t *testing.T) {
	ids := []string{"x-9va.10", "x-9va.2", "x-9va", "x-9va.2.1", "x-abc", "x-9va.1"}
	slices.SortFunc(ids, compareIDs)
	if got := strings.Join(ids, " "); got != "x-9va x-9va.1 x-9va.2 x-9va.2.1 x-9va.10 x-abc" {
		t.Error(got)
	}
}
