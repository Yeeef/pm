package work

import (
	"crypto/rand"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Ids, as the work-store page's Ids section gives them: <prefix>-<root>(.<n>)*. The prefix is the repo name, the root
// 3 to 8 characters of [0-9a-z], each n a positive number without leading zeros. Every existing id keeps its text; the
// store indexes by the string and never re-derives it.

const (
	rootChars  = "0123456789abcdefghijklmnopqrstuvwxyz"
	rootMin    = 3 // bd minted 3
	rootMax    = 8
	rootMinted = 4 // pm mints 4, longer on a hit
)

// CheckID is nil when id has the id format, else an error naming it.
func CheckID(id string) error {
	_, _, err := splitID(id)
	return err
}

// splitID is the id's prefix and its root with the child numbers (["9va", "84", "4"]).
func splitID(id string) (string, []string, error) {
	bad := func(why string) error {
		return fmt.Errorf("%q is not a work-store id (<prefix>-<root>(.<n>)*): %s", id, why)
	}
	dash := strings.LastIndexByte(id, '-')
	if dash <= 0 {
		return "", nil, bad("no <prefix>-")
	}
	prefix, segs := id[:dash], strings.Split(id[dash+1:], ".")
	for _, r := range prefix {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return "", nil, bad(fmt.Sprintf("the prefix has %q", r))
		}
	}
	root := segs[0]
	if len(root) < rootMin || len(root) > rootMax || strings.Trim(root, rootChars) != "" {
		return "", nil, bad("the root is not 3 to 8 of [0-9a-z]")
	}
	for _, n := range segs[1:] {
		if n == "" || n[0] == '0' || strings.Trim(n, "0123456789") != "" {
			return "", nil, bad(fmt.Sprintf("child number %q", n))
		}
		if _, err := strconv.Atoi(n); err != nil {
			return "", nil, bad(fmt.Sprintf("child number %q", n))
		}
	}
	return prefix, segs, nil
}

// childNumber is n when id is <parent>.<n>, else 0.
func childNumber(id, parent string) int {
	rest, ok := strings.CutPrefix(id, parent+".")
	if !ok || strings.Contains(rest, ".") {
		return 0
	}
	if rest == "" || rest[0] == '0' || strings.Trim(rest, "0123456789") != "" {
		return 0
	}
	n, err := strconv.Atoi(rest)
	if err != nil {
		return 0
	}
	return n
}

// NextChild is the id of parent's next child: <parent>.<n>, n one more than the highest n of any id with one more
// segment under parent, wherever that item sits now, so a moved-away child's number is never reused.
func NextChild(ids []string, parent string) string {
	top := 0
	for _, id := range ids {
		top = max(top, childNumber(id, parent))
	}
	return fmt.Sprintf("%s.%d", parent, top+1)
}

// NewRoot mints a root id: <prefix>-<4 random base36 characters>, checked against taken, and a character longer on
// each hit, up to 8.
func NewRoot(prefix string, taken func(id string) bool, random io.Reader) (string, error) {
	if prefix == "" {
		return "", fmt.Errorf("work store: no id prefix to mint a root id with")
	}
	for n := rootMinted; n <= rootMax; n++ {
		b := make([]byte, n)
		if _, err := io.ReadFull(random, b); err != nil {
			return "", fmt.Errorf("work store: random root id: %w", err)
		}
		for i := range b {
			b[i] = rootChars[int(b[i])%len(rootChars)]
		}
		id := prefix + "-" + string(b)
		if err := CheckID(id); err != nil {
			return "", err
		}
		if !taken(id) {
			return id, nil
		}
	}
	return "", fmt.Errorf("work store: every minted root id under %s up to %d characters was taken", prefix, rootMax)
}

// newUUID is a random (version 4) UUID, the form of comment ids.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// CompareIDs is the natural order of ids, which pm's lists and pages use: the prefix and root as text, then each
// child part in turn, a number by its value before any other part, so 9va.9 < 9va.39 < 9va.100 and 9va.9.2 <
// 9va.9.10; the text breaks ties ("01" before "1"), and a shorter id comes first. Python's beads.id_key is the same
// order.
func CompareIDs(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	if c := strings.Compare(as[0], bs[0]); c != 0 {
		return c
	}
	for i := 1; i < len(as) && i < len(bs); i++ {
		if c := comparePart(as[i], bs[i]); c != 0 {
			return c
		}
	}
	return len(as) - len(bs)
}

// comparePart orders two child parts: numbers first, by value (any length), then the text.
func comparePart(a, b string) int {
	an, bn := isNumber(a), isNumber(b)
	switch {
	case an && !bn:
		return -1
	case !an && bn:
		return 1
	case an:
		at, bt := strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
		if len(at) != len(bt) {
			return len(at) - len(bt)
		}
		if c := strings.Compare(at, bt); c != 0 {
			return c
		}
	}
	return strings.Compare(a, b)
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
