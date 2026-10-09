package work

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	embedded "github.com/dolthub/driver/v2"
)

// Dolt is the work store on embedded Dolt (dolthub/driver), as the work-store page's Storage section and the pm-go
// page's "Store sharing between the CLI and the service" give it: a process takes the gate, opens the engine once,
// runs every query on that one open store and closes it at exit (Shutdown). Every write is one SQL transaction that
// loads the items, applies the change, checks every invariant over the result and ends in DOLT_COMMIT, so it lands
// whole or not at all. Every read checks the invariants too and fails hard naming the item.
type Dolt struct {
	o      Options
	gate   *gate
	engine *embedded.Connector
	db     *sql.DB
	conn   *sql.Conn
}

var _ Store = (*Dolt)(nil)

// Options locate a store and set how it mints ids and tells time.
type Options struct {
	Dir         string           // the store: <main checkout>/.pm/store/work
	RunDir      string           // <main checkout>/.pm/run: the gate's lock file and its wait log
	Prefix      string           // the prefix of minted root ids: the repo name
	GateTimeout time.Duration    // 0: GateTimeout
	Now         func() time.Time // nil: the clock
}

// Locations is the store and run directory of the clone whose main checkout is main.
func Locations(main string) (dir, run string) {
	return filepath.Join(main, ".pm", "store", "work"), filepath.Join(main, ".pm", "run")
}

// dbName is the Dolt database inside the store directory.
const dbName = "work"

var ctx = context.Background()

// Exists is whether dir holds a work store.
func Exists(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, dbName, ".dolt"))
	return err == nil && st.IsDir()
}

// OpenStore takes the gate and opens the store in o.Dir, migrating an older schema; it fails when there is no store or its
// schema is newer than this pm's.
func OpenStore(o Options) (*Dolt, error) {
	d, err := start(o)
	if err != nil {
		return nil, err
	}
	if !Exists(o.Dir) {
		d.Shutdown()
		return nil, fmt.Errorf("work store: none at %s; pm init creates or clones it", o.Dir)
	}
	if err := d.connect(true); err != nil {
		d.Shutdown()
		return nil, err
	}
	if err := d.migrate(); err != nil {
		d.Shutdown()
		return nil, err
	}
	return d, nil
}

// CreateStore takes the gate and makes a new store in o.Dir with the schema at SchemaVersion; it fails when one is there.
func CreateStore(o Options) (*Dolt, error) {
	d, err := start(o)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*Dolt, error) {
		d.Shutdown()
		return nil, err
	}
	if Exists(o.Dir) {
		return fail(fmt.Errorf("work store: one is already at %s", o.Dir))
	}
	if err := os.MkdirAll(o.Dir, 0o755); err != nil {
		return fail(fmt.Errorf("work store: %w", err))
	}
	if err := d.connect(false); err != nil {
		return fail(err)
	}
	if _, err := d.conn.ExecContext(ctx, "CREATE DATABASE `"+dbName+"`"); err != nil {
		return fail(fmt.Errorf("work store: create the database: %w", err))
	}
	if _, err := d.conn.ExecContext(ctx, "USE `"+dbName+"`"); err != nil {
		return fail(fmt.Errorf("work store: %w", err))
	}
	if err := d.inTx(fmt.Sprintf("pm: create the work store at schema version %d", SchemaVersion),
		func(tx *sql.Tx) error { return execAll(tx, schema) }); err != nil {
		return fail(err)
	}
	return d, nil
}

// start takes the gate.
func start(o Options) (*Dolt, error) {
	if o.Dir == "" || o.RunDir == "" {
		return nil, errors.New("work store: no store or run directory given")
	}
	if o.GateTimeout == 0 {
		o.GateTimeout = GateTimeout
	}
	g, _, err := takeGate(o.RunDir, o.GateTimeout)
	if err != nil {
		return nil, err
	}
	return &Dolt{o: o, gate: g}, nil
}

// connect opens the engine on the store directory, with the work database selected when withDB. No open retry: the
// gate serialises pm processes, so a locked engine is an impossible state and fails hard.
func (d *Dolt) connect(withDB bool) error {
	v := url.Values{}
	v.Set(embedded.CommitNameParam, "pm")
	v.Set(embedded.CommitEmailParam, "pm@localhost")
	if withDB {
		v.Set(embedded.DatabaseParam, dbName)
	}
	cfg, err := embedded.ParseDSN("file://" + d.o.Dir + "?" + v.Encode())
	if err != nil {
		return fmt.Errorf("work store: %w", err)
	}
	if d.engine, err = embedded.NewConnector(cfg); err != nil {
		return fmt.Errorf("work store: %w", err)
	}
	d.db = sql.OpenDB(d.engine)
	d.db.SetMaxOpenConns(1)
	if d.conn, err = d.db.Conn(ctx); err != nil {
		return fmt.Errorf("work store: open %s: %w", d.o.Dir, err)
	}
	return nil
}

// migrate checks the schema version: a newer one is refused, an older one is brought up to SchemaVersion.
func (d *Dolt) migrate() error {
	var v int
	if err := d.conn.QueryRowContext(ctx, "SELECT version FROM schema_version WHERE one = 1").Scan(&v); err != nil {
		return fmt.Errorf("work store: read the schema version: %w", err)
	}
	if v > SchemaVersion {
		return fmt.Errorf("work store: its schema is version %d, newer than this pm's %d: run pm upgrade", v,
			SchemaVersion)
	}
	for ; v < SchemaVersion; v++ {
		steps := append(slices.Clone(migrations[v-1]), fmt.Sprintf("UPDATE schema_version SET version = %d", v+1))
		if err := d.inTx(fmt.Sprintf("pm: migrate the work store schema to version %d", v+1),
			func(tx *sql.Tx) error { return execAll(tx, steps) }); err != nil {
			return err
		}
	}
	return nil
}

// Shutdown closes the store and releases the gate; the process opens it no more.
func (d *Dolt) Shutdown() error {
	var errs []error
	if d.conn != nil {
		errs = append(errs, d.conn.Close())
		d.conn = nil
	}
	if d.db != nil {
		errs = append(errs, d.db.Close())
		d.db = nil
	}
	if d.engine != nil {
		errs = append(errs, d.engine.Close())
		d.engine = nil
	}
	errs = append(errs, d.gate.release())
	d.gate = nil
	for _, err := range errs {
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("work store: close: %w", err)
		}
	}
	return nil
}

// now is the store's clock: UTC, whole seconds.
func (d *Dolt) now() time.Time {
	t := time.Now()
	if d.o.Now != nil {
		t = d.o.Now()
	}
	return t.UTC().Truncate(time.Second)
}

func execAll(tx *sql.Tx, stmts []string) error {
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("work store: %w", err)
		}
	}
	return nil
}

// inTx runs fn in one transaction that ends in a Dolt commit with msg; on any error nothing lands.
func (d *Dolt) inTx(msg string, fn func(tx *sql.Tx) error) error {
	if d.conn == nil {
		return errors.New("work store: closed")
	}
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("work store: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, "CALL DOLT_COMMIT('-Am', ?)", msg); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("work store: commit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("work store: commit: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------- reads

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

const itemColumns = `id, type, parent, title, description, status, resolution, close_reason, number, holder_session,
	holder_host, holder_claimed_at, started_at, created_at, updated_at, closed_at, closed_by, need_kind, raised_session,
	raised_inbox, raised_host, delivered, review_pr, review_focus, review_merged, review_merge_reported`

// load reads every item with its labels, blockers, review targets and comments, ordered by id; it does not check them.
func load(q querier) ([]Item, error) {
	rows, err := q.QueryContext(ctx, "SELECT "+itemColumns+" FROM items ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("work store: %w", err)
	}
	var items []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("work store: %w", err)
	}
	by := make(map[string]*Item, len(items))
	for i := range items {
		by[items[i].ID] = &items[i]
	}
	of := func(table, id string) (*Item, error) {
		if it := by[id]; it != nil {
			return it, nil
		}
		return nil, fmt.Errorf("work store: %s has a row for %s, which is not an item", table, id)
	}
	pairs := func(table, query string, add func(it *Item, v string)) error {
		r, err := q.QueryContext(ctx, query)
		if err != nil {
			return fmt.Errorf("work store: %w", err)
		}
		defer r.Close()
		for r.Next() {
			var id, v string
			if err := r.Scan(&id, &v); err != nil {
				return fmt.Errorf("work store: %w", err)
			}
			it, err := of(table, id)
			if err != nil {
				return err
			}
			add(it, v)
		}
		return r.Err()
	}
	if err := pairs("labels", "SELECT item_id, label FROM labels ORDER BY item_id, label",
		func(it *Item, v string) { it.Labels = append(it.Labels, v) }); err != nil {
		return nil, err
	}
	if err := pairs("blocked_by", "SELECT item_id, blocker_id FROM blocked_by ORDER BY item_id, blocker_id",
		func(it *Item, v string) { it.BlockedBy = append(it.BlockedBy, v) }); err != nil {
		return nil, err
	}
	var targetErr error
	if err := pairs("review_targets",
		"SELECT item_id, CONCAT(kind, ' ', target) FROM review_targets ORDER BY item_id, kind, pos",
		func(it *Item, v string) {
			kind, target, _ := strings.Cut(v, " ")
			if it.Need == nil || it.Need.Review == nil {
				targetErr = itemError(it.ID, "has review targets but is no review need")
				return
			}
			if kind == "sprint" {
				it.Need.Review.Sprints = append(it.Need.Review.Sprints, target)
			} else {
				it.Need.Review.Designs = append(it.Need.Review.Designs, target)
			}
		}); err != nil {
		return nil, err
	}
	if targetErr != nil {
		return nil, targetErr
	}
	r, err := q.QueryContext(ctx, "SELECT id, item_id, kind, author, text, created_at FROM comments ORDER BY item_id, pos")
	if err != nil {
		return nil, fmt.Errorf("work store: %w", err)
	}
	defer r.Close()
	for r.Next() {
		var c Comment
		var id string
		if err := r.Scan(&c.ID, &id, &c.Kind, &c.Author, &c.Text, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("work store: %w", err)
		}
		c.CreatedAt = c.CreatedAt.UTC()
		it, err := of("comments", id)
		if err != nil {
			return nil, err
		}
		it.Comments = append(it.Comments, c)
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("work store: %w", err)
	}
	return items, nil
}

// scanItem reads one items row; a column set that fits no item (a need column without need_kind, half a holder)
// fails hard.
func scanItem(rows *sql.Rows) (Item, error) {
	var it Item
	var parent, resolution, closeReason, holderSession, holderHost, closedBy, needKind, raisedSession, raisedInbox,
		raisedHost, reviewPR, reviewFocus, reviewMerged, reviewMergeReported sql.NullString
	var number, delivered sql.NullInt64
	var holderClaimed, started, closed sql.NullTime
	if err := rows.Scan(&it.ID, &it.Type, &parent, &it.Title, &it.Description, &it.Status, &resolution, &closeReason,
		&number, &holderSession, &holderHost, &holderClaimed, &started, &it.CreatedAt, &it.UpdatedAt, &closed,
		&closedBy, &needKind, &raisedSession, &raisedInbox, &raisedHost, &delivered, &reviewPR, &reviewFocus,
		&reviewMerged, &reviewMergeReported); err != nil {
		return it, fmt.Errorf("work store: %w", err)
	}
	it.Parent, it.Resolution, it.CloseReason = parent.String, Resolution(resolution.String), closeReason.String
	it.Number, it.ClosedBy = int(number.Int64), closedBy.String
	if number.Valid && number.Int64 <= 0 {
		return it, itemError(it.ID, "has number %d", number.Int64)
	}
	it.CreatedAt, it.UpdatedAt = it.CreatedAt.UTC(), it.UpdatedAt.UTC()
	if started.Valid {
		it.StartedAt = started.Time.UTC()
	}
	if closed.Valid {
		it.ClosedAt = closed.Time.UTC()
	}
	if holderSession.Valid != holderClaimed.Valid || holderHost.Valid && !holderSession.Valid {
		return it, itemError(it.ID, "has half a holder (holder_session, holder_host, holder_claimed_at)")
	}
	if holderSession.Valid {
		it.Holder = &Holder{Session: holderSession.String, Host: holderHost.String, ClaimedAt: holderClaimed.Time.UTC()}
	}
	if !needKind.Valid {
		for _, c := range []bool{raisedSession.Valid, raisedInbox.Valid, raisedHost.Valid, delivered.Valid,
			reviewPR.Valid, reviewFocus.Valid, reviewMerged.Valid, reviewMergeReported.Valid} {
			if c {
				return it, itemError(it.ID, "has need columns but no need_kind")
			}
		}
		return it, nil
	}
	if !delivered.Valid {
		return it, itemError(it.ID, "is a need without delivered")
	}
	n := &NeedInfo{Kind: NeedKind(needKind.String), Delivered: int(delivered.Int64)}
	if raisedSession.Valid {
		n.RaisedBy = &RaisedBy{Session: raisedSession.String, Inbox: raisedInbox.String, Host: raisedHost.String}
	} else if raisedInbox.Valid || raisedHost.Valid {
		return it, itemError(it.ID, "has a raised_by inbox or host without a session")
	}
	if n.Kind == Review {
		n.Review = &ReviewInfo{PR: reviewPR.String, Sprints: []string{}, Focus: reviewFocus.String,
			Merged: reviewMerged.String, MergeReported: reviewMergeReported.String}
	} else if reviewPR.Valid || reviewFocus.Valid || reviewMerged.Valid || reviewMergeReported.Valid {
		return it, itemError(it.ID, "is a %s need with review columns", n.Kind)
	}
	it.Need = n
	return it, nil
}

// loadChecked is load, then Check: what every read returns.
func loadChecked(q querier) ([]Item, error) {
	items, err := load(q)
	if err != nil {
		return nil, err
	}
	if err := Check(items); err != nil {
		return nil, err
	}
	return items, nil
}

// Items is every item, closed ones included, each with its comments, ordered by id: one read.
func (d *Dolt) Items() ([]Item, error) {
	if d.conn == nil {
		return nil, errors.New("work store: closed")
	}
	return loadChecked(d.conn)
}

// Get is the items with these ids, in that order; a missing id is an error.
func (d *Dolt) Get(ids ...string) ([]Item, error) {
	items, err := d.Items()
	if err != nil {
		return nil, err
	}
	x, err := NewIndex(items)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(ids))
	for _, id := range ids {
		it := x.Item(id)
		if it == nil {
			return nil, fmt.Errorf("work store: no item %s", id)
		}
		out = append(out, *it)
	}
	return out, nil
}

// Needs is the needs the session raised, ordered by id.
func (d *Dolt) Needs(session string) ([]Item, error) {
	items, err := d.Items()
	if err != nil {
		return nil, err
	}
	var out []Item
	for _, it := range items {
		if it.Need != nil && it.Need.RaisedBy != nil && it.Need.RaisedBy.Session == session {
			out = append(out, it)
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- writes

// change is one write's edit: given the store's items (fresh copies, indexed), it returns the items it changed or
// made, each a whole item.
type change func(x *Index) ([]Item, error)

// write runs one change as one transaction: load and check, apply, check the result, write the changed rows, and
// DOLT_COMMIT with msg. Nothing lands unless all of it does.
func (d *Dolt) write(msg string, fn change) error {
	return d.inTx(msg, func(tx *sql.Tx) error {
		items, err := loadChecked(tx)
		if err != nil {
			return err
		}
		x, err := NewIndex(items)
		if err != nil {
			return err
		}
		had := map[string][]Comment{} // each item's comments before the change
		for _, it := range items {
			had[it.ID] = slices.Clone(it.Comments)
		}
		changed, err := fn(x)
		if err != nil {
			return err
		}
		after := slices.Clone(items)
		at := map[string]int{}
		for i, it := range after {
			at[it.ID] = i
		}
		for _, it := range changed {
			if i, ok := at[it.ID]; ok {
				after[i] = it
			} else {
				at[it.ID] = len(after)
				after = append(after, it)
			}
		}
		if err := Check(after); err != nil {
			return err
		}
		return writeRows(tx, after, changed, had)
	})
}

// writeRows writes the changed items' rows: each items row upserted, parents before children; then each item's
// labels, blockers and review targets replaced, and its new comments added. Comments are append-only: an item's
// comments before the change must stay, unchanged and in order, at the head of its list.
func writeRows(tx *sql.Tx, all, changed []Item, had map[string][]Comment) error {
	x, err := NewIndex(all)
	if err != nil {
		return err
	}
	order := slices.Clone(changed)
	slices.SortStableFunc(order, func(a, b Item) int { return len(x.Ancestors(a.ID)) - len(x.Ancestors(b.ID)) })
	for i := range order {
		if err := upsertItem(tx, &order[i]); err != nil {
			return err
		}
	}
	for _, it := range order {
		before := had[it.ID]
		if len(it.Comments) < len(before) || !slices.EqualFunc(before, it.Comments[:len(before)],
			func(a, b Comment) bool { return a == b }) {
			return itemError(it.ID, "lost or changed a comment; comments are append-only")
		}
		if err := writeLists(tx, &it, len(before)); err != nil {
			return err
		}
	}
	return nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC()
}

// upsertItem writes an item's row in items, inserting it or setting every column.
func upsertItem(tx *sql.Tx, it *Item) error {
	var number, holderSession, holderHost, holderClaimed, needKind, raisedSession, raisedInbox, raisedHost, delivered,
		reviewPR, reviewFocus, reviewMerged, reviewMergeReported any
	if it.Number > 0 {
		number = it.Number
	}
	if h := it.Holder; h != nil {
		holderSession, holderHost, holderClaimed = h.Session, nullString(h.Host), nullTime(h.ClaimedAt)
	}
	if n := it.Need; n != nil {
		needKind, delivered = string(n.Kind), n.Delivered
		if r := n.RaisedBy; r != nil {
			raisedSession, raisedInbox, raisedHost = r.Session, nullString(r.Inbox), nullString(r.Host)
		}
		if r := n.Review; r != nil {
			reviewPR, reviewFocus = r.PR, r.Focus
			reviewMerged, reviewMergeReported = nullString(r.Merged), nullString(r.MergeReported)
		}
	}
	cols := strings.Fields(strings.ReplaceAll(itemColumns, ",", " "))
	set := make([]string, 0, len(cols))
	for _, c := range cols[1:] {
		set = append(set, c+" = VALUES("+c+")")
	}
	q := "INSERT INTO items (" + strings.Join(cols, ", ") + ") VALUES (" +
		strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", ") + ") ON DUPLICATE KEY UPDATE " +
		strings.Join(set, ", ")
	_, err := tx.ExecContext(ctx, q, it.ID, string(it.Type), nullString(it.Parent), it.Title, it.Description,
		string(it.Status), nullString(string(it.Resolution)), nullString(it.CloseReason), number, holderSession,
		holderHost, holderClaimed, nullTime(it.StartedAt), it.CreatedAt.UTC(), it.UpdatedAt.UTC(),
		nullTime(it.ClosedAt), nullString(it.ClosedBy), needKind, raisedSession, raisedInbox, raisedHost, delivered,
		reviewPR, reviewFocus, reviewMerged, reviewMergeReported)
	if err != nil {
		return fmt.Errorf("work store: write %s: %w", it.ID, err)
	}
	return nil
}

// writeLists replaces an item's labels, blockers and review targets with its own, and adds its comments from position
// kept on, each with its position in the item's list.
func writeLists(tx *sql.Tx, it *Item, kept int) error {
	exec := func(q string, args ...any) error {
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("work store: write %s: %w", it.ID, err)
		}
		return nil
	}
	for _, table := range []string{"labels", "blocked_by", "review_targets"} {
		if err := exec("DELETE FROM "+table+" WHERE item_id = ?", it.ID); err != nil {
			return err
		}
	}
	for _, l := range it.Labels {
		if err := exec("INSERT INTO labels (item_id, label) VALUES (?, ?)", it.ID, l); err != nil {
			return err
		}
	}
	for _, b := range it.BlockedBy {
		if err := exec("INSERT INTO blocked_by (item_id, blocker_id) VALUES (?, ?)", it.ID, b); err != nil {
			return err
		}
	}
	if it.Need != nil && it.Need.Review != nil {
		for kind, targets := range map[string][]string{"sprint": it.Need.Review.Sprints, "design": it.Need.Review.Designs} {
			for pos, t := range targets {
				if err := exec("INSERT INTO review_targets (item_id, kind, target, pos) VALUES (?, ?, ?, ?)",
					it.ID, kind, t, pos); err != nil {
					return err
				}
			}
		}
	}
	for pos := kept; pos < len(it.Comments); pos++ {
		c := it.Comments[pos]
		if err := exec("INSERT INTO comments (id, item_id, pos, kind, author, text, created_at) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?)", c.ID, it.ID, pos, string(c.Kind), c.Author, c.Text, c.CreatedAt.UTC()); err != nil {
			return err
		}
	}
	return nil
}

// Import writes items into an empty store as one transaction and one Dolt commit (pm init --import-bd). It refuses a
// store that holds any item, and items that fail Check.
func (d *Dolt) Import(items []Item, msg string) error {
	if len(items) == 0 {
		return errors.New("work store: an import of no items")
	}
	return d.write(msg, func(x *Index) ([]Item, error) {
		if len(x.items) > 0 {
			return nil, fmt.Errorf("work store: it holds %d items already; an import goes into an empty store only",
				len(x.items))
		}
		return items, nil
	})
}

// ---------------------------------------------------------------- the Store methods

// one is a copy of the item with this id, so a change edits only its copy.
func one(x *Index, id string) (Item, error) {
	it := x.Item(id)
	if it == nil {
		return Item{}, fmt.Errorf("work store: no item %s", id)
	}
	return cloneItem(*it), nil
}

// cloneItem is a deep copy of an item: its lists and pointed-to fields are its own.
func cloneItem(it Item) Item {
	it.Labels, it.BlockedBy, it.Comments = slices.Clone(it.Labels), slices.Clone(it.BlockedBy), slices.Clone(it.Comments)
	if it.Holder != nil {
		h := *it.Holder
		it.Holder = &h
	}
	if it.Need != nil {
		n := *it.Need
		if n.RaisedBy != nil {
			r := *n.RaisedBy
			n.RaisedBy = &r
		}
		if n.Review != nil {
			r := *n.Review
			r.Sprints, r.Designs = slices.Clone(r.Sprints), slices.Clone(r.Designs)
			n.Review = &r
		}
		it.Need = &n
	}
	return it
}

// Create writes a new item and returns it: its id minted under the parent (a root id for a project), a sprint's
// number one above its project's highest, status open. The id is minted from this store copy only; the compare-and-
// swap through the remote that keeps two clones from minting the same child id comes with sync.
func (d *Dolt) Create(n New) (Item, error) {
	var made Item
	err := d.write(fmt.Sprintf("pm: create a %s under %q", n.Type, n.Parent), func(x *Index) ([]Item, error) {
		now := d.now()
		it := Item{Type: n.Type, Parent: n.Parent, Title: n.Title, Description: n.Description, Status: Open,
			CreatedAt: now, UpdatedAt: now}
		ids := make([]string, 0, len(x.items))
		for _, o := range x.items {
			ids = append(ids, o.ID)
		}
		switch {
		case n.Type == Project && n.Parent != "":
			return nil, fmt.Errorf("work store: a project has no parent, not %s", n.Parent)
		case n.Type == Project:
			id, err := NewRoot(d.o.Prefix, func(id string) bool { return x.Item(id) != nil }, rand.Reader)
			if err != nil {
				return nil, err
			}
			it.ID = id
		case n.Parent == "":
			return nil, fmt.Errorf("work store: a %s needs a parent", n.Type)
		case x.Item(n.Parent) == nil:
			return nil, fmt.Errorf("work store: no item %s to put a %s under", n.Parent, n.Type)
		default:
			it.ID = NextChild(ids, n.Parent)
		}
		if n.Type == Sprint {
			for _, o := range x.items {
				if o.Type == Sprint && o.Parent == n.Parent {
					it.Number = max(it.Number, o.Number)
				}
			}
			it.Number++
		}
		it.Labels = slices.Compact(slices.Sorted(slices.Values(n.Labels)))
		it.Need = cloneItem(Item{Need: n.Need}).Need
		made = it
		return []Item{it}, nil
	})
	if err != nil {
		return Item{}, err
	}
	return made, nil
}

// update is a write that changes one existing item.
func (d *Dolt) update(verb, id string, fn func(it *Item, x *Index) error) error {
	return d.write(fmt.Sprintf("pm: %s %s", verb, id), func(x *Index) ([]Item, error) {
		it, err := one(x, id)
		if err != nil {
			return nil, err
		}
		if err := fn(&it, x); err != nil {
			return nil, err
		}
		it.UpdatedAt = d.now()
		return []Item{it}, nil
	})
}

// Edit sets the title and the description; nil leaves one as it is.
func (d *Dolt) Edit(id string, title, description *string) error {
	return d.update("edit", id, func(it *Item, _ *Index) error {
		if title != nil {
			it.Title = *title
		}
		if description != nil {
			it.Description = *description
		}
		return nil
	})
}

// Close closes an open item: its reason, resolution and closing session; it clears the holder.
func (d *Dolt) Close(id, reason string, resolution Resolution, session string) error {
	return d.update("close", id, func(it *Item, _ *Index) error {
		if it.Status != Open {
			return itemError(id, "is closed already")
		}
		it.Status, it.Resolution, it.CloseReason, it.ClosedBy = Closed, resolution, reason, session
		it.ClosedAt, it.Holder = d.now(), nil
		return nil
	})
}

// SetResolution sets a closed item's resolution (pm decision close marks a closed need no-decision).
func (d *Dolt) SetResolution(id string, resolution Resolution) error {
	return d.update("set the resolution of", id, func(it *Item, _ *Index) error {
		if it.Status != Closed {
			return itemError(id, "is open; only a closed item has a resolution")
		}
		it.Resolution = resolution
		return nil
	})
}

// Move gives a task a new parent; its id stays.
func (d *Dolt) Move(id, parent string) error {
	return d.update("move", id, func(it *Item, _ *Index) error {
		if it.Type != Task {
			return itemError(id, "is a %s; only a task moves", it.Type)
		}
		it.Parent = parent
		return nil
	})
}

// Claim makes h the holder, as a compare-and-set in one transaction: it succeeds when the item has no holder, is held
// by h.Session, or is held by a session that live reports not live; it sets StartedAt on the first claim. The claim
// time is the store's clock.
func (d *Dolt) Claim(id string, h Holder, live func(session string) bool) error {
	return d.update("claim", id, func(it *Item, _ *Index) error {
		if it.Status != Open {
			return itemError(id, "is closed")
		}
		if h.Session == "" {
			return itemError(id, "cannot be claimed without a session")
		}
		if o := it.Holder; o != nil && o.Session != h.Session && live(o.Session) {
			return itemError(id, "is held by live session %s", o.Session)
		}
		now := d.now()
		it.Holder = &Holder{Session: h.Session, Host: h.Host, ClaimedAt: now}
		if it.StartedAt.IsZero() {
			it.StartedAt = now
		}
		return nil
	})
}

// Release clears the holder when the session holds the item.
func (d *Dolt) Release(id, session string) error {
	return d.update("release", id, func(it *Item, _ *Index) error {
		if it.Holder == nil || it.Holder.Session != session {
			return itemError(id, "is not held by %s", session)
		}
		it.Holder = nil
		return nil
	})
}

// DepAdd makes blocker block id; a cycle over blocked_by, ancestors included, or a missing item is an error.
func (d *Dolt) DepAdd(id, blocker string) error {
	return d.update("make "+blocker+" block", id, func(it *Item, x *Index) error {
		if x.Item(blocker) == nil {
			return fmt.Errorf("work store: no item %s", blocker)
		}
		if slices.Contains(it.BlockedBy, blocker) {
			return itemError(id, "is blocked by %s already", blocker)
		}
		it.BlockedBy = slices.Sorted(slices.Values(append(it.BlockedBy, blocker)))
		return nil
	})
}

// DepRemove removes that link.
func (d *Dolt) DepRemove(id, blocker string) error {
	return d.update("remove blocker "+blocker+" from", id, func(it *Item, _ *Index) error {
		i := slices.Index(it.BlockedBy, blocker)
		if i < 0 {
			return itemError(id, "is not blocked by %s", blocker)
		}
		it.BlockedBy = slices.Delete(it.BlockedBy, i, i+1)
		return nil
	})
}

// Comment adds a comment and returns it.
func (d *Dolt) Comment(id string, kind CommentKind, author, text string) (Comment, error) {
	var c Comment
	err := d.update("comment on", id, func(it *Item, _ *Index) error {
		cid, err := newUUID()
		if err != nil {
			return err
		}
		c = Comment{ID: cid, Kind: kind, Author: author, Text: text, CreatedAt: d.now()}
		it.Comments = append(it.Comments, c)
		return nil
	})
	return c, err
}

// Answer records the owner's answer to a need: a reply comment, then the close with resolution answered and the
// reason "Responded", as bd human respond closed it.
func (d *Dolt) Answer(id, text string) error {
	return d.update("answer", id, func(it *Item, _ *Index) error {
		if it.Type != Need {
			return itemError(id, "is a %s; only a need is answered", it.Type)
		}
		if it.Status != Open {
			return itemError(id, "is closed already")
		}
		cid, err := newUUID()
		if err != nil {
			return err
		}
		now := d.now()
		it.Comments = append(it.Comments, Comment{ID: cid, Kind: Reply, Author: "owner", Text: text, CreatedAt: now})
		it.Status, it.Resolution, it.CloseReason, it.ClosedAt, it.Holder = Closed, Answered, "Responded", now, nil
		return nil
	})
}

// UpdateNeed sets a need's delivery and review-merge fields; nil leaves one as it is.
func (d *Dolt) UpdateNeed(id string, u NeedUpdate) error {
	return d.update("update need", id, func(it *Item, _ *Index) error {
		if it.Need == nil {
			return itemError(id, "is a %s, not a need", it.Type)
		}
		if u.Delivered != nil {
			it.Need.Delivered = *u.Delivered
		}
		if u.ReviewMerged != nil || u.ReviewMergeReported != nil {
			if it.Need.Review == nil {
				return itemError(id, "is a %s need, not a review", it.Need.Kind)
			}
			if u.ReviewMerged != nil {
				it.Need.Review.Merged = *u.ReviewMerged
			}
			if u.ReviewMergeReported != nil {
				it.Need.Review.MergeReported = *u.ReviewMergeReported
			}
		}
		return nil
	})
}
