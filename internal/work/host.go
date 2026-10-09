package work

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/dolthub/dolt/go/cmd/dolt/commands/engine"
	"github.com/dolthub/dolt/go/cmd/dolt/doltversion"
	"github.com/dolthub/dolt/go/libraries/doltcore/dbfactory"
	"github.com/dolthub/dolt/go/libraries/doltcore/env"
	"github.com/dolthub/dolt/go/libraries/doltcore/sqle"
	"github.com/dolthub/dolt/go/libraries/doltcore/sqlserver"
	doltconfig "github.com/dolthub/dolt/go/libraries/utils/config"
	"github.com/dolthub/dolt/go/libraries/utils/filesys"
	embedded "github.com/dolthub/driver/v2"
	"github.com/dolthub/go-mysql-server/server"
	gmssql "github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/types"
	"github.com/dolthub/vitess/go/mysql"
	"github.com/sirupsen/logrus"
)

// The host: the work store held open by the pm service and served on a Unix socket (the pm-go page, "Store access:
// the service holds the store"). Only pm service run calls Host; every pm command, and the service's own site,
// writer and loops, reach the store as SQL clients of the socket (Dial). Host loads the Dolt engine as
// dolthub/driver builds it, but keeps it for the service's whole life and serves it with go-mysql-server's server on
// a listener of its own, so no TCP port serves SQL.

const (
	// SockName is the socket's file in <main checkout>/.pm/run.
	SockName = "work.sock"
	// sockMax is the longest socket path the kernel takes (sun_path holds 104 bytes on macOS, 108 on Linux, with
	// the terminating NUL); the smaller one, so a clone that works on one works on both.
	sockMax = 103
)

// Sock is the socket of the clone whose main checkout is main.
func Sock(main string) string { return filepath.Join(main, ".pm", "run", SockName) }

// Ops are what the service runs for pm_sync() and pm_setup(), given a connection to its own socket: the sync's and
// the setup's steps that need the clone's config and git (the remote's URL, git ls-remote), which the work package
// does not know. Each returns its lines; an error is the procedure's error, its text the command's.
type Ops struct {
	Sync  func(ctx context.Context, d *Dolt) ([]string, error)
	Setup func(ctx context.Context, d *Dolt) ([]string, error)
}

// HostOptions configure a host.
type HostOptions struct {
	Main    string // the main checkout: the store at .pm/store/work, the socket at .pm/run/work.sock
	Version string // what pm_version() returns: the service's build
	Ops     Ops
	Now     func() time.Time // the clock its own writes stamp with; nil: the clock
}

// Host is the open work store and its server.
type Host struct {
	o      HostOptions
	dir    string
	sock   string
	mrEnv  *env.MultiRepoEnv
	se     *engine.SqlEngine
	srv    *server.Server
	served chan error
	ready  chan struct{} // closed once the schema is current: pm_version() answers only then
	slot   chan struct{} // the operation slot: one sync, create or setup at a time
	mu     sync.Mutex    // guards holder and since
	holder string        // the operation in the slot, and since when
	since  time.Time
	gcMu   sync.Mutex // one garbage collection at a time; it takes no slot (GC)
	writes writeLock  // the store's write lock (lock.go)
	closed bool
}

// The bounds of the operations in the slot, the wait for the slot included: a hung fetch or push ends at its bound
// and frees the slot, and a caller waiting behind it fails hard at its own, naming what holds the slot. Variables so
// the tests can shorten them.
var (
	// SyncTimeout bounds a sync: the service's loop's, and pm_sync()'s (pm sync, pm push).
	SyncTimeout = 120 * time.Second
	// CreateTimeout bounds a child create, pm_create(): its pull and push, up to casAttempts times.
	CreateTimeout = 180 * time.Second
	// SetupTimeout bounds pm init's pm_setup(): a clone of the remote's store at most.
	SetupTimeout = 300 * time.Second
)

// Host loads the store in <main>/.pm/store/work (an empty directory when there is none yet: pm_setup makes it),
// migrates an older schema, and serves it on <main>/.pm/run/work.sock until Close. It fails hard when another
// process holds the store or the socket path is too long for the kernel.
func NewHost(o HostOptions) (*Host, error) {
	dir, _ := Locations(o.Main)
	h := &Host{o: o, dir: dir, sock: Sock(o.Main), served: make(chan error, 1), ready: make(chan struct{}),
		slot: make(chan struct{}, 1)}
	if n := len(h.sock); n > sockMax {
		return nil, fmt.Errorf("the pm service's socket %s is %d bytes, over this system's limit of %d; move the clone "+
			"to a shorter path", h.sock, n, sockMax)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("work store: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(h.sock), 0o755); err != nil {
		return nil, fmt.Errorf("work store: %w", err)
	}
	if err := os.Setenv(infoBranchEnv, ""); err != nil { // read when the engine first reaches the remote
		return nil, fmt.Errorf("work store: %w", err)
	}
	// The server logs every connection and every failed query (a refusal) at info and warning; pm's commands report
	// their own errors, so only the server's errors reach the service log.
	logrus.SetLevel(logrus.ErrorLevel)
	if err := h.load(); err != nil {
		return nil, err
	}
	if err := h.listen(); err != nil {
		h.se.Close()
		return nil, err
	}
	if err := h.migrate(); err != nil {
		h.Close()
		return nil, err
	}
	close(h.ready)
	return h, nil
}

// load opens the engine on the store directory, as dolthub/driver's connector does, failing fast when another
// process holds the store's lock (the driver's FailOnJournalLockTimeout) rather than opening it read-only.
func (h *Host) load() error {
	cfg := doltconfig.NewMapConfig(map[string]string{doltconfig.UserNameKey: "pm", doltconfig.UserEmailKey: "pm@localhost"})
	fs, err := filesys.LocalFS.WithWorkingDir(h.dir)
	if err != nil {
		return fmt.Errorf("work store: %w", err)
	}
	params := map[string]any{dbfactory.DisableSingletonCacheParam: struct{}{},
		dbfactory.FailOnJournalLockTimeoutParam: struct{}{}}
	held := func(err error) error {
		return fmt.Errorf("work store %s: another process holds it (a second pm service?): %w", h.dir, err)
	}
	mrEnv, err := embedded.LoadMultiEnvFromDir(ctx, cfg, fs, ".", doltversion.Version, params)
	if err != nil {
		return held(err)
	}
	var readOnly error
	mrEnv.Iter(func(name string, d *env.DoltEnv) (bool, error) {
		if ro, err := d.IsAccessModeReadOnly(ctx); err != nil || ro {
			readOnly = fmt.Errorf("database %s opened read-only: its lock is taken", name)
			return true, nil
		}
		return false, nil
	})
	if readOnly != nil {
		mrEnv.Close(ctx)
		return held(readOnly)
	}
	se, err := engine.NewSqlEngine(ctx, mrEnv, &engine.SqlEngineConfig{ServerUser: "root", Autocommit: true,
		DBLoadParams: params})
	if err != nil {
		mrEnv.Close(ctx)
		return held(err)
	}
	h.mrEnv, h.se = mrEnv, se
	return nil
}

// listen removes a socket a crash left (safe: the engine is held, so no other service serves this store), listens
// with mode 0600, registers pm_version() and the operations, and serves.
func (h *Host) listen() error {
	if err := os.Remove(h.sock); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("work store: remove the old socket: %w", err)
	}
	old := syscall.Umask(0o177)
	ln, err := net.Listen("unix", h.sock)
	syscall.Umask(old)
	if err != nil {
		return fmt.Errorf("work store: listen on %s: %w", h.sock, err)
	}
	gms := h.se.GetUnderlyingEngine()
	gms.Analyzer.Catalog.RegisterFunction(gmssql.NewEmptyContext(),
		gmssql.NewFunction0("pm_version", func(*gmssql.Context) gmssql.Expression { return versionExpr{h} }))
	p, ok := gms.Analyzer.Catalog.DbProvider.(*sqle.DoltDatabaseProvider)
	if !ok {
		ln.Close()
		return fmt.Errorf("work store: the engine's provider is a %T, not Dolt's", gms.Analyzer.Catalog.DbProvider)
	}
	line := gmssql.Schema{{Name: "line", Type: types.LongText}}
	h.writes.alive = h.alive
	p.Register(gmssql.ExternalStoredProcedureDetails{Name: "pm_lock", Function: h.procLock})
	p.Register(gmssql.ExternalStoredProcedureDetails{Name: "pm_unlock", Function: h.procUnlock})
	p.Register(gmssql.ExternalStoredProcedureDetails{Name: "pm_sync", Schema: line, Function: h.procSync})
	p.Register(gmssql.ExternalStoredProcedureDetails{Name: "pm_setup", Schema: line, Function: h.procSetup})
	p.Register(gmssql.ExternalStoredProcedureDetails{Name: "pm_create", Schema: gmssql.Schema{{Name: "item",
		Type: types.LongText}}, Function: h.procCreate})
	sb := func(c context.Context, conn *mysql.Conn, addr string) (gmssql.Session, error) {
		bs, err := gmssql.BaseSessionFromConnection(c, conn, addr)
		if err != nil {
			return nil, err
		}
		return h.se.NewDoltSession(c, bs)
	}
	srv, err := server.NewServer(server.Config{Protocol: "unix", Listener: ln, Version: "8.0.33-pm-" + h.o.Version},
		gms, h.se.ContextFactory, sb, nil)
	if err != nil {
		ln.Close()
		return fmt.Errorf("work store: serve: %w", err)
	}
	if err := sqlserver.SetRunningServer(srv); err != nil {
		srv.Close()
		return fmt.Errorf("work store: serve: %w", err)
	}
	h.srv = srv
	go func() { h.served <- srv.Start() }()
	return nil
}

// migrate brings an older schema up to SchemaVersion through a connection of the host's own; only the service
// migrates. A store with a newer schema fails the start: an older pm does not write a newer schema.
func (h *Host) migrate() error {
	d, err := h.client(true)
	if errors.Is(err, ErrNoStore) {
		return nil
	}
	if err != nil {
		return err
	}
	defer d.Shutdown()
	return d.migrate()
}

// Close stops the server, which unlinks the socket, then closes the engine.
func (h *Host) Close() error {
	if h.closed {
		return nil
	}
	h.closed = true
	var errs []error
	if h.srv != nil {
		sqlserver.UnsetRunningServer()
		errs = append(errs, h.srv.Close())
		<-h.served
	}
	_ = os.Remove(h.sock)
	if h.se != nil {
		errs = append(errs, h.se.Close())
	}
	if err := errors.Join(errs...); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("work store: close: %w", err)
	}
	return nil
}

// Sock is the socket the host serves on.
func (h *Host) Sock() string { return h.sock }

// client is a connection to the host's own socket, with the work database selected when withDB; ErrNoStore when
// withDB and there is none yet. It skips the version handshake: the host is its own version.
func (h *Host) client(withDB bool) (*Dolt, error) {
	return dial(h.sock, dialConfig{prefix: filepath.Base(h.o.Main), now: h.o.Now, withDB: withDB})
}

// op runs the operation name in the host's operation slot, on a connection of the host's own, bounded by timeout from
// now (or c's deadline, if sooner), the wait for the slot included. A caller still waiting at its bound fails hard,
// naming the operation in the slot. What the bound stops is the operation's fetch and push (remoteCall): the client
// drops their connection, the operation fails and frees the slot. The server may still finish the dropped statement,
// until Dolt kills its git, so a dying fetch or push can overlap the next operation's; no other step is cut, and a
// local merge in progress runs to its end, past the bound (Dolt's fast-forward is two root updates, and a merge cut
// between them would leave the working set behind the head).
func (h *Host) op(c context.Context, name string, timeout time.Duration, withDB bool, fn func(d *Dolt) error) error {
	c, cancel := context.WithTimeout(c, timeout)
	defer cancel()
	select {
	case h.slot <- struct{}{}:
	case <-c.Done():
		h.mu.Lock()
		holder, since := h.holder, h.since
		h.mu.Unlock()
		return fmt.Errorf("work store: the %s waited %s for the pm service's %s, running for %s, and gave up; "+
			"nothing was written: run the command again, and see pm service logs if it recurs", name,
			timeout.Round(time.Millisecond), holder, time.Since(since).Round(time.Millisecond))
	}
	h.mu.Lock()
	h.holder, h.since = name, time.Now()
	h.mu.Unlock()
	defer func() { <-h.slot }()
	d, err := h.client(withDB)
	if err != nil {
		return err
	}
	d.op = c
	err = fn(d)
	if err != nil && c.Err() != nil {
		err = fmt.Errorf("work store: the %s did not finish within %s and was stopped: %w", name,
			timeout.Round(time.Millisecond), err)
	}
	return errors.Join(err, d.Shutdown())
}

// holding is the operation in the slot, "" when none is.
func (h *Host) holding() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.slot) == 0 {
		return ""
	}
	return h.holder
}

// Sync is the sync the service's loop runs every pmsync.Interval and pm_sync() runs on a command's call, within
// SyncTimeout.
func (h *Host) Sync(c context.Context) ([]string, error) {
	if h.o.Ops.Sync == nil {
		return nil, errors.New("work store: this host runs no sync")
	}
	var lines []string
	err := h.op(c, "sync", SyncTimeout, true, func(d *Dolt) (err error) {
		lines, err = h.o.Ops.Sync(d.op, d)
		return err
	})
	return lines, err
}

// GC collects the store's garbage (CALL DOLT_GC()), online: Dolt's session-aware safepoints let open connections go
// on. It deletes no item and squashes no commit. It takes no operation slot: the collection waits for every session
// in the middle of a statement to end it, and a command in CALL pm_sync() or pm_create() is in the middle of one
// while it waits for the slot, so a collection that held the slot would wait for it until its timeout. One
// collection runs at a time (Dolt allows one).
func (h *Host) GC(c context.Context) error {
	h.gcMu.Lock()
	defer h.gcMu.Unlock()
	d, err := h.client(true)
	if err != nil {
		return err
	}
	_, err = d.conn.ExecContext(c, "CALL DOLT_GC()")
	if err != nil {
		err = fmt.Errorf("work store: gc: %w", d.broken(err))
	}
	return errors.Join(err, d.Shutdown())
}

// Mark is the store's change mark: main's HEAD commit, which moves on every write; "" when there is no store yet.
func (h *Host) Mark() (string, error) {
	d, err := h.client(true)
	if errors.Is(err, ErrNoStore) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer d.Shutdown()
	return d.Mark()
}

// alive is whether the server still has the session: its connection has not dropped.
func (h *Host) alive(session uint32) bool {
	if h.srv == nil {
		return true
	}
	found := false
	_ = h.srv.SessionManager().Iter(func(s gmssql.Session) (bool, error) {
		found = s.ID() == session
		return found, nil
	})
	return found
}

// procLock is CALL pm_lock(ms, pid, what): the store's write lock for the calling connection, whose client is
// process pid making the write what, waiting at most ms milliseconds.
func (h *Host) procLock(c *gmssql.Context, ms, pid int64, what string) (gmssql.RowIter, error) {
	if err := h.writes.lock(c.Session.ID(), pid, what, time.Duration(ms)*time.Millisecond, c.Done()); err != nil {
		return nil, fmt.Errorf("pm_lock: %w", err)
	}
	return nil, nil
}

// procUnlock is CALL pm_unlock(): the calling connection releases the store's write lock.
func (h *Host) procUnlock(c *gmssql.Context) (gmssql.RowIter, error) {
	if err := h.writes.unlock(c.Session.ID()); err != nil {
		return nil, fmt.Errorf("pm_unlock: %w", err)
	}
	return nil, nil
}

func lineRows(lines []string) gmssql.RowIter {
	rows := make([]gmssql.Row, len(lines))
	for i, l := range lines {
		rows[i] = gmssql.Row{l}
	}
	return gmssql.RowsToRowIter(rows...)
}

func (h *Host) procSync(c *gmssql.Context) (gmssql.RowIter, error) {
	lines, err := h.Sync(c)
	if err != nil {
		return nil, err
	}
	return lineRows(lines), nil
}

func (h *Host) procSetup(c *gmssql.Context) (gmssql.RowIter, error) {
	if h.o.Ops.Setup == nil {
		return nil, errors.New("work store: this host runs no setup")
	}
	var lines []string
	err := h.op(c, "setup", SetupTimeout, false, func(d *Dolt) (err error) {
		lines, err = h.o.Ops.Setup(d.op, d)
		return err
	})
	if err != nil {
		return nil, err
	}
	return lineRows(lines), nil
}

func (h *Host) procCreate(c *gmssql.Context, spec string) (gmssql.RowIter, error) {
	var n New
	if err := json.Unmarshal([]byte(spec), &n); err != nil {
		return nil, fmt.Errorf("work store: pm_create: %w", err)
	}
	var made Item
	err := h.op(c, "create of a "+string(n.Type)+" under "+n.Parent, CreateTimeout, true, func(d *Dolt) (err error) {
		made, err = d.createShared(n)
		return err
	})
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(made)
	if err != nil {
		return nil, err
	}
	return gmssql.RowsToRowIter(gmssql.Row{string(out)}), nil
}

// versionExpr is pm_version(): the service's build, answered once the host is ready, so no command uses the store
// before its schema is current.
type versionExpr struct{ h *Host }

var _ gmssql.Expression = versionExpr{}

func (v versionExpr) Resolved() bool                   { return true }
func (v versionExpr) String() string                   { return "pm_version()" }
func (v versionExpr) Type(*gmssql.Context) gmssql.Type { return types.LongText }
func (v versionExpr) IsNullable(*gmssql.Context) bool  { return false }
func (v versionExpr) Children() []gmssql.Expression    { return nil }
func (v versionExpr) CollationCoercibility(*gmssql.Context) (gmssql.CollationID, byte) {
	return gmssql.Collation_Default, 4
}
func (v versionExpr) WithChildren(_ *gmssql.Context, c ...gmssql.Expression) (gmssql.Expression, error) {
	if len(c) != 0 {
		return nil, gmssql.ErrInvalidChildrenNumber.New(v, len(c), 0)
	}
	return v, nil
}
func (v versionExpr) Eval(c *gmssql.Context, _ gmssql.Row) (any, error) {
	select {
	case <-v.h.ready:
		return v.h.o.Version, nil
	case <-c.Done():
		return nil, c.Err()
	}
}
