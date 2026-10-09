package work

// Store is the work store as pm's commands, site and service use it: one store per clone, opened at most once per
// process under the gate and released at exit. Every write is one transaction that lands whole or not at all, checks
// the item invariants (the type's fields present, closed means no holder, the tree rules) and fails hard naming the
// item; there is no delete. Each method replaces one use of bd (the work-store page's "Inside pm" table).
type Store interface {
	// Items is every item, closed ones included, each with its comments: one read.
	Items() ([]Item, error)
	// Get is the items with these ids, in that order; a missing id is an error.
	Get(ids ...string) ([]Item, error)
	// Needs is the needs the session raised.
	Needs(session string) ([]Item, error)

	// Create writes a new item and returns it: its id minted under the parent (a root id for a project), and a
	// sprint's number one above its project's highest.
	Create(n New) (Item, error)
	// Edit sets the title and the description; nil leaves one as it is.
	Edit(id string, title, description *string) error
	// Close closes an open item: its reason, resolution and closing session; it clears the holder.
	Close(id, reason string, resolution Resolution, session string) error
	// SetResolution sets a closed item's resolution (pm decision close marks a closed need no-decision).
	SetResolution(id string, resolution Resolution) error
	// Move gives a task or a need a new parent; its id stays.
	Move(id, parent string) error

	// Claim makes h the holder, as a compare-and-set in one transaction: it succeeds when the item has no holder, is
	// held by h.Session, or is held by a session that live reports not live; it sets StartedAt on the first claim.
	Claim(id string, h Holder, live func(session string) bool) error
	// Release clears the holder when the session holds the item.
	Release(id, session string) error

	// DepAdd makes blocker block id; a cycle over blocked_by, ancestors included, or a missing item is an error.
	DepAdd(id, blocker string) error
	// DepRemove removes that link.
	DepRemove(id, blocker string) error

	// Comment adds a comment and returns it.
	Comment(id string, kind CommentKind, author, text string) (Comment, error)
	// Answer records the owner's answer to a need: a reply comment, then the close with resolution answered.
	Answer(id, text string) error
	// UpdateNeed sets a need's delivery and review-merge fields; nil leaves one as it is.
	UpdateNeed(id string, u NeedUpdate) error

	// Shutdown closes the store and releases the gate; the process opens it no more.
	Shutdown() error
}

// New is what Create needs; the store sets the id, the number, the status (open) and the timestamps.
type New struct {
	Type        Type
	Parent      string
	Title       string
	Description string
	Labels      []string
	Need        *NeedInfo // for Type Need only
}

// NeedUpdate is the need fields the pm service and pm reply read move.
type NeedUpdate struct {
	RaisedInbox, RaisedHost *string // the raising session's inbox now (pm show --refresh-inbox); a need it raised only
	Delivered               *int
	ReviewMerged            *string
	ReviewMergeReported     *string
}
