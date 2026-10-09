package work

// The work store's schema, as the work-store page's Storage section gives it: typed tables, no JSON cell, since Dolt
// merges a cell whole. A list is a table of rows, so Dolt's row merge unions two clones' additions per entry. The
// schema enforces types, NOT NULL, enums (a CHECK on a VARCHAR: dolthub/driver v2.2.0 panics reading an ENUM
// column) and foreign keys; the rest of the invariants are Check's.

// SchemaVersion is the version this pm's schema is at. A store at a lower version is migrated on open by the steps in
// migrations; a store at a higher one is refused: an older pm does not write a newer schema.
const SchemaVersion = 2

// schema is version 1, statement by statement; CreateStore then runs migrations up to SchemaVersion, as an open of an
// older store does.
var schema = []string{
	`CREATE TABLE schema_version (
		one TINYINT NOT NULL PRIMARY KEY,
		version INT NOT NULL,
		CHECK (one = 1)
	)`,
	`CREATE TABLE items (
		id VARCHAR(128) NOT NULL PRIMARY KEY,
		type VARCHAR(7) NOT NULL,
		parent VARCHAR(128),
		title VARCHAR(1024) NOT NULL,
		description TEXT NOT NULL,
		status VARCHAR(6) NOT NULL,
		resolution VARCHAR(11),
		close_reason TEXT,
		number INT,
		holder_session VARCHAR(128),
		holder_host VARCHAR(255),
		holder_claimed_at DATETIME,
		started_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		closed_at DATETIME,
		closed_by VARCHAR(128),
		need_kind VARCHAR(8),
		raised_session VARCHAR(128),
		raised_inbox VARCHAR(1024),
		raised_host VARCHAR(255),
		delivered INT,
		review_pr VARCHAR(1024),
		review_focus TEXT,
		review_merged VARCHAR(64),
		review_merge_reported VARCHAR(64),
		KEY items_parent (parent),
		KEY items_raised_session (raised_session),
		CONSTRAINT items_parent_fk FOREIGN KEY (parent) REFERENCES items (id),
		CHECK (type IN ('project','sprint','task','need')),
		CHECK (status IN ('open','closed')),
		CHECK (resolution IN ('done','answered','no-decision','dismissed')),
		CHECK (need_kind IN ('decision','action','review'))
	)`,
	// A review's sprints (kind sprint) and design pages (kind design); pos keeps the order they were given in.
	`CREATE TABLE review_targets (
		item_id VARCHAR(128) NOT NULL,
		kind VARCHAR(6) NOT NULL,
		target VARCHAR(255) NOT NULL,
		pos INT NOT NULL,
		PRIMARY KEY (item_id, kind, target),
		CONSTRAINT review_targets_item_fk FOREIGN KEY (item_id) REFERENCES items (id),
		CHECK (kind IN ('sprint','design'))
	)`,
	`CREATE TABLE labels (
		item_id VARCHAR(128) NOT NULL,
		label VARCHAR(255) NOT NULL,
		PRIMARY KEY (item_id, label),
		CONSTRAINT labels_item_fk FOREIGN KEY (item_id) REFERENCES items (id)
	)`,
	`CREATE TABLE blocked_by (
		item_id VARCHAR(128) NOT NULL,
		blocker_id VARCHAR(128) NOT NULL,
		PRIMARY KEY (item_id, blocker_id),
		CONSTRAINT blocked_by_item_fk FOREIGN KEY (item_id) REFERENCES items (id),
		CONSTRAINT blocked_by_blocker_fk FOREIGN KEY (blocker_id) REFERENCES items (id)
	)`,
	`CREATE TABLE comments (
		id VARCHAR(64) NOT NULL PRIMARY KEY,
		item_id VARCHAR(128) NOT NULL,
		pos INT NOT NULL,
		kind VARCHAR(5) NOT NULL,
		author VARCHAR(255) NOT NULL,
		text TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		UNIQUE KEY comments_item_pos (item_id, pos),
		CONSTRAINT comments_item_fk FOREIGN KEY (item_id) REFERENCES items (id),
		CHECK (kind IN ('reply','note'))
	)`,
	`INSERT INTO schema_version (one, version) VALUES (1, 1)`,
}

// migrations[i] takes a store from version i+1 to i+2, statement by statement; each runs in one transaction with
// one Dolt commit.
var migrations = [][]string{
	// 2: comments union by id across clones (the merge table), so two clones that each add a comment to one item may
	// both use the same pos; a unique (item_id, pos) made that a constraint violation on merge. pos stays the order an
	// item's comments were added in, ties ordered by created_at and id.
	{
		`ALTER TABLE comments ADD KEY comments_item_order (item_id, pos)`,
		`ALTER TABLE comments DROP INDEX comments_item_pos`,
	},
}
