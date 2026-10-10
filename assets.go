// Package pm holds the files pm embeds: prime.md, style.css and the prompts in prompts/. go:embed reaches only files at
// or below the embedding package's directory, so this package at the module root embeds them.
package pm

import (
	_ "embed"
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

// The model prompts and hook texts in prompts/, each without its final newline.
var (
	OwnerRequestSystem   = prompt(ownerRequestSystem)   // the owner-request judge's system prompt
	OwnerRequestReason   = prompt(ownerRequestReason)   // its block reason for an uncovered request; {asks} filled in
	OwnerRequestNeedless = prompt(ownerRequestNeedless) // its block reason for a needless ask; {asks} filled in
	DaySummary           = prompt(daySummary)           // pm day summarize's prompt; {day} and {activity} filled in
)

//go:embed prompts/owner_request_system.txt
var ownerRequestSystem string

//go:embed prompts/owner_request_reason.txt
var ownerRequestReason string

//go:embed prompts/owner_request_needless.txt
var ownerRequestNeedless string

//go:embed prompts/day_summary.txt
var daySummary string

func prompt(file string) string { return strings.TrimSuffix(file, "\n") }
