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
	opMu   sync.Mutex    // one operation at a time: sync, create, setup, gc
	closed bool
}

// Host loads the store in <main>/.pm/store/work (an empty directory when there is none yet: pm_setup makes it),
// migrates an older schema, and serves it on <main>/.pm/run/work.sock until Close. It fails hard when another
// process holds the store or the socket path is too long for the kernel.
func NewHost(o HostOptions) (*Host, error) {
	dir, _ := Locations(o.Main)
	h := &Host{o: o, dir: dir, sock: Sock(o.Main), served: make(chan error, 1), ready: make(chan struct{})}
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

// op runs one operation under opMu, on a connection of the host's own.
func (h *Host) op(withDB bool, fn func(d *Dolt) error) error {
	h.opMu.Lock()
	defer h.opMu.Unlock()
	d, err := h.client(withDB)
	if err != nil {
		return err
	}
	return errors.Join(fn(d), d.Shutdown())
}

// Sync is the sync the service's loop runs every pmsync.Interval and pm_sync() runs on a command's call.
func (h *Host) Sync(c context.Context) ([]string, error) {
	if h.o.Ops.Sync == nil {
		return nil, errors.New("work store: this host runs no sync")
	}
	var lines []string
	err := h.op(true, func(d *Dolt) (err error) {
		lines, err = h.o.Ops.Sync(c, d)
		return err
	})
	return lines, err
}

// GC collects the store's garbage (CALL DOLT_GC()), online: Dolt's session-aware safepoints let open connections go
// on. It deletes no item and squashes no commit.
func (h *Host) GC(c context.Context) error {
	return h.op(true, func(d *Dolt) error {
		if _, err := d.conn.ExecContext(c, "CALL DOLT_GC()"); err != nil {
			return fmt.Errorf("work store: gc: %w", d.broken(err))
		}
		return nil
	})
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
	err := h.op(false, func(d *Dolt) (err error) {
		lines, err = h.o.Ops.Setup(c, d)
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
	err := h.op(true, func(d *Dolt) (err error) {
		d.op = c
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
