package site

import (
	"regexp"
	"strings"
	"testing"
)

// A range like mermaid@11 moves under the site whenever Mermaid releases, and a page caught mid-move or by a CDN blip
// shows the diagram's source; every Mermaid URL the pages load names one exact release.
func TestMermaidLoadsOneExactVersionAtNaturalWidth(t *testing.T) {
	urls := regexp.MustCompile(`https://[^"]*`).FindAllString(mermaidScript, -1)
	if len(urls) < 2 {
		t.Fatalf("want a CDN and a fallback, got %q", urls)
	}
	exact := regexp.MustCompile(`/mermaid@(\d+\.\d+\.\d+)/dist/mermaid\.esm\.min\.mjs$`)
	for _, u := range urls {
		if m := exact.FindStringSubmatch(u); m == nil || m[1] != MermaidVersion {
			t.Errorf("%s does not name mermaid@%s exactly", u, MermaidVersion)
		}
	}
	for _, want := range []string{"flowchart: wide", "useMaxWidth: false", `addEventListener("change"`,
		`attributeFilter: ["data-theme"]`} {
		if !strings.Contains(mermaidScript, want) {
			t.Errorf("the script lacks %q", want)
		}
	}
}
