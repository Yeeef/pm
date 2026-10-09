package records

import "strings"

// The prompt line(s) each section opens with, by record type; Python's *_PROMPTS in cli.py.
var (
	SprintPrompts = map[string]string{
		"Goal": "> What should be true when this sprint ends, and why now?",
		"Scope": "> What's in, and what's explicitly out? Keep this high level; implementation\n" +
			"> details go in a design page.",
		"Done when":    "> What evidence will show the goal is met?",
		"Design pages": "> Where is the detail?",
		"Progress": "> Where is the sprint now? Generated from Beads when the page is rendered.\n" +
			"> Do not write here.",
		"Decisions": "> What did we choose inside this sprint, and why?",
		"Findings": "> What did we learn that changes the design, the plan, or how we work? Add\n" +
			"> results with their numbers.",
		"Delivery report":     "> Written at close. Each part holds \"Not closed yet.\" until then.",
		"Outcome":             "> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.",
		`Against "Done when"`: "> Each item, met or not, with its evidence (a page, a command, a number).",
	}
	ProjectPrompts = map[string]string{
		"Goal": "> Why do we do it? What is it? What outcome do we expect?",
		"Progress": "> Where are we now, and what's next? Generated from Beads and the sprint\n" +
			"> records when the page is rendered. Do not write here.",
		"Decisions":    "> What constrains every future sprint? Sprint-only choices live in the sprint\n> record.",
		"Design pages": "> Where is the detail?",
		"Outcome": "> Written when the project closes: what was achieved against the goal, what\n" +
			"> was learned, what was retired, and links to the sprint delivery reports.",
	}
	DesignPrompts = []string{
		"Problem", "> What are we solving, and why now?",
		"Goals and non-goals", "> What must the design achieve, and what does it deliberately leave out?",
		"Constraints and key facts", "> Which facts, findings and constraints shaped the design? Only those that\n" +
			"> still hold; sprint records keep the findings as they happened.",
		"Design", "> What is it, in its final state? Free `###` subsections. Decisions and plans\n" +
			"> live in the project and sprint records.",
		"Alternatives considered", "> What else was considered and not adopted, and why not?",
		"Prior art", "> Optional. What existing work did we learn from, and what did we take?",
		"Open questions", "> What is still unresolved?",
	}
	PostmortemPrompts = []string{
		"Summary", "> What broke, for whom, and how was it noticed?",
		"Timeline", "> What happened when? Times with their zone, from the first cause to the fix.",
		"Cost", "> What did it cost: time lost, sessions or people blocked, work redone?",
		"Root cause", "> Why did it happen? The cause under the trigger.",
		"What changed", "> What was fixed, and what changed so it does not recur? Name the commits and PRs.",
		"What would have caught it earlier", "> Which test, check or rule would have caught it before it cost anything?",
	}
)

// SprintText is a new sprint record: the frame's Goal, Scope and Done when, every other section empty.
func SprintText(title, bead string, frame map[string]string) string {
	p := SprintPrompts
	return "---\ntype: sprint\ntitle: " + title + "\nbead: " + bead + "\n---\n\n" +
		"## Goal\n\n" + p["Goal"] + "\n\n" + frame["Goal"] + "\n\n" +
		"## Scope\n\n" + p["Scope"] + "\n\n" + frame["Scope"] + "\n\n" +
		"## Done when\n\n" + p["Done when"] + "\n\n" + frame["Done when"] + "\n\n" +
		"## Design pages\n\n" + p["Design pages"] + "\n\n" + NoneYet + "\n\n" +
		"## Progress\n\n" + p["Progress"] + "\n\n" +
		"## Decisions\n\n" + p["Decisions"] + "\n\n" + NoneYet + "\n\n" +
		"## Findings\n\n" + p["Findings"] + "\n\n" + NoneYet + "\n\n" +
		"## Delivery report\n\n" + p["Delivery report"] + "\n\n" +
		"### Outcome\n\n" + p["Outcome"] + "\n\n" + NotClosed + "\n\n" +
		"### Against \"Done when\"\n\n" + p[`Against "Done when"`] + "\n\n" + NotClosed + "\n"
}

// ProjectText is a new project record with its goal.
func ProjectText(title, bead, goal string) string {
	p := ProjectPrompts
	return "---\ntype: project\ntitle: " + title + "\nbead: " + bead + "\n---\n\n" +
		"## Goal\n\n" + p["Goal"] + "\n\n" + goal + "\n\n" +
		"## Progress\n\n" + p["Progress"] + "\n\n" +
		"## Decisions\n\n" + p["Decisions"] + "\n\n" + NoneYet + "\n\n" +
		"## Design pages\n\n" + p["Design pages"] + "\n\n" + NoneYet + "\n\n" +
		"## Outcome\n\n" + p["Outcome"] + "\n\n" + NotClosed + "\n"
}

func emptySections(prompts []string) string {
	var b strings.Builder
	for i := 0; i < len(prompts); i += 2 {
		b.WriteString("## " + prompts[i] + "\n\n" + prompts[i+1] + "\n\n" + NoneYet + "\n\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// DesignText is a new design page, every section holding its prompt and "None yet.".
func DesignText(title, project string) string {
	return "---\ntype: design\ntitle: " + title + "\nproject: " + project + "\n---\n\n" + emptySections(DesignPrompts)
}

// PostmortemText is a new postmortem; target is its "sprint: …" or "project: …" header line.
func PostmortemText(title, day, target string) string {
	return "---\ntype: postmortem\ntitle: " + title + "\ndate: " + day + "\n" + target + "\n---\n\n" +
		emptySections(PostmortemPrompts)
}
