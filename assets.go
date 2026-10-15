// Package pm holds the files pm embeds: prime.md, style.css, the prompts in prompts/ and the release notes
// (CHANGELOG.md and the entry files in changelog.d/). go:embed reaches only files at or below the embedding package's
// directory, so this package at the module root embeds them.
package pm

import (
	"embed"
	"strings"
)

// Rules is prime.md: the rules pm prime prints.
//
//go:embed prime.md
var Rules string

// Style is style.css: the one stylesheet every page of the site gets.
//
//go:embed style.css
var Style string

// Notes is CHANGELOG.md and changelog.d/ as this pm was built: every earlier release's notes, and this release's
// section or, when the build came before it was written (a release builds before its release commit), its entry
// files. pm upgrade --to X prints the upgrade guide of each release it crosses from them (launch.Guides).
// changelog.d/README.md, tracked and never an entry, keeps the directory from being empty, which go:embed refuses.
//
//go:embed CHANGELOG.md changelog.d
var Notes embed.FS

// The model prompts and hook texts in prompts/, each without its final newline.
var (
	OwnerRequestSystem   = prompt(ownerRequestSystem)   // the owner-request judge's system prompt
	OwnerRequestReason   = prompt(ownerRequestReason)   // its block reason for an uncovered request; {asks} filled in
	OwnerRequestNeedless = prompt(ownerRequestNeedless) // its block reason for a needless ask; {asks} filled in
	DaySummary           = prompt(daySummary)           // pm day summarize's prompt; {day} and {activity} filled in
	// the request rule for each session mode, which pm prime --request-rule (a SessionStart hook) and pm interactive and
	// pm autopilot print
	RequestRuleAutopilot   = prompt(requestRuleAutopilot)
	RequestRuleInteractive = prompt(requestRuleInteractive)
)

//go:embed prompts/owner_request_system.txt
var ownerRequestSystem string

//go:embed prompts/owner_request_reason.txt
var ownerRequestReason string

//go:embed prompts/owner_request_needless.txt
var ownerRequestNeedless string

//go:embed prompts/day_summary.txt
var daySummary string

//go:embed prompts/request_rule_autopilot.txt
var requestRuleAutopilot string

//go:embed prompts/request_rule_interactive.txt
var requestRuleInteractive string

func prompt(file string) string { return strings.TrimSuffix(file, "\n") }
