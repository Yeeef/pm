package work

import (
	"context"
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/Yeeef/pm/internal/buildinfo"
	"github.com/Yeeef/pm/internal/config"
)

// Dolt is the work store as a pm command reaches it: a SQL client of the pm service's socket, with one connection
// held until the command exits (Shutdown), as the pm-go page's "Store access" gives it. The service (Host) holds the
// Dolt engine; no command opens the store itself, and nothing falls back to a direct open. Every write is one SQL
// transaction that loads the items, applies the change, checks every invariant over the result, sets the write stamp
// and ends in DOLT_COMMIT, so it lands whole or not at all; a write that loses to a concurrent one runs again from a
// fresh read. Every read is one read-only transaction and checks the invariants too, failing hard naming the item.
type Dolt struct {
	o    Options
	sock string // the service's socket, which remoteCall dials again
	db   *sql.DB
	conn *sql.Conn
	// pushFn, when a test sets it, runs each push in push's place, given the real push.
	pushFn func(push func() error) error
	// op bounds the operation running now (a sync, a create); nil otherwise.
	op context.Context
}

var _ Store = (*Dolt)(nil)

// Options set how a client mints ids and tells time.
type Options struct {
	Prefix string           // the prefix of minted root ids: the repo name
	Now    func() time.Time // nil: the clock
}

// Locations is the store and run directory of the clone whose main checkout is main.
func Locations(main string) (dir, run string) {
	return filepath.Join(main, ".pm", "store", "work"), filepath.Join(main, ".pm", "run")
}

const (
	// dbName is the Dolt database inside the store directory.
	dbName = "work"
	// ConnectTimeout bounds the connect to the service's socket.
	ConnectTimeout = 2 * time.Second
)

var (
	ctx = context.Background()
	// ErrNoStore is a service with no work database yet: pm init's setup makes it.
	ErrNoStore = errors.New("this clone has no work store yet: run pm init")
)

// dialConfig is how dial connects: the client's options, whether it selects the work database, and the version
// handshake (none for the host's own connections).
type dialConfig struct {
	prefix    string
	now       func() time.Time
	withDB    bool
	handshake func(service string) error
}

// Dial connects to the pm service of the clone whose main checkout is main, checks that it runs this pm's version,
// and selects the work database. It fails hard when nothing answers on the socket, the service runs another version,
// or the clone has no work store yet.
func Dial(main string) (*Dolt, error) {
	return DialSock(Sock(main), main, Options{Prefix: filepath.Base(main)})
}

// DialSock is Dial on the socket sock, with the version handshake against the main checkout main's pin, and o's
// options.
func DialSock(sock, main string, o Options) (*Dolt, error) {
	return dial(sock, dialConfig{prefix: o.Prefix, now: o.Now, withDB: true, handshake: func(service string) error {
		return CheckVersion(service, buildinfo.Version, main)
	}})
}

// DialSetup is Dial without the work database selected, for pm init's CALL pm_setup() on a clone with no store yet.
func DialSetup(main string) (*Dolt, error) {
	return dial(Sock(main), dialConfig{prefix: filepath.Base(main), handshake: func(service string) error {
		return CheckVersion(service, buildinfo.Version, main)
	}})
}

// CheckVersion refuses a service whose version differs from the command's, naming the fix: a stale service when the
// main checkout pins the command's version, else a checkout that pins another version than the main checkout.
func CheckVersion(service, command, main string) error {
	if service == command {
		return nil
	}
	path := filepath.Join(main, config.Rel)
	pin := ""
	if c, err := config.Read(main); err == nil {
		pin = c.Version
	}
	if pin == command {
		return fmt.Errorf("the pm service runs pm %s, not pm %s: run pm service restart", service, command)
	}
	return fmt.Errorf("this checkout pins pm %s, but the clone's pm service runs pm %s, which %s pins: run it from a "+
		"checkout that pins pm %s", command, service, path, service)
}

func notAnswering(sock string) error {
	return fmt.Errorf("the pm service does not answer on %s; pm reaches the work store only through it: run pm "+
		"service restart", sock)
}

// dial opens one connection to the socket and holds it.
func dial(sock string, c dialConfig) (*Dolt, error) {
	cfg := mysql.NewConfig()
	cfg.User, cfg.Net, cfg.Addr = "pm", "unix", sock
	cfg.ParseTime, cfg.Loc, cfg.Timeout = true, time.UTC, ConnectTimeout
	cfg.InterpolateParams = true // one round trip per statement, not a prepare, an execute and a close
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, fmt.Errorf("work store: %w", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	cc, cancel := context.WithTimeout(ctx, ConnectTimeout)
	conn, err := db.Conn(cc)
	cancel()
	if err != nil {
		db.Close()
		return nil, notAnswering(sock)
	}
	d := &Dolt{o: Options{Prefix: c.prefix, Now: c.now}, sock: sock, db: db, conn: conn}
	fail := func(err error) (*Dolt, error) {
		d.Shutdown()
		return nil, err
	}
	if c.handshake != nil {
		var v string
		if err := conn.QueryRowContext(ctx, "SELECT pm_version()").Scan(&v); err != nil {
			return fail(fmt.Errorf("%w (%v)", notAnswering(sock), err))
		}
		if err := c.handshake(v); err != nil {
			return fail(err)
		}
	}
	if c.withDB {
		if _, err := conn.ExecContext(ctx, "USE `"+dbName+"`"); err != nil {
			var me *mysql.MySQLError
			if errors.As(err, &me) && me.Number == 1049 { // unknown database
				return fail(ErrNoStore)
			}
			return fail(d.broken(err))
		}
	}
	return d, nil
}

// Shutdown closes the connection; the command reaches the store no more.
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
	for _, err := range errs {
		if err != nil && !errors.Is(err, driver.ErrBadConn) && !errors.Is(err, mysql.ErrInvalidConn) &&
			!errors.Is(err, sql.ErrConnDone) {
			return fmt.Errorf("work store: close: %w", err)
		}
	}
	return nil
}

// ErrBroken is a connection to the service that broke mid-command; a write in flight has an unknown outcome.
var ErrBroken = errors.New("the connection to the pm service broke")

// broken names a broken connection as such: the command does not reconnect, since a write cut off in DOLT_COMMIT
// may or may not have landed. Any other error is returned as it is.
func (d *Dolt) broken(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, driver.ErrBadConn) || errors.Is(err, mysql.ErrInvalidConn) || errors.Is(err, sql.ErrConnDone) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) {
		return fmt.Errorf("%w (%v); a write in flight may or may not have landed: check with pm show, then pm service "+
			"status", ErrBroken, err)
	}
	return err
}

// serialization is whether err is Dolt's serialization failure: the transaction conflicts with one another client
// committed meanwhile (MySQL error 1213, SQLSTATE 40001).
func serialization(err error) bool {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number == 1213 || string(me.SQLState[:]) == "40001"
	}
	return false
}

// retryable is whether a write's failure leaves nothing written and means only that it lost a race with a write
// outside the write lock: Dolt's serialization failure, or its "dataset head is not ancestor of commit"
// (ErrMergeNeeded), which a commit gets when a fast-forward moved the branch under it. Dolt returns ErrMergeNeeded
// only from inside its root update's compare-and-swap (store/datas database_common.go: FastForward, doCommit,
// doCommitWithWorkingSet), before it writes the new root, so nothing of that commit landed.
func retryable(err error) bool {
	return serialization(err) || err != nil && strings.Contains(err.Error(), "dataset head is not ancestor of commit")
}

// now is the store's clock: UTC, whole seconds.
func (d *Dolt) now() time.Time {
	t := time.Now()
	if d.o.Now != nil {
		t = d.o.Now()
	}
	return t.UTC().Truncate(time.Second)
}

// opCtx is the context of the operation running now, or the background.
func (d *Dolt) opCtx() context.Context {
	if d.op != nil {
		return d.op
	}
	return ctx
}

func execAll(tx *sql.Tx, stmts []string) error {
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("work store: %w", err)
		}
	}
	return nil
}

// CreateStore makes the work database on a connection with none selected (DialSetup), at SchemaVersion; pm init's
// setup and pm init --import-bd run it on a clone with no store yet. It fails when the database is there.
func (d *Dolt) CreateStore() error {
	if _, err := d.conn.ExecContext(ctx, "CREATE DATABASE `"+dbName+"`"); err != nil {
		return fmt.Errorf("work store: create the database: %w", d.broken(err))
	}
	if _, err := d.conn.ExecContext(ctx, "USE `"+dbName+"`"); err != nil {
		return fmt.Errorf("work store: %w", d.broken(err))
	}
	if err := d.inTx("pm: create the work store at schema version 1", false,
		func(tx *sql.Tx) error { return execAll(tx, schema) }); err != nil {
		return err
	}
	return d.migrate()
}

// HasStore is whether the service holds a work database, on a connection with none selected.
func (d *Dolt) HasStore() (bool, error) {
	var n int
	err := d.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?",
		dbName).Scan(&n)
	return n > 0, d.broken(err)
}

// UseStore selects the work database on a connection that had none.
func (d *Dolt) UseStore() error {
	_, err := d.conn.ExecContext(ctx, "USE `"+dbName+"`")
	return d.broken(err)
}

// migrate checks the schema version: a newer one is refused, an older one is brought up to SchemaVersion. Only the
// service migrates, at its start.
func (d *Dolt) migrate() error {
	var v int
	if err := d.conn.QueryRowContext(ctx, "SELECT version FROM schema_version WHERE one = 1").Scan(&v); err != nil {
		return fmt.Errorf("work store: read the schema version: %w", d.broken(err))
	}
	if v > SchemaVersion {
		return fmt.Errorf("work store: its schema is version %d, newer than this pm's %d: run pm upgrade", v,
			SchemaVersion)
	}
	for ; v < SchemaVersion; v++ {
		steps := append(slices.Clone(migrations[v-1]), fmt.Sprintf("UPDATE schema_version SET version = %d", v+1))
		if err := d.inTx(fmt.Sprintf("pm: migrate the work store schema to version %d", v+1), false,
			func(tx *sql.Tx) error { return execAll(tx, steps) }); err != nil {
			return err
		}
	}
	return nil
}

// stamp sets the write stamp to a fresh UUID, so this transaction conflicts with any other write committed while it
// ran (Concurrent writers).
func setStamp(tx *sql.Tx) error {
	id, err := newUUID()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE write_stamp SET txn = ? WHERE one = 1", id); err != nil {
		return fmt.Errorf("work store: set the write stamp: %w", err)
	}
	return nil
}

// WriteLockWait bounds a write's wait for the store's write lock: a write that waits that long fails hard, naming
// it, and writes nothing. A variable so the tests can shorten it.
var WriteLockWait = 60 * time.Second

// Waits counts this process's writes, the time they waited for the write lock and the longest wait, in nanoseconds:
// what the concurrency benchmark reports.
var Waits struct{ Writes, Total, Max atomic.Int64 }

// lockWrites takes the store's write lock (lock.go) for the write what, waiting at most WriteLockWait; the returned
// func releases it.
func (d *Dolt) lockWrites(what string) (func(), error) {
	start := time.Now()
	ms := max(1, (WriteLockWait+time.Millisecond-1)/time.Millisecond) // rounded up: never a wait of 0
	if _, err := d.conn.ExecContext(ctx, "CALL pm_lock(?, ?, ?)", int64(ms), int64(os.Getpid()),
		strings.TrimPrefix(what, "pm: ")); err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) && strings.Contains(me.Message, errLockTimeout.Error()) {
			holder := "unknown"
			if _, after, ok := strings.Cut(me.Message, "held by "); ok {
				holder = after
			}
			return nil, fmt.Errorf("work store: %s waited %s for the store's write lock, which a live pm process "+
				"holds (%s); nothing was written. If that process hangs (stopped, or in a debugger), stop it; then "+
				"run the command again", strings.TrimPrefix(what, "pm: "), WriteLockWait, holder)
		}
		return nil, fmt.Errorf("work store: take the write lock: %w", d.procError(err))
	}
	waited := time.Since(start).Nanoseconds()
	Waits.Writes.Add(1)
	Waits.Total.Add(waited)
	for m := Waits.Max.Load(); waited > m && !Waits.Max.CompareAndSwap(m, waited); {
		m = Waits.Max.Load()
	}
	return func() { _, _ = d.conn.ExecContext(ctx, "CALL pm_unlock()") }, nil
}

// clean refuses a working set that differs from main's head at the start of a write, under the write lock: no pm
// write leaves one, so it is a change outside pm or a merge cut short (Dolt's fast-forward moves the head and the
// working set in two steps), which the write's DOLT_COMMIT('-A') would commit as if it were its own, a silent revert.
func clean(tx *sql.Tx, what string) error {
	rows, err := tx.QueryContext(ctx, "SELECT table_name FROM dolt_status ORDER BY table_name")
	if err != nil {
		return fmt.Errorf("work store: %w", err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return fmt.Errorf("work store: %w", err)
		}
		tables = append(tables, t)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("work store: %w", err)
	}
	if len(tables) > 0 {
		return fmt.Errorf("work store: %s found the store's working set differing from its head in %s, which no pm "+
			"write leaves (a change outside pm, or a merge cut short); nothing was written. Look at it with any MySQL "+
			"client on the pm service's socket (SELECT * FROM dolt_diff_<table>), and drop it there with CALL "+
			"DOLT_RESET('--hard'), which keeps every commit", strings.TrimPrefix(what, "pm: "), strings.Join(tables, ", "))
	}
	return nil
}

// inTx runs fn in one transaction that ends in a Dolt commit with msg, under the store's write lock; on any error
// nothing lands. A write that changed no row (a claim again by its holder within one second) makes no Dolt commit and
// sets no stamp. With stamped, the write sets the write stamp: the lock makes pm's writes run one at a time, and the
// stamp turns a write that raced one outside the lock (a SQL client past pm) into Dolt's serialization failure, which
// fails the write hard, rather than into a broken invariant.
func (d *Dolt) inTx(msg string, stamped bool, fn func(tx *sql.Tx) error) error {
	if d.conn == nil {
		return errors.New("work store: closed")
	}
	unlock, err := d.lockWrites(msg)
	if err != nil {
		return err
	}
	defer unlock()
	err = d.txOnce(msg, stamped, fn)
	if retryable(err) {
		return fmt.Errorf("work store: %s conflicted with a write that did not take the store's write lock (a SQL "+
			"client past pm?); nothing was written: run the command again", strings.TrimPrefix(msg, "pm: "))
	}
	return err
}

func (d *Dolt) txOnce(msg string, stamped bool, fn func(tx *sql.Tx) error) error {
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("work store: %w", d.broken(err))
	}
	fail := func(err error) error {
		_ = tx.Rollback()
		return d.broken(err)
	}
	if err := clean(tx, msg); err != nil {
		return fail(err)
	}
	if err := fn(tx); err != nil {
		return fail(err)
	}
	var changed int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM dolt_status").Scan(&changed); err != nil {
		return fail(fmt.Errorf("work store: %w", err))
	}
	if changed == 0 {
		return d.broken(tx.Rollback())
	}
	if stamped {
		if err := setStamp(tx); err != nil {
			return fail(err)
		}
	}
	if _, err := tx.ExecContext(ctx, "CALL DOLT_COMMIT('-Am', ?)", msg); err != nil {
		if retryable(err) {
			_ = tx.Rollback()
			return err
		}
		return fail(fmt.Errorf("work store: commit: %w", err))
	}
	if err := tx.Commit(); err != nil {
		if retryable(err) {
			return err
		}
		return fmt.Errorf("work store: commit: %w", d.broken(err))
	}
	return nil
}

// remoteCall runs a statement that reaches the git remote (DOLT_FETCH, DOLT_PUSH, DOLT_CLONE) on a connection of its own, bounded
// by c. go-sql-driver drops a connection whose context ends mid-statement; dropping this one leaves d's connection,
// and the write lock or transaction it may hold, as they were, so the outcome can still be checked. (The server may
// still finish the dropped statement: Dolt kills its git once it sees the connection gone.)
func (d *Dolt) remoteCall(c context.Context, q string, args ...any) error {
	return d.remoteCallOn(c, true, q, args...)
}

// remoteCallOn is remoteCall on a connection with the work database selected when withDB.
func (d *Dolt) remoteCallOn(c context.Context, withDB bool, q string, args ...any) error {
	side, err := dial(d.sock, dialConfig{withDB: withDB})
	if err != nil {
		return err
	}
	defer side.Shutdown()
	if _, err := side.conn.ExecContext(c, q, args...); err != nil {
		if c.Err() != nil {
			return fmt.Errorf("%w (%v)", c.Err(), err)
		}
		return err
	}
	return nil
}

// Mark is the store's change mark: main's HEAD commit, which every write moves.
func (d *Dolt) Mark() (string, error) {
	var h string
	if err := d.conn.QueryRowContext(ctx, "SELECT HASHOF('main')").Scan(&h); err != nil {
		return "", fmt.Errorf("work store: %w", d.broken(err))
	}
	return h, nil
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
	r, err := q.QueryContext(ctx,
		"SELECT id, item_id, kind, author, text, created_at FROM comments ORDER BY item_id, pos, created_at, id")
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
	tx, err := d.conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("work store: %w", d.broken(err))
	}
	items, err := loadChecked(tx)
	if err != nil {
		_ = tx.Rollback()
		return nil, d.broken(err)
	}
	return items, d.broken(tx.Commit())
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
	return d.inTx(msg, true, func(tx *sql.Tx) error {
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

// Import writes items into an empty store as one transaction and one Dolt commit (pm init --import-bd and --import).
// It refuses a store that holds any item, and items that fail Check.
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
// number one above its project's highest, status open. A child minted in a store with a remote goes through the
// compare-and-swap on the remote, which the service runs (CALL pm_create(?), createShared); a project's random root id,
// or any id in a store with no remote, is minted from this store copy by an ordinary write.
func (d *Dolt) Create(n New) (Item, error) {
	if n.Type != Project {
		if _, ok, err := d.remoteURL(); err != nil {
			return Item{}, err
		} else if ok {
			return d.callCreate(n)
		}
	}
	return d.createLocal(n)
}

// createLocal mints the item's id from this store copy and writes it as one transaction.
func (d *Dolt) createLocal(n New) (Item, error) {
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

// Move gives a task or a need a new parent (pm task move takes any open item that is no project or sprint); its id
// stays.
func (d *Dolt) Move(id, parent string) error {
	return d.update("move", id, func(it *Item, _ *Index) error {
		if it.Type != Task && it.Type != Need {
			return itemError(id, "is a %s; only a task or a need moves", it.Type)
		}
		it.Parent = parent
		return nil
	})
}

// Claim makes h the holder, as a compare-and-set in one transaction: it succeeds when the item has no holder, is held
// by h.Session, or is held by a session that live reports not live; it sets StartedAt on the first claim. The claim
// time is the store's clock.
func (d *Dolt) Claim(id string, h Holder, live func(session string) bool) error {
	// whether the holder is live is read before the write lock: it reads session transcripts on disk, slow work
	// that no write should wait on; only a holder that changed meanwhile is read again under the lock
	known := map[string]bool{}
	if items, err := d.Get(id); err == nil && items[0].Holder != nil && items[0].Holder.Session != h.Session {
		known[items[0].Holder.Session] = live(items[0].Holder.Session)
	}
	isLive := func(session string) bool {
		if v, ok := known[session]; ok {
			return v
		}
		return live(session)
	}
	return d.update("claim", id, func(it *Item, _ *Index) error {
		if it.Status != Open {
			return itemError(id, "is closed")
		}
		if h.Session == "" {
			return itemError(id, "cannot be claimed without a session")
		}
		if o := it.Holder; o != nil && o.Session != h.Session && isLive(o.Session) {
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
		if u.RaisedInbox != nil || u.RaisedHost != nil {
			if it.Need.RaisedBy == nil {
				return itemError(id, "was raised by no session, so it has no inbox")
			}
			if u.RaisedInbox != nil {
				it.Need.RaisedBy.Inbox = *u.RaisedInbox
			}
			if u.RaisedHost != nil {
				it.Need.RaisedBy.Host = *u.RaisedHost
			}
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
