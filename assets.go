// Package pm holds the files Go pm and Python pm share. Until the cut-over, prime.md and style.css stay in the Python
// package (src/pm), which ships and reads them there; go:embed reaches only files at or below the embedding package's
// directory, so this package at the module root embeds them, and both implementations read one copy.
package pm

import _ "embed"

// Rules is prime.md: the rules pm prime prints.
//
//go:embed src/pm/prime.md
var Rules string

// Style is style.css: the one stylesheet every page of the site gets.
//
//go:embed src/pm/style.css
var Style string
