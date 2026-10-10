package launch

import "testing"

func TestNewerComparesVersionsAndAPreReleaseWithItsRelease(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.5.0", "0.4.0", true}, {"0.10.0", "0.9.1", true}, {"0.4.0", "0.4.0", false}, {"0.3.9", "0.4.0", false},
		{"0.4.0", "0.4.0-rc.1", true}, {"0.4.0-rc.2", "0.4.0", false}, {"0.4.1", "0.5.0-rc.1", false},
		{"x", "0.4.0", false}, {"0.5.0", "dev", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
