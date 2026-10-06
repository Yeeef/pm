# pm rules

## Model
- Beads holds the work: projects, sprints, tasks, needs.
- Records hold the context: goals, decisions, findings, designs. `pm` writes them. `pm <noun> --help` tells how to use each command.
- The site shows all of it to the owner.

## Core rules
- Put every change in a task in a sprint. Put work outside a sprint into a small new sprint.
- Read the holders and open needs in `pm show` before you take or delegate a task. Do not take work that another live session holds.
- Claim a task before you start it. Tell a subagent that it holds a task only after the claim succeeds.
- Record a scope change as a sprint decision. Do not rename a task or rewrite its description for it.
- Add findings when they occur. Give each result its numbers.
- Record a decision at project level when a later sprint must obey it. Move it to the project when it applies beyond one sprint.
- Use owner source only when the decision answers a need or the owner confirmed it.
- Read the decisions before you ask the owner. Do not ask again about a decided question.
- When work waits on the owner, raise a need. Then continue other ready work. Do not decide silently.
- Do not add a Beads comment to an open request. The site reads it as an owner reply.
- Start the title of a need from a test with "[TEST]". Dismiss it when the check ends.
- Do not post status outside the site. The site is the owner's status view.
- Examine a subagent's report against the files before you accept it.
- Link a record only with the URL that `pm record link` prints. Do not build a URL or give a records path.
- Keep one area on each design page. Write its final state, not a history of findings.
- When the pm service is down, restart it. If the restart fails, raise an action and add a bug task.

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
