// Code moved verbatim from the argparse tree in src/pm/cli.py (parser()): every command, its help, description and
// arguments, in the order Python defines them. A help text changes here and in cli.py together; the parity test
// compares every command's --help with Python pm's.

package cli

import (
	"fmt"
	"strconv"

	"github.com/Yeeef/yeeef-agents/pm/internal/hooks"
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
	epilog:      "pm runs the pm version the repo pins in .pm/config.toml: another Go version's release binary, a Python version through uv; pm where names the version running and why.",
	subDest:     "cmd",
	subs: []*command{
		{
			name:        "show",
			help:        "compact status of open projects for agents, level by level",
			description: "Project state, level by level. Without a flag, the top level: push failures, tasks other live sessions hold, the site, and per open project a line with each open owner request and undelivered reply. Each level names the command for the next: --project, then --sprint, then --record with --section.",
			args: []arg{
				{flags: []string{"--json"}, dest: "json", kind: flagTrue, help: "every level's data as one JSON object"},
				{flags: []string{"--project"}, dest: "project", metavar: []string{"NAME"}, kind: value, help: "one open project (its name or id): its goal, owner requests in full, feedback, open sprints with their tasks, and last decisions"},
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
					name: "add",
					help: "append a bullet to a sprint's Findings",
					args: []arg{
						{flags: []string{"--sprint"}, dest: "sprint", metavar: []string{"ID"}, required: true, kind: value, help: "the sprint's id"},
						{dest: "text", nargs: "+", required: true, kind: value},
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
					help:        "append an entry to the project's pm feedback doc; the text with --text",
					description: "When pm got in the way (a confusing refusal, a missing command, a rule that cost time), say once what happened and what would have helped. Appends a dated entry with this session's id to records/docs/<date of first use>-<project>-feedback.md, creating it on first use.",
					groups:      []bool{false},
					args: []arg{
						{flags: []string{"--project"}, dest: "project", metavar: []string{"NAME"}, required: true, kind: value, help: "the project the feedback doc belongs to"},
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
					description: "Close the need with the owner's answer and the reason, as no-decision; on a need the owner already closed, only the resolution no-decision and the reason (as a comment) are set, and no answer is needed. An answer that sets a rule is recorded with pm decision add --need instead. Answers that set no rule: a name, a port, which of two equal files. If you are not sure, record a decision. If setting the resolution fails, run it again; it does not repeat the reason.",
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
					description: "Close a sprint epic at the committed records. Refuses until the Delivery report is written and every task is closed, and each PR review naming the sprint is closed with pm action done <id> --reason \"merged as <sha>\" once its PR is on main; it does not ask GitHub. With reviews, it stamps 'Merged as <sha> (PR #N).' into the Outcome after the verdict and commits it on the records branch; the close reason names the records commit. Only the Outcome's first paragraph becomes the close reason.",
					args: []arg{
						{dest: "sprint_id", required: true, kind: value, help: "the sprint's id"},
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
					name:   "add",
					help:   "create a task in an open sprint; description with --text (optional)",
					groups: []bool{false},
					args: []arg{
						{flags: []string{"--sprint"}, dest: "sprint", metavar: []string{"ID"}, required: true, kind: value, help: "the open sprint's id"},
						{flags: []string{"--title"}, dest: "title", required: true, kind: value},
						{flags: []string{"--text-file"}, dest: "text_file", metavar: []string{"PATH"}, kind: value, group: 1, help: "optional: the task's description; read from PATH, or with - from stdin as a quoted heredoc: --text-file - <<'EOF' … EOF"},
						{flags: []string{"--text"}, dest: "text", kind: value, def: "", group: 1, help: "the same body inline, for one plain line only; several lines, backticks, $ or quotes go in --text-file"},
					},
				},
				{
					name:        "close",
					help:        "close a task with a reason naming its commit",
					description: "Close a task. The reason ends with '(commit <hash>)': HEAD if it was committed after the task started, or the commit given with --commit; with neither, a warning.",
					args: []arg{
						{dest: "task_id", required: true, kind: value, help: "the task's id"},
						{flags: []string{"--reason"}, dest: "reason", kind: value, help: "what was done (default: Done)"},
						{flags: []string{"--commit"}, dest: "commit", metavar: []string{"REF"}, kind: value, help: "the commit holding the work (default: HEAD, if newer than the task)"},
					},
				},
				{
					name:        "claim",
					help:        "claim a task for this agent session",
					description: "Claim an open task: make the session ($CLAUDE_CODE_SESSION_ID, else $CODEX_THREAD_ID) its holder, with the time. Refuses when another live session holds it: one whose transcript was written in the last 30 minutes. A subagent shares its session's id, so it may claim what its session holds. Refuses in the main checkout, since agents change code only in a worktree of their own, and says how to make one.",
					args: []arg{
						{dest: "task_id", required: true, kind: value, help: "the task's id"},
						{flags: []string{"--session"}, dest: "session", metavar: []string{"ID"}, kind: value, help: "the session to record when no session id is in the environment"},
					},
				},
				{
					name:        "move",
					help:        "move a task to another open sprint; reason with --text",
					description: "Move an open task to another open sprint and record the scope change as a source=agent decision in the sprint it leaves. The reason (--text) states why, on at least two lines.",
					groups:      []bool{false},
					args: []arg{
						{dest: "task_id", required: true, kind: value, help: "the task's id"},
						{flags: []string{"--to"}, dest: "to", metavar: []string{"SPRINT_ID"}, required: true, kind: value, help: "the open sprint the task moves to"},
						{flags: []string{"--text-file"}, dest: "text_file", metavar: []string{"PATH"}, kind: value, group: 1, help: "required: why the task moves, on at least two lines; read from PATH, or with - from stdin as a quoted heredoc: --text-file - <<'EOF' … EOF"},
						{flags: []string{"--text"}, dest: "text", kind: value, def: "", group: 1, help: "the same body inline, for one plain line only; several lines, backticks, $ or quotes go in --text-file"},
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
					description: "Print the URL of a record's page on the site the pm service serves on localhost:$PORT (default: port in .pm/config.toml), printed with the repo's site_url instead when .pm/config.toml sets one. The pm service's pages follow the records within 10 s and state their data's age, so no render step is needed. Fails with the command that fixes it when nothing serves there, or when what answers is not the pm service for this store.",
					args: []arg{
						{dest: "target", required: true, kind: value, help: "a sprint or project id, a project name, a design slug, or a record path (records/<…>.md; records/ and .md optional)"},
					},
				},
			},
		},
		{
			name: "check",
			help: "check that every record renders with the work store, writing nothing; the check before a commit, and the one pm commit runs",
		},
		{
			name:        "service",
			help:        "the pm service: one background process per clone serves the site, syncs the work store and pushes the records branch every 10 minutes",
			description: "The pm service: one supervised background process per clone, `pm service run` in the main checkout. It\nserves the site live from the records store and the work store (a page is at most 10 s behind them), delivers\nthe owner's site replies and reviewed PRs' merges (GitHub polled every 60 s) into the sessions that\nraised them, and syncs the work store, pushes today's summary and the records branch every\n600 s, the first push 600 s after it starts. Sessions push neither.\n\nSupervisor: it starts the service at login and again after a crash. A launchd agent with KeepAlive on\nmacOS (~/Library/LaunchAgents/local.pm.<dir>.<hash>.plist); a systemd user service with Restart=always\non Linux ($XDG_CONFIG_HOME/systemd/user/local.pm.<dir>.<hash>.service, ~/.config by default). On a\nmachine with neither there is no service, and install refuses.\n\nUnit: runs the pm uv tool's interpreter (`<tool python> -m pm.cli service run`; pm init installs the\ntool) in the main checkout, with the PATH install ran with (bd, git and uv must be on it) and PORT. The tool\nruns the version the main checkout pins, through uv when it is another.\n\nPort: $PORT, else the installed unit's port, else `port` in .pm/config.toml. Installing again keeps the unit's\nport. A second clone of the repo on this machine needs its own: PORT=<n> pm service install.\n\nState and logs, in <main checkout>/.pm/run/ (never committed): service.log (pm service logs), push.json\n(each push step's last outcome), push.log (a line per step per run), push.lock (one push at a time).\n\nHealth (pm service status, pm where): the supervisor holds the unit, and the site answers on its port\nwith X-PM-Store naming this clone's store and X-PM-Version naming this pm's build. Status also\nflags a push step that failed or has not succeeded for 1800 s.\n\nStale build: every 1 s the service rereads the pin in .pm/config.toml; once it pins another version (a\npull after pm upgrade) the service exits and the supervisor starts the pm uv tool again, which runs the new\npin. A service that answers on another build than the running pm, the pinned one, is stale in pm where, pm\nservice status and pm doctor. Fix: pm init (it installs the pm uv tool at this build unless the tool launched\nit, then the service); when the tool already runs this build, pm service install or pm service restart. A unit\nthat runs another interpreter than the pm uv tool's (a pin older than 0.1.2 wrote it to run its own tool) is\nstale there too, and session start leaves it: pm service install rewrites it.\n\nAgents: pm prime carries pm where's service line and push state, and pm show warns when a push needs\nattention. When the service is down, run pm service restart; if that fails, raise an action for the\nowner (pm action need) and add a bug task (pm task add). pm init installs the service; pm uninstall stops\nit and removes its unit.",
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
					description: "Restart the installed service, or load it when the supervisor does not hold it, then wait 15 s for the site to answer for this store. The fix when pm where shows the service down. Refused when it is not installed (run pm service install); when it fails, read pm service logs, raise an action for the owner (pm action need) and add a bug task (pm task add).",
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
					description: "Serve the site on localhost:$PORT (default: port in .pm/config.toml): a page is at most 10 s behind the records and the work store and states its data's age; an open page never reloads itself but shows within ~10 s that newer data exists, loaded on reload. Every 10 minutes, the first 10 after start, run pm push. Exits once .pm/config.toml pins another pm version, so the supervisor starts the pm uv tool again, which runs the new pin. The supervisor runs it; run it by hand only to debug, or on another PORT.",
				},
			},
		},
		{
			name: "doctor",
			help: "compare every managed piece with what this pm writes, and the clone and worktree with what pm init makes; report each difference, exit 1 on any",
		},
		{
			name: "upgrade",
			help: "move the pin in .pm/config.toml to this pm and rewrite every managed piece as it writes them (the fix pm doctor names for a changed one), removing the pre-package harness's; writes files and prints the commit to make, never commits",
			args: []arg{
				{flags: []string{"--to"}, dest: "to", metavar: []string{"X"}, kind: value, help: "the version to move to: the running pm's (the default), or another, which the pm uv tool runs to make the move; without it pm upgrade refuses a pin newer than the running pm (--to the pin rewrites the pieces at it)"},
			},
		},
		{
			name: "uninstall",
			help: "remove pm's pieces from this worktree (hook entries, workflows, .gitignore block, pm's git hooks in .pm/hooks, .pm/) and the clone's and machine's setup (the store checkout, records/ links and sparse checkouts in every worktree, the pm service, this clone's Codex writable_roots (uv's cache, shared by every clone, stays), pm's lines in .git/info/exclude); keeps the records branch, records/ on the main branch and Beads; never commits",
		},
		{
			name:        "init",
			help:        "install pm: the repo's files on first install (--site-url sets the public site link), then this clone, this worktree and the pm service (PORT=<n> sets its port, and on a first install the config's; PORT=<n> pm service install moves only the service's); session start runs it; never commits on the code branch",
			description: "Install pm, doing only what is missing. Repo, on first install only (no .pm/config.toml yet): write .pm/ (config.toml, README.md, .gitignore), pm's hook entries in .claude/settings.json and .codex/hooks.json, pm's git hooks .pm/hooks/post-checkout and pre-commit, the workflows .github/workflows/pm-records-{guard,copy}.yml and pm's .gitignore lines; create the records branch with an empty store and push it when the remote has none; print the commit to make. After that pm init leaves the repo's files alone: pm doctor reports a changed or missing piece and pm upgrade rewrites it. Clone and worktree, every run: copy this pm into the bin dir when another is there, attach the work store (cloned from the remote's refs/pm/work, or created and pushed), take out Beads' hook entries, CLAUDE.md block and hooks path, install the git hooks (core.hooksPath .pm/hooks), list .pm/store/ and .pm/run/ in .git/info/exclude, check out the records store at <main checkout>/.pm/store/records if missing, link this worktree's records/ to it and keep records/ out of its sparse checkout, add the clone's .git and the stores to the writable roots of $CODEX_HOME/config.toml and the store to this worktree's .claude/settings.local.json, then install the pm service (pm service install). Session start runs it in every worktree; once all is set up it prints 'already set up'. Refuses a core.hooksPath other than .pm/hooks or Beads' .beads/hooks. Site port: a new repo's config gets $PORT, else the first free port from 8000 up that no pm service unit on this machine names; a clone's service serves on $PORT, else its unit's port, else the config's. Refuses, writing nothing, when another server holds that port, and names a free one: PORT=<n> pm init. .pm/config.toml, tracked: version (the pm every session must run; pm upgrade moves it), remote and main_branch (origin and its default branch), port (the site port) and site_url (--site-url).",
			args: []arg{
				{flags: []string{"--session-start"}, dest: "session_start", kind: flagTrue, help: "what session start runs, without $PORT: install the pm service only when it is missing, and report an installed one that is stale or down instead of restarting it (pm service restart does)"},
				{flags: []string{"--site-url"}, dest: "site_url", metavar: []string{"URL"}, kind: value, help: "the site's public base URL (a tunnel to the pm service), written to site_url in .pm/config.toml for you to commit: every link pm prints (pm record link, pm show, pm where) uses it instead of http://localhost:<port>, and the site accepts the owner's replies from its host besides localhost; '' clears it"},
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
					help: "Stop: block once while records this session's tool calls name are uncommitted in the store",
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
					help: "git pre-commit (.pm/hooks/pre-commit): refuse staged records/ changes on a code branch, unless a merge is in progress",
				},
			},
		},
		{
			name:        "commit",
			help:        "commit your hand edits in the store, named by path, on the records branch",
			description: "Commit only the named records, so other sessions' uncommitted edits in the shared store are left alone. With no path, it lists what is uncommitted and commits nothing. A record uses only these fenced blocks: ::: decision, ::: result and ```mermaid; put a one-line reading under each diagram or large table.",
			args: []arg{
				{flags: []string{"-m", "--message"}, dest: "message", required: true, kind: value, help: "what the hand edit changed"},
				{dest: "paths", metavar: []string{"PATH"}, nargs: "*", kind: value, help: "a record you edited: records/<…>.md, a path in the store, or relative to it from inside it"},
			},
		},
	},
}
