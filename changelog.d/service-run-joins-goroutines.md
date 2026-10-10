### Fixed

- A pm service that stops (its pin moved) waits for its own loops before it closes the work store: a reply or merge
  being written, the sweep, the merge watch, a sync or a gc under way. It used to close the store while one of them
  could still be writing to it; a sync or gc under way is now canceled.
