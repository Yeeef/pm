pm reads each option only by its whole name.

### Removed

- An abbreviated option is refused: `pm task add --tit x` no longer means `--title x` but fails with
  `unrecognized arguments: --tit x`, as any unknown argument does. Write each option in full. A prefix could change
  meaning when a command gained an option that shares it, and no agent or doc in use relied on one. A command line
  with both an unknown argument and a missing required one now names the unknown one.
