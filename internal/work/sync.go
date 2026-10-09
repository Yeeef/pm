package work

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Sync through the repo's git remote, as the work-store page's Storage section gives it (Remote, Sync, Merge, Cycle
// check, Ahead/behind, Child id compare-and-swap) and its Ids section the compare-and-swap. Dolt keeps the store under
// a ref of pm's own on the git remote, beside bd's refs/dolt/data; Dolt's visible info branch is turned off, since
// bd's store already writes one on this repo's remote and pm's push would overwrite it.

const (
	// RemoteRef is the ref on the repo's git remote that holds the work store.
	RemoteRef = "refs/pm/work"
	// PushTimeout bounds one push; a push that outlasts it has an unknown outcome (Ids, step 4). It must stay well
	// above Dolt's 1 s read dedup (a fetch within 1 s of the remote's last read in the process is skipped), so the
	// fetch that checks the outcome reads the remote.
	PushTimeout = 60 * time.Second
	remote      = "origin"
	branch      = "main"
	remoteHead  = remote + "/" + branch
	// casAttempts is how many times a child create pulls, mints and pushes before it fails (Ids, step 3); a sync
	// retries its push as often.
	casAttempts = 3
	// infoBranchEnv set empty turns off the branch Dolt force-pushes beside the data ref after each push.
	infoBranchEnv = "DOLT_REMOTE_INFO_BRANCH"
)

var (
	// errRemoteMoved is a push the remote rejected as non-fast-forward: it moved since the pull.
	errRemoteMoved = errors.New("the remote moved since the pull (push rejected as non-fast-forward)")
	// errPushTimeout is a push that outlasted PushTimeout: whether it landed is unknown.
	errPushTimeout = errors.New("the push timed out")
)

// DoltRemoteURL is the Dolt URL of a git remote: an absolute path or file:// URL becomes git+file://, an https:// or
// ssh:// URL git+https:// or git+ssh://, and scp-like user@host:path git+ssh://user@host/path. A git+ URL stays.
func DoltRemoteURL(gitURL string) (string, error) {
	u := strings.TrimSpace(gitURL)
	switch {
	case strings.HasPrefix(u, "git+"):
		return u, nil
	case strings.HasPrefix(u, "/"):
		return "git+file://" + filepath.Clean(u), nil
	case strings.HasPrefix(u, "file://"), strings.HasPrefix(u, "https://"), strings.HasPrefix(u, "http://"),
		strings.HasPrefix(u, "ssh://"):
		return "git+" + u, nil
	}
	if at, colon := strings.Index(u, "@"), strings.Index(u, ":"); at > 0 && colon > at && !strings.Contains(u, "://") {
		path := u[colon+1:]
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		return "git+ssh://" + u[:colon] + path, nil
	}
	return "", fmt.Errorf("work store: %q is no git remote URL pm can sync through (a path, file://, https://, ssh:// "+
		"or user@host:path)", gitURL)
}

// AddRemote points the store at the repo's git remote, under RemoteRef; it fails when the store has a remote.
func (d *Dolt) AddRemote(gitURL string) error {
	u, err := DoltRemoteURL(gitURL)
	if err != nil {
		return err
	}
	if _, ok, err := d.remoteURL(); err != nil {
		return err
	} else if ok {
		return errors.New("work store: it has a remote already")
	}
	if _, err := d.conn.ExecContext(ctx, "CALL DOLT_REMOTE('add', ?, ?, '--ref', ?)", remote, u, RemoteRef); err != nil {
		return fmt.Errorf("work store: add the remote: %w", err)
	}
	return nil
}

// Push pushes the store's branch to its remote under RemoteRef: pm init publishes a store the remote lacks with it.
func (d *Dolt) Push() error {
	if _, ok, err := d.remoteURL(); err != nil {
		return err
	} else if !ok {
		return errors.New("work store: it has no remote to push to")
	}
	return d.pushNow()
}

// Tracking is the store's commits the remote lacks and the remote's commits the store lacks, as of the last fetch or
// push, and whether the store has fetched or pushed the remote's branch at all (false: no counts).
func (d *Dolt) Tracking() (ahead, behind int, tracked bool, err error) {
	var n int
	if err := d.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM dolt_remote_branches WHERE name = ?",
		"remotes/"+remoteHead).Scan(&n); err != nil {
		return 0, 0, false, fmt.Errorf("work store: %w", err)
	}
	if n == 0 {
		return 0, 0, false, nil
	}
	if ahead, err = d.count(d.conn, remoteHead+".."+branch); err != nil {
		return 0, 0, false, err
	}
	if behind, err = d.count(d.conn, branch+".."+remoteHead); err != nil {
		return 0, 0, false, err
	}
	return ahead, behind, true, nil
}

// opCtx is the context of the sync running now, or the background.
func (d *Dolt) opCtx() context.Context {
	if d.op != nil {
		return d.op
	}
	return ctx
}

// remoteURL is the store's remote, and whether it has one; a remote under another ref than RemoteRef fails hard.
func (d *Dolt) remoteURL() (string, bool, error) {
	if d.conn == nil {
		return "", false, errors.New("work store: closed")
	}
	var url, params string
	err := d.conn.QueryRowContext(ctx, "SELECT url, CAST(params AS CHAR) FROM dolt_remotes WHERE name = ?", remote).
		Scan(&url, &params)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("work store: read the remote: %w", err)
	}
	if !strings.Contains(params, `"git_ref": "`+RemoteRef+`"`) {
		return "", false, fmt.Errorf("work store: its remote %s keeps the store under another ref than %s (params %s)",
			url, RemoteRef, params)
	}
	return url, true, nil
}

// CloneStore takes the gate and makes the store in o.Dir a clone of the one on the git remote, under RemoteRef; it
// fails when a store is there or the remote holds none.
func CloneStore(o Options, gitURL string) (*Dolt, error) {
	u, err := DoltRemoteURL(gitURL)
	if err != nil {
		return nil, err
	}
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
	if _, err := d.conn.ExecContext(ctx, "CALL DOLT_CLONE('--remote', ?, '--ref', ?, ?, ?)", remote, RemoteRef, u,
		dbName); err != nil {
		return fail(fmt.Errorf("work store: clone %s: %w", gitURL, err))
	}
	if _, err := d.conn.ExecContext(ctx, "USE `"+dbName+"`"); err != nil {
		return fail(fmt.Errorf("work store: %w", err))
	}
	if err := d.migrate(); err != nil {
		return fail(err)
	}
	return d, nil
}

// SyncResult is what one sync did.
type SyncResult struct {
	Pulled    int        // commits the pull brought in
	Pushed    int        // commits the push sent
	Resolved  int        // items rows both sides changed, resolved by the merge rules
	Overrides []Override // claims a later claim beat
}

// Sync pulls from the remote, resolves conflicts by the merge rules, checks the result and pushes what the remote
// lacks: pm sync, and the pm service every 600 s. A push the remote rejects because it moved pulls again, up to
// casAttempts times. A pull that fails rolls back and leaves the store at its pre-pull commit.
func (d *Dolt) Sync() (SyncResult, error) { return d.SyncContext(ctx) }

// SyncContext is Sync bounded by c: its fetches, merge and pushes stop once c is done (Dolt runs git under the
// query's context), and the sync fails as a pull or push that failed does.
func (d *Dolt) SyncContext(c context.Context) (SyncResult, error) {
	d.op = c
	defer func() { d.op = nil }()
	var res SyncResult
	if _, ok, err := d.remoteURL(); err != nil {
		return res, err
	} else if !ok {
		return res, errors.New("work store: it has no remote to sync with; pm init attaches it to the repo's remote")
	}
	for range casAttempts {
		p, err := d.pull()
		if err != nil {
			return res, err
		}
		res.Pulled += p.Pulled
		res.Resolved += p.Resolved
		res.Overrides = append(res.Overrides, p.Overrides...)
		ahead, err := d.ahead()
		if err != nil {
			return res, err
		}
		if ahead == 0 {
			return res, nil
		}
		err = d.pushNow()
		if err == nil {
			res.Pushed += ahead
			return res, nil
		}
		if !errors.Is(err, errRemoteMoved) {
			return res, err
		}
	}
	return res, fmt.Errorf("work store: sync: %v on each of %d attempts; run pm sync again", errRemoteMoved, casAttempts)
}

// fetch brings the remote's branch into the store's remote-tracking branch, and says whether the remote holds one.
func (d *Dolt) fetch() (bool, error) {
	if _, err := d.conn.ExecContext(d.opCtx(), "CALL DOLT_FETCH(?)", remote); err != nil {
		return false, fmt.Errorf("work store: fetch from the remote: %w", err)
	}
	var n int
	if err := d.conn.QueryRowContext(d.opCtx(), "SELECT COUNT(*) FROM dolt_remote_branches WHERE name = ?",
		"remotes/"+remoteHead).Scan(&n); err != nil {
		return false, fmt.Errorf("work store: %w", err)
	}
	return n > 0, nil
}

// count is the number of commits in a dolt_log range such as "main..origin/main".
func (d *Dolt) count(q querier, rng string) (int, error) {
	rows, err := q.QueryContext(d.opCtx(), "SELECT COUNT(*) FROM dolt_log(?)", rng)
	if err != nil {
		return 0, fmt.Errorf("work store: %w", err)
	}
	defer rows.Close()
	var n int
	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			return 0, fmt.Errorf("work store: %w", err)
		}
	}
	return n, rows.Err()
}

// ahead is the number of local commits the remote lacks, as of the last fetch; all of them when the remote holds no
// store yet.
func (d *Dolt) ahead() (int, error) {
	var n int
	if err := d.conn.QueryRowContext(d.opCtx(), "SELECT COUNT(*) FROM dolt_remote_branches WHERE name = ?",
		"remotes/"+remoteHead).Scan(&n); err != nil {
		return 0, fmt.Errorf("work store: %w", err)
	}
	if n == 0 {
		return d.count(d.conn, branch)
	}
	return d.count(d.conn, remoteHead+".."+branch)
}

// head is the store's HEAD commit.
func (d *Dolt) head() (string, error) {
	var h string
	if err := d.conn.QueryRowContext(d.opCtx(), "SELECT HASHOF('HEAD')").Scan(&h); err != nil {
		return "", fmt.Errorf("work store: %w", err)
	}
	return h, nil
}

// pull fetches and, when the store is behind, merges the remote's branch in one transaction: Dolt merges each cell,
// pm refuses a schema newer than its own, resolves each conflicted items row by the merge rules, clears the holder of
// every closed item (close beats claim), checks every invariant and the blocked_by cycles, and commits. Any failure
// rolls the transaction back and resets the branch to its pre-pull commit: a fast-forward moves the branch outside
// the transaction (Dolt's FastForward commits the working set at once), so the rollback alone would keep it.
func (d *Dolt) pull() (SyncResult, error) {
	var res SyncResult
	has, err := d.fetch()
	if err != nil || !has {
		return res, err
	}
	behind, err := d.count(d.conn, branch+".."+remoteHead)
	if err != nil || behind == 0 {
		return res, err
	}
	res.Pulled = behind
	pre, err := d.head()
	if err != nil {
		return res, err
	}
	tx, err := d.conn.BeginTx(d.opCtx(), nil)
	if err != nil {
		return res, fmt.Errorf("work store: %w", err)
	}
	defer d.conn.ExecContext(ctx, "SET @@dolt_allow_commit_conflicts = 0")
	fail := func(err error) (SyncResult, error) {
		_ = tx.Rollback()
		if rerr := d.reset(pre); rerr != nil {
			return SyncResult{}, fmt.Errorf("work store: pull: %w; and the reset to the pre-pull commit %s failed: %v",
				err, pre, rerr)
		}
		return SyncResult{}, fmt.Errorf("work store: pull: %w; the store stays at its pre-pull commit", err)
	}
	if _, err := tx.ExecContext(d.opCtx(), "SET @@dolt_allow_commit_conflicts = 1"); err != nil {
		return fail(err)
	}
	if _, err := tx.ExecContext(d.opCtx(), "CALL DOLT_MERGE('--no-commit', ?)", remoteHead); err != nil {
		return fail(err)
	}
	var version int
	if err := tx.QueryRowContext(d.opCtx(), "SELECT version FROM schema_version WHERE one = 1").Scan(&version); err != nil {
		return fail(fmt.Errorf("read the merged schema version: %w", err))
	}
	if version > SchemaVersion {
		return fail(fmt.Errorf("the remote's schema is version %d, newer than this pm's %d: run pm upgrade", version,
			SchemaVersion))
	}
	if res.Resolved, res.Overrides, err = resolveConflicts(tx); err != nil {
		return fail(err)
	}
	if _, err := tx.ExecContext(d.opCtx(), `UPDATE items SET holder_session = NULL, holder_host = NULL,
		holder_claimed_at = NULL WHERE status = 'closed' AND holder_session IS NOT NULL`); err != nil {
		return fail(err)
	}
	if _, err := loadChecked(tx); err != nil {
		return fail(err)
	}
	var merging bool
	if err := tx.QueryRowContext(d.opCtx(), "SELECT is_merging FROM dolt_merge_status").Scan(&merging); err != nil {
		return fail(err)
	}
	var changed int
	if err := tx.QueryRowContext(d.opCtx(), "SELECT COUNT(*) FROM dolt_status").Scan(&changed); err != nil {
		return fail(err)
	}
	if merging || changed > 0 {
		if _, err := tx.ExecContext(d.opCtx(), "CALL DOLT_COMMIT('-A', '--allow-empty', '-m', ?)",
			fmt.Sprintf("pm: merge %d commits from %s", behind, remoteHead)); err != nil {
			return fail(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fail(err)
	}
	return res, nil
}

// resolveConflicts resolves the merge's conflicted items rows by mergeItem and marks them resolved. A conflict in any
// other table, a constraint violation, or an items row added on both sides (add/add, the same new id) fails hard.
func resolveConflicts(tx *sql.Tx) (int, []Override, error) {
	rows, err := tx.QueryContext(ctx, "SELECT `table`, num_conflicts FROM dolt_conflicts")
	if err != nil {
		return 0, nil, err
	}
	var other []string
	for rows.Next() {
		var table string
		var n int
		if err := rows.Scan(&table, &n); err != nil {
			rows.Close()
			return 0, nil, err
		}
		if table != "items" {
			other = append(other, fmt.Sprintf("%d in %s", n, table))
		}
	}
	rows.Close()
	if len(other) > 0 {
		return 0, nil, fmt.Errorf("conflicts %s; no merge rule settles them", strings.Join(other, ", "))
	}
	if v, err := violations(tx); err != nil || v != "" {
		if err == nil {
			err = fmt.Errorf("constraint violations: %s", v)
		}
		return 0, nil, err
	}
	kinds, err := tx.QueryContext(ctx, `SELECT COALESCE(our_id, their_id, base_id), our_diff_type, their_diff_type
		FROM dolt_conflicts_items ORDER BY dolt_conflict_id`)
	if err != nil {
		return 0, nil, err
	}
	for kinds.Next() {
		var id, ours, theirs string
		if err := kinds.Scan(&id, &ours, &theirs); err != nil {
			kinds.Close()
			return 0, nil, err
		}
		if ours != "modified" || theirs != "modified" {
			kinds.Close()
			if ours == "added" && theirs == "added" {
				return 0, nil, fmt.Errorf("both sides added item %s with different fields (the same new id on two "+
					"clones); no merge rule settles it", id)
			}
			return 0, nil, fmt.Errorf("item %s was %s here and %s on the remote; no merge rule settles it", id, ours,
				theirs)
		}
	}
	kinds.Close()
	side := func(prefix string) ([]Item, error) {
		cols := strings.Fields(strings.ReplaceAll(itemColumns, ",", " "))
		for i, c := range cols {
			cols[i] = prefix + c
		}
		r, err := tx.QueryContext(ctx, "SELECT "+strings.Join(cols, ", ")+
			" FROM dolt_conflicts_items ORDER BY dolt_conflict_id")
		if err != nil {
			return nil, err
		}
		defer r.Close()
		var out []Item
		for r.Next() {
			it, err := scanItem(r)
			if err != nil {
				return nil, err
			}
			out = append(out, it)
		}
		return out, r.Err()
	}
	base, err := side("base_")
	if err != nil {
		return 0, nil, err
	}
	ours, err := side("our_")
	if err != nil {
		return 0, nil, err
	}
	theirs, err := side("their_")
	if err != nil {
		return 0, nil, err
	}
	var overrides []Override
	for i := range ours {
		m, over, err := mergeItem(base[i], ours[i], theirs[i])
		if err != nil {
			return 0, nil, err
		}
		if over != nil {
			overrides = append(overrides, *over)
		}
		if err := upsertItem(tx, &m); err != nil {
			return 0, nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM dolt_conflicts_items"); err != nil {
		return 0, nil, err
	}
	return len(ours), overrides, nil
}

// violations names the tables with constraint violations after a merge, "" when none.
func violations(tx *sql.Tx) (string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT `table`, num_violations FROM dolt_constraint_violations")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var table string
		var n int
		if err := rows.Scan(&table, &n); err != nil {
			return "", err
		}
		out = append(out, fmt.Sprintf("%d in %s", n, table))
	}
	return strings.Join(out, ", "), rows.Err()
}

// pushNow pushes the store's branch to the remote, through d.pushFn when a test set one.
func (d *Dolt) pushNow() error {
	if d.pushFn != nil {
		return d.pushFn(d.push)
	}
	return d.push()
}

// push is DOLT_PUSH of the branch, bounded by PushTimeout; a non-fast-forward rejection is errRemoteMoved, a timeout
// errPushTimeout, and any other failure an error whose outcome is unknown too (the remote ref may have moved).
func (d *Dolt) push() error {
	c, cancel := context.WithTimeout(d.opCtx(), PushTimeout)
	defer cancel()
	_, err := d.conn.ExecContext(c, "CALL DOLT_PUSH(?, ?)", remote, branch)
	switch {
	case err == nil:
		return nil
	case strings.Contains(err.Error(), "non-fast-forward"):
		return errRemoteMoved
	case errors.Is(err, context.DeadlineExceeded) || c.Err() != nil:
		return fmt.Errorf("%w after %s: %v", errPushTimeout, PushTimeout, err)
	}
	return fmt.Errorf("work store: push to the remote: %w", err)
}

// reset moves the store back to commit c, dropping what was committed after it.
func (d *Dolt) reset(c string) error {
	if _, err := d.conn.ExecContext(ctx, "CALL DOLT_RESET('--hard', ?)", c); err != nil {
		return fmt.Errorf("work store: reset to %s: %w", c, err)
	}
	return nil
}

// createShared is Create on a store with a remote: the compare-and-swap of the work-store page's Ids section, so two
// clones never mint the same child id or sprint number. It pulls, notes HEAD as C0, mints and commits the item, and
// pushes; a push the remote rejects as non-fast-forward resets to C0 and starts again, up to casAttempts in all. A
// push that times out or fails otherwise fetches: the new commit in the remote's history means the create landed;
// else reset and retry; a failing fetch resets and fails, the outcome unknown. With the remote unreachable, the pull
// fails and a create refuses.
func (d *Dolt) createShared(n New) (Item, error) {
	fail := func(c0 string, err error) (Item, error) {
		if rerr := d.reset(c0); rerr != nil {
			err = errors.Join(err, rerr)
		}
		return Item{}, err
	}
	var last error
	for range casAttempts {
		if _, err := d.pull(); err != nil {
			return Item{}, fmt.Errorf("work store: create: a child id is minted only against the remote: %w", err)
		}
		c0, err := d.head()
		if err != nil {
			return Item{}, err
		}
		it, err := d.createLocal(n)
		if err != nil {
			return Item{}, err
		}
		err = d.pushNow()
		last = err
		switch {
		case err == nil:
			return it, nil
		case errors.Is(err, errRemoteMoved):
			if err := d.reset(c0); err != nil {
				return Item{}, err
			}
			continue
		default: // a timeout, or another failure: whether the push landed is unknown
			made, herr := d.head()
			if herr != nil {
				return fail(c0, herr)
			}
			if _, ferr := d.fetch(); ferr != nil {
				return fail(c0, fmt.Errorf("work store: create %s: %v, and the fetch to check it failed too (%v): the "+
					"outcome is unknown; the store is back at its pre-create commit, and the next sync brings the item "+
					"back if the push landed", it.ID, err, ferr))
			}
			// Landed when the remote's history holds the new commit: another clone may have pushed on top since.
			var landed int
			if qerr := d.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM dolt_log(?) WHERE commit_hash = ?",
				remoteHead, made).Scan(&landed); qerr != nil {
				return fail(c0, qerr)
			}
			if landed > 0 {
				return it, nil
			}
			if err := d.reset(c0); err != nil {
				return Item{}, err
			}
			continue
		}
	}
	return Item{}, fmt.Errorf("work store: create: no push landed in %d attempts, the last: %v; nothing was created",
		casAttempts, last)
}
