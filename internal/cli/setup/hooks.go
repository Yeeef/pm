package setup

// GitPostCheckout is pm hook git-post-checkout, retired with pm's git hooks: it does nothing, as session start's pm
// init --session-start sets each new worktree up. An earlier pm's section in the main checkout's
// .pm/hooks/post-checkout runs it in every worktree of the clone, through core.hooksPath, until pm init unsets that
// path or pm upgrade removes that section, so a worktree already pinned to this pm must still check out.
func GitPostCheckout() error { return nil }

// GitPreCommit is pm hook git-pre-commit, retired with the main branch's records/ copy, whose guard it was: it does
// nothing. An earlier pm's section in the main checkout's .pm/hooks/pre-commit runs it in every worktree of the clone
// until pm upgrade's removal of that section reaches the main checkout, so a worktree already pinned to this pm must
// still commit.
func GitPreCommit() error { return nil }
