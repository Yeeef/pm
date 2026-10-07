# pm rules

## Model
- Two layers hold a project: Beads is the work layer, and the Markdown records are the record layer. `pm` is the orchestration layer on top of both. The site shows both layers to the owner.
- Records point at Beads ids. Do not copy a status into a record.
- `records/` links to one store on the records branch. All sessions and worktrees write in it. Commit only the paths that you edited.
- Run `pm <noun> --help` for its commands and flags.
- Each rule has a reason. When a rule and its reason disagree in a new case, obey the reason.

| Object | In Beads | Record |
|---|---|---|
| Project: a standing goal of many sprints | epic | `records/projects/<name>.md` |
| Sprint: one goal; hours to days | child epic of the project | `records/sprints/<project>-<n>.md` |
| Task: one unit of work, one holder | task under the sprint | none |
| Need: what waits on the owner | issue labelled `human` | none |
| Decision | none | `::: decision` block in a project or sprint record |
| Design page: one area, final state | none | `records/design/<name>.md` |
| Doc: a dated result or explainer | none | `records/docs/<date>-<slug>.md` |
| Postmortem: a costly incident | none | `records/postmortems/<date>-<slug>.md` |
| Day page: generated from activity | none | nobody writes it |

| Record | You write by hand; `pm` writes | Generated: never write |
|---|---|---|
| Project | Goal, Design pages, Outcome; `pm`: Decisions | Progress, Docs, Postmortems |
| Sprint | Goal, Scope, Done when, Design pages, Delivery report; `pm`: Decisions, Findings | Progress, Decisions await you, Actions await you, Docs, Postmortems |
| Design page | Problem, Goals and non-goals, Constraints and key facts, Design, Alternatives considered, Prior art (optional), Open questions | none |
| Doc | All of it | none |
| Postmortem | Summary, Timeline, Cost, Root cause, What changed, What would have caught it earlier | none |

| Decisions and needs | Do |
|---|---|
| The owner must choose | `pm decision need` |
| The owner must do a step | `pm action need` |
| A PR needs review | `pm action need --pr` |
| An answer sets a rule | `pm decision add --need`, or `--confirmed` for an answer in chat |
| An answer sets no rule | `pm decision close`. If you are not sure, record a decision. |
| The action is done | `pm action done`, with the evidence in `--reason` |
| Decision level | Project if a later sprint must obey it, else sprint |
| Decision source | Agent, unless it answers a need or the owner confirmed it |

| Question | Where the state lives |
|---|---|
| What waits on the owner? | Open `human` issues: Decisions and Actions await you, in `pm show` |
| What constrains the work? | The project decisions, then the sprint decisions |
| What is ready to pick up? | `bd ready --exclude-type=epic` |
| Where does a project or sprint stand? | Its Progress on its page, `pm show`, `pm show --sprint ID` |

## Core rules
- Put every change in a task in a sprint. Put work outside a sprint into a small new sprint.
- Read the holders and open needs in `pm show` before you take or delegate a task. Do not take work that another live session holds.
- Read one section of a record with `pm show --record R --section S`, not the whole file.
- Run `pm show` again before you tell the owner the project state.
- Claim a task only with `pm task claim`. Do not use `bd update --claim`. Tell a subagent that it holds a task only after the claim succeeds.
- Close a task with `pm task close`. Do not use `bd close`.
- A subagent claims and closes its own task.
- Record a scope change as a sprint decision. Do not rename a task or rewrite its description for it.
- Add findings when they occur. Give each result its numbers.
- Read the decisions before you ask the owner. Do not ask again about a decided question.
- When work waits on the owner, raise a need. Then continue other ready work. Do not decide silently.
- The owner's replies and PR merges come into this session as new turns. Do not poll or wait for them.
- After an owner reply, do its next step at once. Then close the need.
- Start the title of a need from a test with "[TEST]". Dismiss it with `bd human dismiss <id>` when the check ends.
- Do not post status outside the site. The site is the owner's status view.
- Do not push Beads data or the records branch. The scheduled `pm push` does it.
- Examine a subagent's report against the files before you accept it.
- Link a record only with the URL that `pm record link` prints. Do not build a URL or give a records path.
- Do not put decisions or plans on a design page.
- In a sprint's Design pages, link the sub pages that its work changed.
- Write a postmortem when an incident costs more than a day, or breaks other sessions or the site.
- Write "not recorded" for a date or fact that the records do not have. Do not guess.
- Get the owner's confirmation of the project goal before you open a project.
- Run `pm feedback add` when pm stops you or costs you time.

## Records and reviews
These rules have guards. Obey them so that the guards do not stop you.
- Write records with `pm` commands. Each `pm` write commits itself on the records branch.
- Do not commit `records/` on a code branch.
- Commit each hand edit with `pm commit` before you end your turn.
- Give each `pm` write its target with `--sprint` or `--project`.
- Keep each section and its prompt line. Write "None yet." in an empty section.
- Use only these blocks: `::: decision`, `::: result`, mermaid. Put a one-line reading under each diagram or large table.
- Start the delivery report's Outcome with done, partial or voided and one sentence.
- In Against Done when, give each item as met or not, with evidence.
- Push the sprint branch and open its PR without approval.
- Close a sprint in this order: write the report → push and open the PR → `pm action need --pr` → close or move other tasks.
- After the merge is on main: `pm action done <review> --reason "merged as <sha>"` → `pm sprint close`. A merge into a stacked base is not on main. A voided sprint without a PR closes without one.

## Reading pm show
| Text | Meaning | Do |
|---|---|---|
| `held by <session>, <age>, live` | A session works on it. | Leave it. |
| `held by <session>, <age>, idle` | Its session wrote nothing for 30 minutes. | `pm task claim` can take it. |
| `held by <name> without a session` | Someone claimed it outside `pm`. | Leave it, unless the owner tells you to take it. |
| `[undelivered reply: pm reply read <id>]` | An owner reply did not get to its session. | Run `pm reply read <id>`. Then do its next step. |
| `warning: the scheduled push needs attention` | The push failed or is late. | Read `pm where` and `.git/pm-push.log`. If you cannot fix it, raise an action and add a bug task. |

## Planning
- Write the sprint frame before work starts:
  - Goal: what is true at the end, and why now.
  - Scope: In and Out, at a high level. Put the detail in a design page.
  - Done when: a check that shows the goal. Write the expected result before the run.
- Make a sprint take hours or days, never weeks.
- Split work along independence, so that sprints run in parallel. Record each dependency in Beads.
- Test each task against the goal. Build tools or checkers only when a named sprint is blocked without them.
- When a finding voids the goal, close the sprint as voided, not failed.
- Plan one sprint ahead. Choose the next sprint from the last result.
- Run the smallest test that can change the decision. If each result leads to the same next step, do not run it.

## Writing to the owner
Obey these rules in chat replies, needs, actions and the records the owner reads:
- Put the conclusion first.
- Use 4 bullets or fewer.
- Do not use ids, hashes or file names unless the owner asks.
- Give the page link, not a file path.
- Tell what each number measures.
- Use ASD-STE100 approved words. Give each word one meaning.
- Use at most 20 words in an instruction sentence and 25 in a description sentence.
- Use the active voice and simple tenses.
- Write one instruction in each sentence.
