// Every command, its help, description and arguments, in the order pm --help lists them.

package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/pflag"

	"github.com/Yeeef/pm/internal/hooks"
)

// chunkNumbers is pm prime --rules's choices: 1 to the number of rules chunks.
func chunkNumbers() []string {
	out := make([]string, len(hooks.Starts))
	for i := range out {
		out[i] = strconv.Itoa(i + 1)
	}
	return out
}

// tree is pm's command tree.
var tree = &command{
	name:        "pm",
	description: "pm: the write path for project records, and project actions that touch both records and the work store.",
	epilog:      "pm runs the pm version the repo pins in .pm/config.toml: another version's release binary, downloaded once; a pin below 0.2.0 (a retired Python release) is refused. pm where names the version running and why.",
	subDest:     "cmd",
	subs: []*command{
		{
			name:        "show",
			help:        "compact status of open projects for agents, level by level",
			description: "Project state, level by level. Without a flag, the top level: push failures, tasks other live sessions hold, the site, the repo's pm feedback doc, and per open project a line with each open owner request and undelivered reply. Each level names the command for the next: --project, then --sprint, then --record with --section. pm show ID [--json] prints one item of any type instead: its fields, holder, blockers, children, needs and comments (pm show ID --help).",
			args: []arg{
				{flags: []string{"--json"}, dest: "json", kind: flagTrue, help: "every level's data as one JSON object"},
				{flags: []string{"--project"}, dest: "project", metavar: []string{"NAME"}, kind: value, help: "one open project (its name or id): its goal, owner requests in full, the repo's pm feedback doc, open sprints with their tasks, and last decisions"},
				{flags: []string{"--sprint"}, dest: "sprint", metavar: []string{"ID"}, kind: value, help: "one sprint's frame, findings and tasks"},
				{flags: []string{"--record"}, dest: "record", metavar: []string{"PATH"}, kind: value, help: "with --section: the record to read (a path, sprint id, project name or design slug)"},
				{flags: []string{"--section"}, dest: "section", metavar: []string{"NAME"}, kind: value, help: "with --record: print that one section, heading included"},
				{flags: []string{"--refresh-inbox"}, dest: "refresh_inbox", kind: flagTrue, help: "first point this session's open requests at its current inbox ($CLAUDE_CODE_MESSAGING_SOCKET); the session-start hook passes it, since a resumed session binds a new one"},
			},
		},
		{
			name:    "day",
			help:    "day pages: generated from the day's activity; nobody writes a day record",
			subDest: "sub",
			subs: []*command{
				{
					name:        "summarize",
					help:        "generate today's Today summary with claude -p; the pm service runs it",
					description: "Summarize today's activity (what the day page shows, plus the records committed today) with claude -p --model claude-haiku-5-5 into records/days/<today>.summary.json, committed on the records branch; the day page shows it as Today, labelled with the time it was generated. Skips when nothing happened today or the activity's digest is unchanged since the last summary. Fails, writing nothing, when claude is missing or fails. The pm service runs it every 10 minutes.",
					args: []arg{
						{flags: []string{"--dry-run"}, dest: "dry_run", kind: flagTrue, help: "print the summary it would write; write nothing"},
					},
				},
			},
		},
		{
			name:    "finding",
			help:    "sprint findings",
			subDest: "sub",
			subs: []*command{
				{
					name:        "add",
					help:        "append a bullet to a sprint's Findings; the text as one argument, or with --text",
					description: "Append the finding, as a bullet, to the open sprint's Findings, with its numbers. Give the text as one quoted argument, or as the body: --text=\"…\" for one plain line, or --text-file - <<'EOF' … EOF. Refuses a text that starts with --, which is an option misspelt rather than a finding.",
					groups:      []bool{false},
					args: []arg{
						{flags: []string{"--sprint"}, dest: "sprint", metavar: []string{"ID"}, required: true, kind: value, help: "the sprint's id"},
						{dest: "words", metavar: []string{"TEXT"}, nargs: "*", kind: value, help: "the finding, as one quoted argument (words given apart are joined by spaces)"},
						{flags: []string{"--text-file"}, dest: "text_file", metavar: []string{"PATH"}, kind: value, group: 1, help: "the finding instead, read from PATH, or with - from stdin as a quoted heredoc: --text-file - <<'EOF' … EOF"},
						{flags: []string{"--text"}, dest: "text", kind: value, def: "", group: 1, help: "the finding instead, inline, for one plain line only; backticks, $ or quotes go in --text-file"},
					},
				},
			},
		},
		{
			name:    "feedback",
			help:    "feedback on pm itself",
			subDest: "sub",
			subs: []*command{
				{
					name:        "add",
					help:        "append an entry to the repo's pm feedback doc; the text with --text",
					description: "When pm got in the way (a confusing refusal, a missing command, a rule that cost time), say once what happened and what would have helped. Appends a dated entry with this session's id to the repo's one feedback doc, records/docs/pm-feedback.md, creating it on first use; --project, --sprint and --task tag the entry with what it is about. pm show and pm show --project link the doc.",
					groups:      []bool{false},
					args: []arg{
						{flags: []string{"--project"}, dest: "project", metavar: []string{"NAME"}, kind: value, help: "the project the feedback is about"},
						{flags: []string{"--sprint"}, dest: "sprint", metavar: []string{"ID"}, kind: value, help: "the sprint the feedback is about"},
						{flags: []string{"--task"}, dest: "task", metavar: []string{"ID"}, kind: value, help: "the task the feedback is about"},
						{flags: []string{"--text-file"}, dest: "text_file", metavar: []string{"PATH"}, kind: value, group: 1, help: "required: what happened and what would have helped; read from PATH, or with - from stdin as a quoted heredoc: --text-file - <<'EOF' … EOF"},
						{flags: []string{"--text"}, dest: "text", kind: value, def: "", group: 1, help: "the same body inline, for one plain line only; several lines, backticks, $ or quotes go in --text-file"},
						{flags: []string{"--session"}, dest: "session", metavar: []string{"ID"}, kind: value, help: "the session to record; default: this session's id from the environment"},
					},
				},
			},
		},
		{
			name:    "decision",
			help:    "decisions: record one, or ask the owner for one",
			subDest: "sub",
			subs: []*command{
				{
					name:        "add",
					help:        "append a decision to a project's or sprint's Decisions",
					description: "Append a ::: decision block, dated today, to the Decisions of the named project or sprint: the --decision line, then the --reason line. Each is one line, in single quotes (in double quotes the shell runs a `code span` as a command). Choosing --level: project if a later sprint must follow it; sprint if it is about this sprint's own work; skip choices cheap to reverse. Source is agent unless --need or --confirmed. With --need, the decision is the owner's answer to that decision need: the block ends 'Answers `<need-id>`.', and the need is closed as answered with the same text unless the owner already closed it.",
					groups:      []bool{false},
					args: []arg{
						{flags: []string{"--decision"}, dest: "decision", metavar: []string{"TEXT"}, required: true, kind: value, help: "the decision, in one line"},
						{flags: []string{"--reason"}, dest: "reason", metavar: []string{"TEXT"}, required: true, kind: value, help: "why it holds, in one line"},
						{flags: []string{"--level"}, dest: "level", choices: []string{"project", "sprint"}, kind: value, help: "required, no default: project if a later sprint must follow it; sprint if it is about this sprint's own work; skip choices cheap to reverse"},
						{flags: []string{"--project"}, dest: "project", metavar: []string{"NAME"}, kind: value, help: "the project record name (with --level project)"},
						{flags: []string{"--sprint"}, dest: "sprint", metavar: []string{"ID"}, kind: value, help: "the sprint's id (with --level sprint)"},
						{flags: []string{"--until"}, dest: "until", kind: value, help: "a known condition to revisit the decision"},
						{flags: []string{"--need"}, dest: "need", metavar: []string{"ID"}, kind: value, group: 1, help: "source=owner: the decision answers this decision need, and closes it if it is open"},
						{flags: []string{"--confirmed"}, dest: "confirmed", kind: flagTrue, group: 1, help: "source=owner: the owner confirmed it"},
					},
				},
				{
					name:        "need",
					help:        "ask the owner for a decision under a sprint or task; its parts as flags",
					description: "Raise a decision need under a sprint or task. Its parts are flags, one line each, and stdin is not read: one --question, one or more --fact, two or more --option LABEL TEXT, one --cost LABEL TEXT for each option, and one --default LABEL REASON that names the option taken if the owner does not answer. Put each value in single quotes: in double quotes the shell runs a `code span` as a command. pm writes the description in one Markdown layout and refuses fewer than two options, a repeated label, an option without exactly one cost, a cost or default that names no option, and a sentence of more than 25 words. Record the answer with pm decision add --need, or close a small answer with pm decision close.",
					args: []arg{
						{flags: []string{"--question"}, dest: "question", metavar: []string{"TEXT"}, required: true, kind: appendValue, help: "what the owner decides, as one question"},
						{flags: []string{"--fact"}, dest: "fact", metavar: []string{"TEXT"}, required: true, kind: appendValue, help: "a fact the owner needs to decide; repeat it for each fact"},
						{flags: []string{"--option"}, dest: "option", metavar: []string{"LABEL", "TEXT"}, nargs: "2", required: true, kind: appendValue, help: "a choice: its label (letters and digits) and what it does; repeat it for each option, two or more"},
						{flags: []string{"--cost"}, dest: "cost", metavar: []string{"LABEL", "TEXT"}, nargs: "2", required: true, kind: appendValue, help: "what the option with this label costs; one for each option"},
						{flags: []string{"--default"}, dest: "default", metavar: []string{"LABEL", "REASON"}, nargs: "2", required: true, kind: appendValue, help: "the option taken if the owner does not answer, and why"},
						{flags: []string{"--title"}, dest: "title", required: true, kind: value},
						{flags: []string{"--parent"}, dest: "parent", metavar: []string{"ID"}, required: true, kind: value, help: "the sprint or task the decision belongs to"},
					},
				},
				{
					name:        "close",
					help:        "close a decision need whose answer sets no rule, with no decision record; the answer with --text",
					description: "Close the need with the owner's answer and the reason, as no-decision; on a need the owner already closed, only the resolution no-decision and the reason (as a comment) are set, and no answer is needed. An answer that sets a rule is recorded with pm decision add --need instead. Answers that set no rule: a name, a port, which of two equal files. If you are not sure, record a decision. A need that became moot before the owner answered has no answer to close it with: close it with pm need dismiss <id> --reason \"…\". If setting the resolution fails, run it again; it does not repeat the reason.",
					groups:      []bool{false},
					args: []arg{
						{dest: "need_id", required: true, kind: value, help: "the decision need's id"},
						{flags: []string{"--reason"}, dest: "reason", required: true, kind: value, help: "why the answer sets no rule, in a sentence"},
						{flags: []string{"--text-file"}, dest: "text_file", metavar: []string{"PATH"}, kind: value, group: 1, help: "the owner's answer, as they gave it; required unless the owner already closed the need; read from PATH, or with - from stdin as a quoted heredoc: --text-file - <<'EOF' … EOF"},
						{flags: []string{"--text"}, dest: "text", kind: value, def: "", group: 1, help: "the same body inline, for one plain line only; several lines, backticks, $ or quotes go in --text-file"},
					},
				},
			},
		},
		{
			name:    "action",
			help:    "actions: ask the owner to do something (run, apply), or to review a PR (--pr)",
			subDest: "sub",
			subs: []*command{
				{
					name:        "need",
					help:        "ask the owner to do something under a sprint or task, or to review a PR (--pr); description with --text",
					description: "Raise an action under a sprint or task. The description (--text) says what to do and why. With --pr it is a PR review instead: every sprint named must be open with its delivery report written (Outcome and 'Against \"Done when\"') and committed; the review goes under the first and blocks its close until the PR merges and you close the review with pm action done <id> --reason \"merged as <sha>\"; the site's card links the PR, each sprint's record and delivery report, and the design pages named with --design plus those the sprints' records list, and shows the focus; --text then holds optional extra context. Close it with pm action done once you see it done.",
					groups:      []bool{false},
					args: []arg{
						{flags: []string{"--title"}, dest: "title", kind: value, help: "required without --pr; with --pr, default: Review PR #<n>"},
						{flags: []string{"--parent"}, dest: "parent", metavar: []string{"ID"}, kind: value, help: "the sprint or task the action belongs to (required without --pr; not allowed with it)"},
						{flags: []string{"--pr"}, dest: "pr", metavar: []string{"URL"}, kind: value, help: "the pull request to review; requires --sprint and --focus"},
						{flags: []string{"--sprint"}, dest: "sprint", metavar: []string{"ID"}, kind: appendValue, help: "with --pr: an open sprint the PR delivers, its report written (repeatable)"},
						{flags: []string{"--focus"}, dest: "focus", kind: value, help: "with --pr: what to look at first: risky changes, open choices"},
						{flags: []string{"--design"}, dest: "design", metavar: []string{"SLUG"}, kind: appendValue, help: "with --pr: a design page behind the PR (repeatable, optional)"},
						{flags: []string{"--text-file"}, dest: "text_file", metavar: []string{"PATH"}, kind: value, group: 1, help: "without --pr, required: what the owner should do and why; with --pr, optional extra context; read from PATH, or with - from stdin as a quoted heredoc: --text-file - <<'EOF' … EOF"},
						{flags: []string{"--text"}, dest: "text", kind: value, def: "", group: 1, help: "the same body inline, for one plain line only; several lines, backticks, $ or quotes go in --text-file"},
					},
				},
				{
					name: "done",
					help: "close an action once you see the owner did it",
					args: []arg{
						{dest: "need_id", required: true, kind: value, help: "the action's id"},
						{flags: []string{"--reason"}, dest: "reason", required: true, kind: value, help: "what showed you it is done (a merged PR, a command's output)"},
					},
				},
			},
		},
		{
			name:    "need",
			help:    "needs of any kind: decisions, actions and reviews",
			subDest: "sub",
			subs: []*command{
				{
					name:        "edit",
					help:        "rewrite an open action's or decision need's body before the owner replies",
					description: "Rewrite the body of an open need that holds no reply yet: an action's description (--text), or a decision need's parts, given as pm decision need takes them (--question, --fact, --option, --cost, --default; pm decision need --help), which pm lays out and checks the same way. The title stays. Refuses a closed need, a need that holds a reply (the owner answered the body it had: raise a new need and dismiss this one with pm need dismiss), and a PR review, whose card pm builds from its PR, sprints and focus.",
					groups:      []bool{false},
					args: []arg{
						{dest: "need_id", required: true, kind: value, help: "the open need's id"},
						{flags: []string{"--text-file"}, dest: "text_file", metavar: []string{"PATH"}, kind: value, group: 1, help: "an action's new description: what the owner should do and why; read from PATH, or with - from stdin as a quoted heredoc: --text-file - <<'EOF' … EOF"},
						{flags: []string{"--text"}, dest: "text", kind: value, def: "", group: 1, help: "the same body inline, for one plain line only; several lines, backticks, $ or quotes go in --text-file"},
						{flags: []string{"--question"}, dest: "question", metavar: []string{"TEXT"}, kind: appendValue, help: "a decision need's question"},
						{flags: []string{"--fact"}, dest: "fact", metavar: []string{"TEXT"}, kind: appendValue, help: "a decision need's fact; repeat it for each fact"},
						{flags: []string{"--option"}, dest: "option", metavar: []string{"LABEL", "TEXT"}, nargs: "2", kind: appendValue, help: "a decision need's choice; two or more"},
						{flags: []string{"--cost"}, dest: "cost", metavar: []string{"LABEL", "TEXT"}, nargs: "2", kind: appendValue, help: "what the option with this label costs; one for each option"},
						{flags: []string{"--default"}, dest: "default", metavar: []string{"LABEL", "REASON"}, nargs: "2", kind: appendValue, help: "the option taken if the owner does not answer, and why"},
					},
				},
				{
					name: "dismiss",
					help: "close an open need as dismissed: a [TEST] need, a replaced review, or one that became moot",
					store: &storeCommand{
						usage: "need dismiss ID --reason REASON",
						about: "Close an open need as dismissed, with no answer: a [TEST] need, a replaced review, or a decision or action that became moot. pm sprint close skips a dismissed review.",
						flags: func(fs *pflag.FlagSet) { fs.String("reason", "", "why it is dismissed (required)") },
						args:  1,
						run:   needDismiss,
					},
				},
			},
		},
		{
			name:    "reply",
			help:    "the owner's replies from the site",
			subDest: "sub",
			subs: []*command{
				{
					name:        "read",
					help:        "print the owner's replies and reviews' merges that did not reach your session",
					description: "Print the requests' site replies not yet delivered and their reviews' PR merges not yet reported, with what to do next, and mark them delivered; it does not wait. The pm service pushes each reply and merge into the inbox of the session that raised the request when it can; what it could not deliver (the session had ended, or has no inbox) waits here and is flagged by pm show. Without ids, this session's open requests ($CLAUDE_CODE_SESSION_ID); with ids, those, closed ones too.",
					args: []arg{
						{dest: "ids", metavar: []string{"ID"}, nargs: "*", kind: value, help: "a decision need, action or review (default: this session's)"},
					},
				},
				{
					name: "add",
					help: "the owner's answer to an open need, at a shell, as a reply on the site writes; the answer with --text",
					store: &storeCommand{
						usage: "reply add ID (--text TEXT | --text-file FILE)",
						about: "The owner's answer to an open need, at a shell: a reply, as a reply on the site writes. The need stays open; the session that raised it reads the reply and records it, which closes the need.",
						flags: func(fs *pflag.FlagSet) { textFlags(fs, "the answer") },
						args:  1,
						run:   replyAdd,
					},
				},
			},
		},
		{
			name:    "doc",
			help:    "free-form dated docs",
			subDest: "sub",
			subs: []*command{
				{
					name:   "new",
					help:   "create records/docs/<today>-<slug>.md; body with --text",
					groups: []bool{true, false},
					args: []arg{
						{dest: "slug", required: true, kind: value, help: "what the doc is for, lowercase words joined by '-'"},
						{flags: []string{"--title"}, dest: "title", required: true, kind: value},
						{flags: []string{"--bead"}, dest: "bead", metavar: []string{"ID"}, kind: value, group: 1, help: "the sprint or task the doc belongs to"},
						{flags: []string{"--project"}, dest: "project", metavar: []string{"NAME"}, kind: value, group: 1, help: "the project the doc belongs to"},
						{flags: []string{"--text-file"}, dest: "text_file", metavar: []string{"PATH"}, kind: value, group: 2, help: "required: the doc's Markdown body; read from PATH, or with - from stdin as a quoted heredoc: --text-file - <<'EOF' … EOF"},
						{flags: []string{"--text"}, dest: "text", kind: value, def: "", group: 2, help: "the same body inline, for one plain line only; several lines, backticks, $ or quotes go in --text-file"},
					},
				},
			},
		},
		{
			name:    "design",
			help:    "design pages",
			subDest: "sub",
			subs: []*command{
				{
					name:        "new",
					help:        "create records/design/<slug>.md with every template section",
					description: "Create a design page with every section of the template, each with its prompt line and \"None yet.\"; then edit it by hand. A page covers one area and holds its final state; decisions and plans go in the project or sprint record. Put no date in the slug. When a page grows to cover several areas, split it into sub pages and keep a short summary per area linking them.",
					args: []arg{
						{dest: "slug", required: true, kind: value, help: "what the design is, lowercase words joined by '-'"},
						{flags: []string{"--title"}, dest: "title", required: true, kind: value},
						{flags: []string{"--project"}, dest: "project", metavar: []string{"NAME"}, required: true, kind: value, help: "the project the design belongs to"},
					},
				},
			},
		},
		{
			name:    "postmortem",
			help:    "incident postmortems",
			subDest: "sub",
			subs: []*command{
				{
					name:        "new",
					help:        "create records/postmortems/<today>-<slug>.md with every template section",
					description: "Create a postmortem with every section of the template, each with its prompt line and \"None yet.\"; then write it by hand. Due for an incident that cost more than a day, or broke other sessions or the owner's view. Write it once the incident is fixed.",
					groups:      []bool{true},
					args: []arg{
						{dest: "slug", required: true, kind: value, help: "what broke, lowercase words joined by '-'"},
						{flags: []string{"--title"}, dest: "title", required: true, kind: value},
						{flags: []string{"--sprint"}, dest: "sprint", metavar: []string{"ID"}, kind: value, group: 1, help: "the sprint the incident hit, open or closed"},
						{flags: []string{"--project"}, dest: "project", metavar: []string{"NAME"}, kind: value, group: 1, help: "the project the incident hit"},
					},
				},
			},
		},
		{
			name:    "project",
			help:    "projects",
			subDest: "sub",
			subs: []*command{
				{
					name:        "open",
					help:        "create a project epic and record; Goal with --text",
					description: "Create a project epic and its record, with the Goal from --text. The owner confirms the goal in their own words before you open the project.",
					groups:      []bool{false},
					args: []arg{
						{dest: "name", required: true, kind: value},
						{flags: []string{"--title"}, dest: "title", required: true, kind: value},
						{flags: []string{"--text-file"}, dest: "text_file", metavar: []string{"PATH"}, kind: value, group: 1, help: "required: the project's Goal; read from PATH, or with - from stdin as a quoted heredoc: --text-file - <<'EOF' … EOF"},
						{flags: []string{"--text"}, dest: "text", kind: value, def: "", group: 1, help: "the same body inline, for one plain line only; several lines, backticks, $ or quotes go in --text-file"},
					},
				},
				{
					name:        "close",
					help:        "close a project epic at the committed records",
					description: "Close a project epic at the committed records. Close every sprint first. Write the record's '## Outcome' first, by hand: the results against the goal in numbers, what was learned, what was retired, and links to the sprints' delivery reports.",
					args: []arg{
						{dest: "name", required: true, kind: value},
					},
				},
			},
		},
		{
			name:    "sprint",
			help:    "sprints",
			subDest: "sub",
			subs: []*command{
				{
					name:   "open",
					help:   "create a sprint epic and record; frame with --text ('## Goal', '## Scope' with **In:**/**Out:**, '## Done when')",
					groups: []bool{false},
					args: []arg{
						{dest: "project", required: true, kind: value, help: "project record name, e.g. pm-harness"},
						{flags: []string{"--title"}, dest: "title", required: true, kind: value},
						{flags: []string{"--text-file"}, dest: "text_file", metavar: []string{"PATH"}, kind: value, group: 1, help: "required: the frame, '## Goal', '## Scope' and '## Done when' sections; read from PATH, or with - from stdin as a quoted heredoc: --text-file - <<'EOF' … EOF"},
						{flags: []string{"--text"}, dest: "text", kind: value, def: "", group: 1, help: "the same body inline, for one plain line only; several lines, backticks, $ or quotes go in --text-file"},
					},
				},
				{
					name:        "close",
					help:        "close a sprint epic once its report is written, every task is closed and each PR review is closed as merged",
					description: "Close a sprint epic at the committed records. Refuses until the Delivery report is written and every task is closed, and each PR review naming the sprint is closed with pm action done <id> --reason \"merged as <sha>\" once its PR is on main; it does not ask GitHub. With reviews, it stamps 'Merged as <sha> (PR #N).' into the Outcome after the verdict and commits it on the records branch; the close reason names the records commit. A PR an agent merged has no review: --merged SHA [--pr URL] gives its merge commit, which must be on the remote's main branch (pm fetches it first), and stamps the same line ('Merged as <sha>.' without --pr); it refuses a sprint that holds a PR review, whose close stamps the merge instead. Only the Outcome's first paragraph becomes the close reason.",
					args: []arg{
						{dest: "sprint_id", required: true, kind: value, help: "the sprint's id"},
						{flags: []string{"--merged"}, dest: "merged", metavar: []string{"SHA"}, kind: value, help: "the merge commit of the sprint's PR, merged with no PR review: a commit on the remote's main branch"},
						{flags: []string{"--pr"}, dest: "pr", metavar: []string{"URL"}, kind: value, help: "with --merged: the merged pull request, named in the stamp"},
					},
				},
				{
					name:        "edit",
					help:        "rename an open sprint; reason with --text",
					description: "Give an open sprint a new title: its work-store title, which keeps its 'Sprint <n>: ' prefix, and its record's title header, in one records commit that also adds the rename as a source=agent decision to the sprint's Decisions. The work store is written first; when the records step fails, the old title is put back. The reason (--text) states why, on at least two lines. Refuses a closed sprint, an unchanged title, and a title that carries its own 'Sprint <n>: ' prefix. A scope change is still a sprint decision.",
					groups:      []bool{false},
					args: []arg{
						{dest: "sprint_id", required: true, kind: value, help: "the sprint's id"},
						{flags: []string{"--title"}, dest: "title", required: true, kind: value, help: "the new title, without 'Sprint <n>: '"},
						{flags: []string{"--text-file"}, dest: "text_file", metavar: []string{"PATH"}, kind: value, group: 1, help: "required: why the title changes, on at least two lines; read from PATH, or with - from stdin as a quoted heredoc: --text-file - <<'EOF' … EOF"},
						{flags: []string{"--text"}, dest: "text", kind: value, def: "", group: 1, help: "the same body inline, for one plain line only; several lines, backticks, $ or quotes go in --text-file"},
					},
				},
				{
					name:        "move",
					help:        "move an open sprint, with its tasks, frame, decisions and findings, to another open project; reason with --text",
					description: "Move an open sprint to another open project. Its id stays, so its tasks, needs and holders stay as they are and the old id shows the sprint in its new place; it takes the project's next sprint number, and its record moves to records/sprints/<project>-<number>.md. The work store is written first: the new parent and number, and a move note on the sprint that keeps its old project and number, so the old project never reuses that number and the old record path still finds the record. Then one records commit renames the record and adds the move as a source=agent decision to both projects' records. If the records step fails or is cut short, run the same command again: it finds the move in the work store and writes the records step alone, with the reason and the date (UTC) the move note holds, so a rerun on a clone whose records have not yet synced another clone's finished step writes that same step, which the records sync then drops. The reason (--text) states why, on at least two lines. Refuses a closed sprint, a closed project, the project the sprint is in, and a move to another project while the last move's records step has not run.",
					groups:      []bool{false},
					args: []arg{
						{dest: "sprint_id", required: true, kind: value, help: "the sprint's id"},
						{flags: []string{"--to"}, dest: "to", metavar: []string{"PROJECT"}, required: true, kind: value, help: "the open project the sprint moves to, by record name, e.g. pm-harness"},
						{flags: []string{"--text-file"}, dest: "text_file", metavar: []string{"PATH"}, kind: value, group: 1, help: "required: why the sprint moves, on at least two lines; read from PATH, or with - from stdin as a quoted heredoc: --text-file - <<'EOF' … EOF"},
						{flags: []string{"--text"}, dest: "text", kind: value, def: "", group: 1, help: "the same body inline, for one plain line only; several lines, backticks, $ or quotes go in --text-file"},
					},
				},
			},
		},
		{
			name:    "task",
			help:    "tasks inside open sprints",
			subDest: "sub",
			subs: []*command{
				{
					name:        "add",
					help:        "create a task in an open sprint, or a sub-task (--parent); description with --text (optional)",
					description: "Create a task in an open sprint. pm task add --parent TASK --title TITLE, in place of --sprint, adds a sub-task under the open task TASK instead (pm task add --parent TASK --help).",
					groups:      []bool{false},
					args: []arg{
						{flags: []string{"--sprint"}, dest: "sprint", metavar: []string{"ID"}, required: true, kind: value, help: "the open sprint's id"},
						{flags: []string{"--title"}, dest: "title", required: true, kind: value},
						{flags: []string{"--text-file"}, dest: "text_file", metavar: []string{"PATH"}, kind: value, group: 1, help: "optional: the task's description; read from PATH, or with - from stdin as a quoted heredoc: --text-file - <<'EOF' … EOF"},
						{flags: []string{"--text"}, dest: "text", kind: value, def: "", group: 1, help: "the same body inline, for one plain line only; several lines, backticks, $ or quotes go in --text-file"},
					},
				},
				{
					name:        "close",
					help:        "close a task with a reason naming its commit, or drop it",
					description: "Close a task as done. The reason ends with what holds the work: '(commit <hash>)' for HEAD if it was committed after the task started, else the --commit given: a commit of this repo, OWNER/REPO@SHA for another repo's commit, or a PR URL, the last two resolved with gh; one that does not resolve, or a PR closed without merging, is refused. Without --commit, a warning when HEAD is older than the task or the working tree has uncommitted changes. --dropped closes it as not done instead (resolution dismissed, shown as dropped and left out of a sprint's task counts), with a required --reason and no commit; a task it blocks no longer waits on it. Refuses a task that another live session holds, as pm task claim does; that session closes it or releases it with pm task release.",
					args: []arg{
						{dest: "task_id", required: true, kind: value, help: "the task's id"},
						{flags: []string{"--reason"}, dest: "reason", kind: value, help: "what was done (default: Done); with --dropped, required: why it is not done"},
						{flags: []string{"--commit"}, dest: "commit", metavar: []string{"REF"}, kind: value, help: "what holds the work: a commit of this repo, OWNER/REPO@SHA or a PR URL (default: HEAD, if newer than the task)"},
						{flags: []string{"--dropped"}, dest: "dropped", kind: flagTrue, help: "close it as not done: no commit, a reason required"},
					},
				},
				{
					name:        "claim",
					help:        "claim a task for this agent session",
					description: "Claim an open task: make the session (--session, else $CLAUDE_CODE_SESSION_ID, else $CODEX_THREAD_ID) its holder, with the time. Refuses when another live session holds it: one whose transcript was written in the last 30 minutes. A subagent shares its session's id, so it may claim what its session holds. Refuses in the main checkout, since agents change code only in a worktree of their own, and says how to make one.",
					args: []arg{
						{dest: "task_id", required: true, kind: value, help: "the task's id"},
						{flags: []string{"--session"}, dest: "session", metavar: []string{"ID"}, kind: value, help: "the session to record; default: this session's id from the environment"},
					},
				},
				{
					name:        "move",
					help:        "move a task to another open sprint, or into a sprint from directly under a project; reason with --text",
					description: "Move an open task to another open sprint and record the scope change as a source=agent decision in the sprint it leaves. A task filed directly under a project (the site's Not in a sprint) moves into one of that project's open sprints, and the decision records the scope added in the sprint it joins. The reason (--text) states why, on at least two lines.",
					groups:      []bool{false},
					args: []arg{
						{dest: "task_id", required: true, kind: value, help: "the task's id"},
						{flags: []string{"--to"}, dest: "to", metavar: []string{"SPRINT_ID"}, required: true, kind: value, help: "the open sprint the task moves to"},
						{flags: []string{"--text-file"}, dest: "text_file", metavar: []string{"PATH"}, kind: value, group: 1, help: "required: why the task moves, on at least two lines; read from PATH, or with - from stdin as a quoted heredoc: --text-file - <<'EOF' … EOF"},
						{flags: []string{"--text"}, dest: "text", kind: value, def: "", group: 1, help: "the same body inline, for one plain line only; several lines, backticks, $ or quotes go in --text-file"},
					},
				},
				{
					name: "ready",
					help: "list the tasks an agent can claim now",
					store: &storeCommand{
						usage: "task ready [--sprint ID] [--json]",
						about: "The tasks an agent can claim now: open, under open ancestors, held by no live session, with no open blocker of their own or an ancestor's. By sprint number, then id; tasks directly under a project last. A task whose holder is not live is ready and marked a stale holder; pm task claim takes it over.",
						flags: func(fs *pflag.FlagSet) {
							fs.String("sprint", "", "only the tasks under this sprint")
							fs.Bool("json", false, "the ready tasks as a JSON array of items")
						},
						run: taskReady,
					},
				},
				{
					name: "edit",
					help: "set a task's title or description; description with --text",
					store: &storeCommand{
						usage: "task edit ID [--title TITLE] [--text TEXT | --text-file FILE]",
						about: "Set a task's title, its description (the body: --text, or --text-file - <<'EOF' … EOF), or both. A scope change is still pm task move or a sprint decision.",
						flags: func(fs *pflag.FlagSet) {
							fs.String("title", "", "the new title")
							textFlags(fs, "the new description")
						},
						args: 1,
						run:  taskEdit,
					},
				},
				{
					name: "release",
					help: "clear the holder of a task this session holds",
					store: &storeCommand{
						usage: "task release ID",
						about: "Clear the holder of a task this session ($CLAUDE_CODE_SESSION_ID, else $CODEX_THREAD_ID) holds.",
						args:  1,
						run:   taskRelease,
					},
				},
			},
		},
		{
			name:    "dep",
			help:    "dependencies: what an item waits on",
			subDest: "sub",
			subs: []*command{
				{
					name: "add",
					help: "make an item, and every task under it, wait until a blocker closes",
					store: &storeCommand{
						usage: "dep add ID --on BLOCKER",
						about: "BLOCKER blocks ID: ID, and every task under it, waits until BLOCKER closes. Refuses a blocker that does not exist and a cycle over blocked_by, ancestors included.",
						flags: onFlag,
						args:  1,
						run:   func(c *storeCall) error { return dep(c, true) },
					},
				},
				{
					name: "rm",
					help: "remove a blocker",
					store: &storeCommand{
						usage: "dep rm ID --on BLOCKER",
						about: "BLOCKER no longer blocks ID.",
						flags: onFlag,
						args:  1,
						run:   func(c *storeCall) error { return dep(c, false) },
					},
				},
			},
		},
		{
			name:    "comment",
			help:    "notes on items",
			subDest: "sub",
			subs: []*command{
				{
					name: "add",
					help: "add a note to an item; the note with --text",
					store: &storeCommand{
						usage: "comment add ID (--text TEXT | --text-file FILE)",
						about: "Add a note to an item: --text, or --text-file - <<'EOF' … EOF. Its author is this session, or owner when no session runs the command.",
						flags: func(fs *pflag.FlagSet) { textFlags(fs, "the note") },
						args:  1,
						run:   commentAdd,
					},
				},
			},
		},
		{
			name:    "record",
			help:    "records on the served site",
			subDest: "sub",
			subs: []*command{
				{
					name:        "link",
					help:        "print a record's page URL on the served site; the only link to give for a record",
					description: "Print the URL of a record's page on the site the pm service serves on localhost:$PORT (default: the installed service's port, else the port in .pm/config.toml), printed with the repo's site_url instead when .pm/config.toml sets one; --local prints the localhost URL even then, for a check on this machine that the public site's login would stop (a headless browser). The pm service's pages follow the records within 10 s and state their data's age, so no render step is needed. Fails with the command that fixes it when nothing serves there, or when what answers is not the pm service for this store.",
					args: []arg{
						{dest: "target", required: true, kind: value, help: "a sprint or project id, a project name, a design slug, or a record path (records/<…>.md; records/ and .md optional)"},
						{flags: []string{"--local"}, dest: "local", kind: flagTrue, help: "print http://127.0.0.1:<port>/…, the URL the service answers on here, even when site_url is set"},
					},
				},
			},
		},
		{
			name: "check",
			help: "check that every record renders with the work store, writing nothing; the check before a commit, and the one pm commit runs",
		},
		{
			name: "sync",
			help: "sync the work store with the remote now, as the pm service does every 10 minutes",
			store: &storeCommand{
				usage: "sync",
				about: "Sync the work store with the repo's remote now: the pm service pulls, resolves conflicts by the merge rules and pushes, as it does every 10 minutes. A conflict no rule settles fails, names the item and field, and leaves the store as it was.",
				run:   syncNow,
			},
		},
		{
			name:        "service",
			help:        "the pm service: one background process per clone serves the site, syncs the work store and pushes the records branch every 10 minutes",
			description: "The pm service: one supervised background process per clone, `pm service run` in the main checkout. It\nholds the work store, which every pm command reaches through its socket, .pm/run/work.sock, and serves the\nsite live from the records store and the work store (a page is at most 10 s behind them), delivers\nthe owner's site replies and reviewed PRs' merges (GitHub polled every 60 s) into the sessions that\nraised them, and syncs the work store, pushes today's summary and the records branch every\n600 s, the first push 600 s after it starts. Sessions push neither.\n\nSupervisor: it starts the service at login and again after a crash. A launchd agent with KeepAlive on\nmacOS (~/Library/LaunchAgents/local.pm.<dir>.<hash>.plist); a systemd user service with Restart=always\non Linux ($XDG_CONFIG_HOME/systemd/user/local.pm.<dir>.<hash>.service, ~/.config by default). On a\nmachine with neither there is no service, and install refuses.\n\nUnit: runs the installed pm (`<bin dir>/pm service run`, the bin dir $PM_BIN_DIR, else ~/.local/bin; pm init\ncopies pm there) in the main checkout, with the PATH install ran with (git must be on it, and uv\nfor a Python pin) and PORT. The installed pm runs the version the main checkout pins: another Go version's release binary, a Python version\nthrough uv.\n\nPort: $PORT, else the installed unit's port, else `port` in .pm/config.toml. Installing again keeps the unit's\nport. A second clone of the repo on this machine needs its own: PORT=<n> pm service install.\n\nState and logs, in <main checkout>/.pm/run/ (never committed): service.log (pm service logs), push.json\n(each push step's last outcome), push.log (a line per step per run), push.lock (one push at a time).\n\nHealth (pm service status, pm where): the supervisor holds the unit, and the site answers on its port\nwith X-PM-Store naming this clone's store and X-PM-Version naming this pm's build. Status also\nflags a push step that failed or has not succeeded for 1800 s.\n\nStale build: every 1 s the service rereads the pin in .pm/config.toml; once it pins another version (a\npull after pm upgrade) the service exits and the supervisor starts the installed pm again, which runs the new\npin. A service that answers on another build than the running pm, the pinned one, is stale in pm where, pm\nservice status and pm doctor. Fix: pm init (it copies this pm into the bin dir when another is there, then\ninstalls the service); when the installed pm already is this build, pm service install or pm service restart.\nA unit that runs another program than the installed pm (Python pm's unit runs the pm uv tool's interpreter)\nis stale there too, and session start leaves it: pm service install rewrites it.\n\nAgents: pm prime carries pm where's service line and push state, and pm show warns when a push needs\nattention. When the service is down, run pm service restart; if that fails, raise an action for the\nowner (pm action need) and add a bug task (pm task add). pm init installs the service; pm service stop\nstops it and keeps it stopped, at login and at session start, until pm service restart or pm init;\npm uninstall stops it and removes its unit.",
			raw:         true,
			subDest:     "sub",
			subs: []*command{
				{
					name:        "install",
					help:        "install and start this clone's service (launchd on macOS, systemd on Linux), or update it; a no-op once installed and current",
					description: "Install the pm service under the machine's supervisor, which starts it at login and restarts it after a crash: a launchd agent with KeepAlive on macOS, a systemd user service on Linux; refused on a machine with neither. The unit runs the installed pm (refused unless it is this pm's build: run pm init) with the current PATH (git must be on it) and serves on $PORT, else the port it was installed with, else the port in .pm/config.toml; give a second clone of the repo its own port once with PORT=<n> pm service install. Installing again rewrites a changed unit and restarts a service on another build. It waits 15 s for the site to answer for this store and fails when it does not (another clone's service on the port, say). pm init runs it.",
				},
				{
					name:        "status",
					help:        "whether the service is up and its site answers for this store, and its pushes; non-zero when either needs attention",
					description: "Print the service's health line (as pm where does), each push step's last outcome, every push problem and the log's path. Up means the supervisor holds the unit and the site answers on its port with X-PM-Store naming this store and X-PM-Version naming this pm's build; a push problem is a step whose last run failed or that has not succeeded for 1800 s. Exits non-zero when the service is down, stale or a push needs attention; each line names the command that fixes it.",
				},
				{
					name:        "restart",
					help:        "restart the service and wait for its site to answer; when it fails, raise an action for the owner and add a bug task",
					description: "Restart the installed service, or load it when the supervisor does not hold it (enabling it first when pm service stop stopped it), then wait 15 s for the site to answer for this store. The fix when pm where shows the service down. Refused when it is not installed (run pm service install); when it fails, read pm service logs, raise an action for the owner (pm action need) and add a bug task (pm task add).",
				},
				{
					name:        "stop",
					help:        "stop this clone's service and keep it stopped, at login and at session start, until pm service restart or pm init",
					description: "Disable the service at its supervisor and stop it (systemd: systemctl --user disable --now; launchd: launchctl disable, then bootout), then wait 15 s until neither the work store's socket nor the site answers, and say so; fails naming what still answers (a pm service run started by hand). The supervisor's disabled state is the record of the stop: the supervisor starts it neither at login nor after a crash, session start (pm init --session-start) leaves it stopped and its state names pm service restart, and pm where, pm service status and pm doctor show it stopped. Meanwhile every pm command that reads or writes work items refuses; pm uninstall still checks for unsynced work, through a pm service run of its own. A typed pm init or pm service restart enables and starts it again. Refused when it is not installed.",
				},
				{
					name:        "logs",
					help:        "print the end of the service's log, <main checkout>/.pm/run/service.log",
					description: "Print the end of <main checkout>/.pm/run/service.log, where the service writes stdout and stderr: a line per request, refresh and reply write with its timings, and any error that stopped it. The push's own log is .pm/run/push.log beside it.",
					args: []arg{
						{flags: []string{"-n", "--lines"}, dest: "lines", kind: value, isInt: true, def: "50", help: "how many lines (default 50)"},
					},
				},
				{
					name:        "run",
					help:        "the service's process, run by its supervisor: serve the site on localhost:$PORT and push every 10 minutes",
					description: "Serve the site on localhost:$PORT (default: port in .pm/config.toml): a page is at most 10 s behind the records and the work store and states its data's age; an open page never reloads itself but shows within ~10 s that newer data exists, loaded on reload. Every 10 minutes, the first 10 after start, run pm push. Exits once .pm/config.toml pins another pm version, so the supervisor starts the installed pm again, which runs the new pin. The supervisor runs it; run it by hand only to debug, or on another PORT.",
				},
			},
		},
		{
			name: "doctor",
			help: "compare every managed piece with what this pm writes, and the clone and worktree with what pm init makes; report each difference, exit 1 on any; and name a pm release newer than the pin, with its notes (or say the release list cannot be read), which leaves the exit code alone",
		},
		{
			name: "upgrade",
			help: "move the pin in .pm/config.toml to this pm and rewrite every managed piece as it writes them (the fix pm doctor names for a changed one), removing the pre-package harness's and the ones an earlier pm wrote and this one retired (the records/ copy and guard workflows, the pre-commit section); writes files and prints the commit to make, never commits",
			args: []arg{
				{flags: []string{"--to"}, dest: "to", metavar: []string{"X"}, kind: value, help: "the version to move to: the running pm's (the default), or another, which the installed pm launches to make the move; without it pm upgrade refuses a pin newer than the running pm (--to the pin rewrites the pieces at it)"},
			},
		},
		{
			name: "uninstall",
			help: "remove pm's pieces from this worktree (hook entries, .gitignore block, pm's git hooks in .pm/hooks, .pm/, an earlier pm's workflows) and the clone's and machine's setup (the store checkout, records/ links and an earlier pm's sparse checkouts in every worktree, the pm service, this clone's Codex writable_roots, pm's lines in .git/info/exclude, the work store (refused while it holds what the remote lacks; with the pm service stopped, it runs pm service run for that check)); keeps the records branch and the remote's work store (refs/pm/work); never commits",
		},
		{
			name:        "init",
			help:        "install pm: the repo's files on first install (--site-url sets the public site link), then this clone, this worktree and the pm service (PORT=<n> sets its port, and on a first install the config's; PORT=<n> pm service install moves only the service's); session start runs it; never commits on the code branch",
			description: "Install pm, doing only what is missing. Repo, on first install only (no .pm/config.toml yet): write .pm/ (config.toml, README.md, .gitignore), pm's hook entries in .claude/settings.json and .codex/hooks.json, pm's git hook .pm/hooks/post-checkout and pm's .gitignore lines; create the records branch with an empty store and push it when the remote has none; print the commit to make. After that pm init leaves the repo's files alone: pm doctor reports a changed or missing piece and pm upgrade rewrites it. Clone and worktree, every run: copy this pm into the bin dir when another is there, take out Beads' hook entries, CLAUDE.md block and hooks path, install the git hooks (core.hooksPath .pm/hooks), list .pm/store/ and .pm/run/ in .git/info/exclude, check out the records store at <main checkout>/.pm/store/records if missing, link this worktree's records/ to it and turn off the sparse checkout an earlier pm set once HEAD and the index track no records/, add the clone's .git and the stores to the writable roots of $CODEX_HOME/config.toml and the store to this worktree's .claude/settings.local.json, then install the pm service (pm service install) and have it attach the work store, which only the service opens (cloned from the remote's refs/pm/work, or created and pushed). Session start runs it in every worktree, and starts an installed service that does not answer; once all is set up it prints 'already set up'. Refuses a core.hooksPath other than .pm/hooks or Beads' .beads/hooks. Site port: a new repo's config gets $PORT, else the first free port from 8000 up that no pm service unit on this machine names; a clone's service serves on $PORT, else its unit's port, else the config's. Refuses, writing nothing, when another server holds that port, and names a free one: PORT=<n> pm init. .pm/config.toml, tracked: version (the pm every session must run; pm upgrade moves it), remote and main_branch (origin and its default branch), port (the site port) and site_url (--site-url).",
			groups:      []bool{false},
			args: []arg{
				{flags: []string{"--session-start"}, dest: "session_start", kind: flagTrue, help: "what session start runs, without $PORT: install the pm service when it is missing and start an installed one that does not answer, so a session in a clone brings its service up; one pm service stop stopped is left stopped and reported (pm service restart starts it); a running one, a stale one included, is left as it is and reported (pm service restart restarts it)"},
				{flags: []string{"--site-url"}, dest: "site_url", metavar: []string{"URL"}, kind: value, help: "the site's public base URL (a tunnel to the pm service), written to site_url in .pm/config.toml for you to commit: every link pm prints (pm record link, pm show, pm where) uses it instead of http://localhost:<port>, and the site accepts the owner's replies from its host besides localhost; '' clears it"},
				{flags: []string{"--import-bd"}, dest: "import_bd", metavar: []string{"FILE"}, kind: value, group: 1, help: "only import the bd export in FILE into this clone's work store, through the pm service, as one commit; refuses a store that holds any item, and an export it cannot map; nothing else runs"},
				{flags: []string{"--import"}, dest: "import", metavar: []string{"FILE"}, kind: value, group: 1, help: "only import the pm export in FILE (what pm export printed in another clone) into this clone's work store, ids kept, as --import-bd does"},
			},
		},
		{
			name:        "push",
			help:        "sync the work store, summarize today and push the records branch; what the pm service runs every 10 minutes, not a session command",
			description: "Sync the work store with the remote, then summarize today (pm day summarize only when today's activity changed), then push the records branch when the store is ahead of <remote>/records: fetch, rebase onto it if it moved (under the store lock; a rebase that stops is aborted, leaving the store as it was), push. Each step has a 120s timeout; a second run while one holds the lock exits at once. Each step's outcome goes to <clone>/.pm/run/push.json (read by pm show, pm where, pm service status and the site) and <clone>/.pm/run/push.log, a line per step.",
		},
		{
			name: "where",
			help: "list every location with its state: the store, this checkout, the work store, the hooks, the Codex sandbox roots, the pm service, the last push, and the site; 'pm where records' prints only the store's path",
			args: []arg{
				{dest: "what", nargs: "?", choices: []string{"records"}, kind: value, help: "print only this location's path"},
			},
		},
		{
			name:        "clean",
			help:        "list each worktree with keep or remove and the reason; --apply removes the agent worktrees (.claude/worktrees/) that no live session owns and whose work is saved",
			description: "List every worktree of this clone with keep or remove and the reason; a dry run unless --apply. Only an agent worktree, under <main checkout>/.claude/worktrees/, is ever removed; never the main checkout, the records store (.pm/store/records) or the worktree pm clean runs in. Owned, so kept: a worktree Claude Code locked for a live process (`claude <kind> <name> (pid N start T)` in .git/worktrees/<name>/locked; the lock is stale when pid N is gone or, on Linux, started at another time than T; on macOS a live pid owns it), a worktree someone else locked, and one a live session used: an entry of a Claude Code transcript (<config dir>/projects/) written in the last 30 minutes whose directory is in the worktree or whose tool call names the worktree's path (a subagent runs in its parent's directory). Removed, when clean (no change, no untracked file; its ignored files go with it, as git worktree remove deletes them) and holding no other worktree: one whose commits are on <remote>/<main branch> (its branch deleted too), one whose branch a PR merged into <main branch> took at its tip (a squash merge, asked of gh; kept when gh is missing or fails; its branch deleted too), and one whose branch has nothing beyond where it was pushed: its upstream, unless that is the main branch, else <remote>/<branch> (its branch kept, as it is not merged). Everything else is kept: uncommitted changes, commits never pushed. The branches compare with <remote>/<main branch> as last fetched: run git fetch first. --apply takes a stale lock off, runs git worktree remove (never --force, so git refuses a worktree that changed meanwhile; for a worktree whose directory is gone it drops git's entry), and deletes a merged branch at the tip it judged. It refuses, removing nothing, when a recent transcript cannot be read. Exits 1 when a removal failed, naming why on its line.",
			args: []arg{
				{flags: []string{"--apply"}, dest: "apply", kind: flagTrue, help: "remove the worktrees the dry run lists as remove; without it pm clean changes nothing"},
			},
		},
		{
			name:        "export",
			help:        "print every item of the work store, one JSON object per line, ordered by id; pm init --import reads it in another clone",
			description: "Print every item of this clone's work store, one JSON object per line, ordered by id. pm init --import FILE in another clone reads it, to move a project's items into a new repo's store.",
			args: []arg{
				{flags: []string{"--store"}, dest: "store", metavar: []string{"DIR"}, kind: value, help: "the work store at DIR (<main checkout>/.pm/store/work), read through the pm service of the clone DIR belongs to, without the repo's config"},
			},
		},
		{
			name: "version",
			help: "print this pm's version, dev for an untagged build; outside any repo too",
		},
		{
			name:   "prime",
			help:   fmt.Sprintf("pm's rules, then pm init, pm where and pm show: the context a session starts with; the SessionStart hooks run --rules 1 to --rules %d and --state, and an agent may run it by hand", len(hooks.Starts)),
			groups: []bool{false},
			args: []arg{
				{flags: []string{"--rules"}, dest: "part", metavar: []string{"N"}, choices: chunkNumbers(), kind: value, isInt: true, group: 1, help: "only chunk N of the rules and the command list, under a title naming its sections: one hook each on SessionStart and SubagentStart, since Claude Code passes a hook's text inline only up to 10,000 characters"},
				{flags: []string{"--state"}, dest: "part", kind: flagConst, constant: "state", group: 1, help: "only pm init, pm where and pm show, cut at a line to 10,000 characters: the last SessionStart hook"},
				{flags: []string{"--subagent"}, dest: "part", kind: flagConst, constant: "subagent", group: 1, help: "only pm's git rule for agents, one line: the last SubagentStart hook, beside the rules chunks"},
				{flags: []string{"--hook-json"}, dest: "hook_json", kind: flagTrue, help: "read the SessionStart or SubagentStart input on stdin and print the hook's JSON envelope, as Claude Code and Codex read it"},
			},
		},
		{
			name:    "hook",
			help:    "what a runtime hook runs: the hook input JSON on stdin; installs nothing",
			subDest: "sub",
			subs: []*command{
				{
					name: "stop",
					help: "Stop: block once while records this session's tool calls name are uncommitted in the store, leaving out a record that only a still-running subagent's prompt names",
				},
				{
					name: "owner-request",
					help: "Stop: block once while the reply asks the owner for something no open need or action of this session covers",
				},
				{
					name: "git-post-checkout",
					help: "git post-checkout (.pm/hooks/post-checkout): in a new worktree, run pm init's clone and worktree half, all but the pm service",
					args: []arg{
						{dest: "git_args", nargs: "*", kind: value, help: "the hook's arguments: previous HEAD, new HEAD, branch flag"},
					},
				},
				{
					name: "git-pre-commit",
					help: "retired: does nothing; an earlier pm's section in .pm/hooks/pre-commit runs it until pm upgrade removes that section",
				},
			},
		},
		{
			name:        "commit",
			help:        "commit your hand edits in the store, named by path, on the records branch",
			description: "Commit only the named records, so other sessions' uncommitted edits in the shared store are left alone. With no path, it lists what is uncommitted and commits nothing. A record uses only these fenced blocks: ::: decision, ::: result and ```mermaid; put a one-line reading under each diagram or large table. An image file (.svg, .png, .jpg, .webp) beside a record is named and committed like one; the site serves it next to the record's page.",
			args: []arg{
				{flags: []string{"-m", "--message"}, dest: "message", required: true, kind: value, help: "what the hand edit changed"},
				{dest: "paths", metavar: []string{"PATH"}, nargs: "*", kind: value, help: "a record you edited: records/<…>.md, a path in the store, relative to it from inside it, or relative to the store from anywhere (sprints/<…>.md)"},
			},
		},
	},
}
