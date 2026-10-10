package work

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	return d.pushNow(branch)
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

// Clone makes the work database a clone of the store on the git remote, under RemoteRef, on a connection with none
// selected (DialSetup), and selects it; an older schema it brings is migrated, since only the service clones (pm
// init's setup). It fails when the remote holds no store.
func (d *Dolt) Clone(gitURL string) error {
	u, err := DoltRemoteURL(gitURL)
	if err != nil {
		return err
	}
	if err := d.remoteCallOn(d.opCtx(), false, "CALL DOLT_CLONE('--remote', ?, '--ref', ?, ?, ?)", remote, RemoteRef,
		u, dbName); err != nil {
		return fmt.Errorf("work store: clone %s: %w", gitURL, err)
	}
	if err := d.UseStore(); err != nil {
		return err
	}
	return d.migrate()
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
		err = d.pushNow(branch)
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
	if err := d.remoteCall(d.opCtx(), "CALL DOLT_FETCH(?)", remote); err != nil {
		return false, fmt.Errorf("work store: fetch from the remote: %w", err)
	}
	var n int
	if err := d.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM dolt_remote_branches WHERE name = ?",
		"remotes/"+remoteHead).Scan(&n); err != nil {
		return false, fmt.Errorf("work store: %w", err)
	}
	return n > 0, nil
}

// count is the number of commits in a dolt_log range such as "main..origin/main".
func (d *Dolt) count(q querier, rng string) (int, error) {
	rows, err := q.QueryContext(ctx, "SELECT COUNT(*) FROM dolt_log(?)", rng)
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
	if err := d.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM dolt_remote_branches WHERE name = ?",
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
	if err := d.conn.QueryRowContext(ctx, "SELECT HASHOF('HEAD')").Scan(&h); err != nil {
		return "", fmt.Errorf("work store: %w", err)
	}
	return h, nil
}

// hashOf is the commit a ref names.
func (d *Dolt) hashOf(ref string) (string, error) {
	var h string
	if err := d.conn.QueryRowContext(ctx, "SELECT HASHOF(?)", ref).Scan(&h); err != nil {
		return "", fmt.Errorf("work store: %w", d.broken(err))
	}
	return h, nil
}

// pull fetches and, when the store is behind, merges the remote's branch. It never resets main, since other sessions'
// writes may land on it meanwhile: it checks the remote's head first (its schema version and every invariant, read on
// the revision database work/<hash>), so a fast-forward, which Dolt applies at once, needs no undo; a 3-way merge runs
// in one write transaction under the store's write lock (Dolt merges each cell, pm resolves each conflicted items
// row by the merge rules, keeps this side's write stamp, clears the holder of every closed item, checks every
// invariant and the blocked_by cycles, sets a fresh write stamp, and commits) that lands whole or rolls back. Only the
// fetch is bounded by the operation's context: the local merge runs to its end, since a merge cut partway could split
// Dolt's fast-forward (the head moved, the working set not), and the write lock keeps it from racing a local write.
func (d *Dolt) pull() (SyncResult, error) {
	has, err := d.fetch()
	if err != nil || !has {
		return SyncResult{}, err
	}
	theirs, err := d.hashOf(remoteHead)
	if err != nil {
		return SyncResult{}, err
	}
	if behind, err := d.count(d.conn, branch+".."+theirs); err != nil || behind == 0 {
		return SyncResult{}, err
	}
	if err := d.checkHead(theirs); err != nil {
		return SyncResult{}, fmt.Errorf("work store: pull: the remote's head %s: %w; the store stays as it was",
			theirs, err)
	}
	return d.merge(theirs)
}

// merge merges the checked remote head theirs into main, under the store's write lock.
func (d *Dolt) merge(theirs string) (res SyncResult, err error) {
	what := "pm: merge " + remoteHead
	unlock, err := d.lockWrites(what)
	if err != nil {
		return res, err
	}
	defer unlock()
	behind, err := d.count(d.conn, branch+".."+theirs)
	if err != nil || behind == 0 {
		return res, err
	}
	res.Pulled = behind
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return res, fmt.Errorf("work store: %w", d.broken(err))
	}
	defer d.conn.ExecContext(ctx, "SET @@dolt_allow_commit_conflicts = 0")
	fail := func(err error) (SyncResult, error) {
		_ = tx.Rollback()
		if retryable(err) {
			err = errors.New("it conflicted with a write that did not take the store's write lock (a SQL client past pm?)")
		}
		// A fast-forward moves main at once, to the head checked before the merge; the rest rolled back.
		switch h, herr := d.hashOf(branch); {
		case herr != nil:
			return SyncResult{}, fmt.Errorf("work store: pull: %w; whether main fast-forwarded to the remote's checked "+
				"head %s is unknown (%v): check with pm show", d.broken(err), theirs, herr)
		case h == theirs:
			return SyncResult{}, fmt.Errorf("work store: pull: %w; main had fast-forwarded to the remote's checked "+
				"head %s, which stays", d.broken(err), theirs)
		}
		return SyncResult{}, fmt.Errorf("work store: pull: %w; the store stays as it was", d.broken(err))
	}
	if err := clean(tx, what); err != nil {
		return fail(err)
	}
	if _, err := tx.ExecContext(ctx, "SET @@dolt_allow_commit_conflicts = 1"); err != nil {
		return fail(err)
	}
	if _, err := tx.ExecContext(ctx, "CALL DOLT_MERGE('--no-commit', ?)", theirs); err != nil {
		return fail(err)
	}
	var version int
	if err := tx.QueryRowContext(ctx, "SELECT version FROM schema_version WHERE one = 1").Scan(&version); err != nil {
		return fail(fmt.Errorf("read the merged schema version: %w", err))
	}
	if version > SchemaVersion {
		return fail(fmt.Errorf("the remote's schema is version %d, newer than this pm's %d: run pm upgrade", version,
			SchemaVersion))
	}
	if res.Resolved, res.Overrides, err = resolveConflicts(tx); err != nil {
		return fail(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE items SET holder_session = NULL, holder_host = NULL,
		holder_claimed_at = NULL WHERE status = 'closed' AND holder_session IS NOT NULL`); err != nil {
		return fail(err)
	}
	if _, err := loadChecked(tx); err != nil {
		return fail(err)
	}
	var merging bool
	if err := tx.QueryRowContext(ctx, "SELECT is_merging FROM dolt_merge_status").Scan(&merging); err != nil {
		return fail(err)
	}
	var changed int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM dolt_status").Scan(&changed); err != nil {
		return fail(err)
	}
	if merging || changed > 0 { // a 3-way merge; a fast-forward moved main already and commits nothing
		if err := setStamp(tx); err != nil {
			return fail(err)
		}
		if _, err := tx.ExecContext(ctx, "CALL DOLT_COMMIT('-A', '--allow-empty', '-m', ?)",
			fmt.Sprintf("pm: merge %d commits from %s", behind, remoteHead)); err != nil {
			return fail(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fail(err)
	}
	return res, nil
}

// checkHead checks the commit h, read on the revision database work/<h>: its schema version is not newer than this
// pm's, and its items hold every invariant.
func (d *Dolt) checkHead(h string) (err error) {
	if _, err := d.conn.ExecContext(ctx, "USE `"+dbName+"/"+h+"`"); err != nil {
		return d.broken(err)
	}
	defer func() {
		if _, uerr := d.conn.ExecContext(ctx, "USE `"+dbName+"`"); uerr != nil && err == nil {
			err = d.broken(uerr)
		}
	}()
	var version int
	if err := d.conn.QueryRowContext(ctx, "SELECT version FROM schema_version WHERE one = 1").Scan(&version); err != nil {
		return fmt.Errorf("read its schema version: %w", d.broken(err))
	}
	if version > SchemaVersion {
		return fmt.Errorf("its schema is version %d, newer than this pm's %d: run pm upgrade", version, SchemaVersion)
	}
	_, err = loadChecked(d.conn)
	return err
}

// resolveConflicts resolves the merge's conflicted items rows by mergeItem and marks them resolved, and keeps this
// side's write stamp. A conflict in any other table, a constraint violation, or an items row added on both sides
// (add/add, the same new id) fails hard.
func resolveConflicts(tx *sql.Tx) (int, []Override, error) {
	rows, err := tx.QueryContext(ctx, "SELECT `table`, num_conflicts FROM dolt_conflicts")
	if err != nil {
		return 0, nil, err
	}
	var other []string
	stampConflict := false
	for rows.Next() {
		var table string
		var n int
		if err := rows.Scan(&table, &n); err != nil {
			rows.Close()
			return 0, nil, err
		}
		switch table {
		case "items":
		case "write_stamp":
			stampConflict = true
		default:
			other = append(other, fmt.Sprintf("%d in %s", n, table))
		}
	}
	rows.Close()
	if len(other) > 0 {
		return 0, nil, fmt.Errorf("conflicts %s; no merge rule settles them", strings.Join(other, ", "))
	}
	if stampConflict { // deleting a conflict row keeps the working set's value: this side's
		if _, err := tx.ExecContext(ctx, "DELETE FROM dolt_conflicts_write_stamp"); err != nil {
			return 0, nil, err
		}
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

// pushNow pushes the local branch from to the remote's main, through d.pushFn when a test set one.
func (d *Dolt) pushNow(from string) error {
	push := func() error { return d.push(from) }
	if d.pushFn != nil {
		return d.pushFn(push)
	}
	return push()
}

// push is DOLT_PUSH of the local branch from to the remote's main, bounded by PushTimeout; a non-fast-forward
// rejection is errRemoteMoved, a timeout errPushTimeout, and any other failure an error whose outcome is unknown too
// (the remote ref may have moved).
func (d *Dolt) push(from string) error {
	c, cancel := context.WithTimeout(d.opCtx(), PushTimeout)
	defer cancel()
	spec := branch
	if from != branch {
		spec = from + ":" + branch
	}
	err := d.remoteCall(c, "CALL DOLT_PUSH(?, ?)", remote, spec)
	switch {
	case err == nil:
		return nil
	case strings.Contains(err.Error(), "non-fast-forward"):
		return errRemoteMoved
	case d.opCtx().Err() != nil: // the operation's own bound ended first
		return fmt.Errorf("%w at the operation's bound: %v", errPushTimeout, err)
	case errors.Is(err, context.DeadlineExceeded) || c.Err() != nil:
		return fmt.Errorf("%w after %s: %v", errPushTimeout, PushTimeout, err)
	}
	return fmt.Errorf("work store: push to the remote: %w", d.broken(err))
}

// casBranch is the scratch branch the child-id compare-and-swap mints on, so it never resets main.
const casBranch = "pm-cas"

// dropCAS deletes the scratch branch if it is there.
func (d *Dolt) dropCAS() error {
	var n int
	if err := d.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM dolt_branches WHERE name = ?", casBranch).Scan(&n); err != nil {
		return fmt.Errorf("work store: %w", d.broken(err))
	}
	if n == 0 {
		return nil
	}
	if _, err := d.conn.ExecContext(ctx, "CALL DOLT_BRANCH('-D', ?)", casBranch); err != nil {
		return fmt.Errorf("work store: delete %s: %w", casBranch, d.broken(err))
	}
	return nil
}

// casWrite is one write the pm service runs through the compare-and-swap (shared): what it mints, and what to do when
// its push's outcome is unknown or its merge here fails after the push landed.
type casWrite struct {
	verb, mints string               // "create", "a child id"
	write       func() (Item, error) // the write, on the scratch branch
	unknown     func(it Item) string // the next step when the push may still land
	landed      func(it Item) string // the next step when the push landed but the merge into main failed
}

// createShared is Create on a store with a remote: the compare-and-swap (shared) whose write is the create.
func (d *Dolt) createShared(n New) (Item, error) {
	return d.shared(casWrite{
		verb:  "create",
		mints: "a child id",
		write: func() (Item, error) { return d.createLocal(n) },
		unknown: func(it Item) string {
			return fmt.Sprintf("the push may still land as %s. Nothing was merged here: run pm sync, then check with "+
				"pm show %s before you create it again", it.ID, it.ID)
		},
		landed: func(it Item) string {
			return fmt.Sprintf("the item %s is on the remote; do not create it again: the next sync (pm sync) brings it",
				it.ID)
		},
	})
}

// shared runs a write that mints against the remote, by the pm service one at a time: the compare-and-swap of the
// work-store page's Ids section, so two clones never mint the same child id or sprint number, and main is never
// reset. It pulls, notes main's HEAD as C0, makes the branch pm-cas at C0, runs the write there, committed as C1,
// and pushes pm-cas to the remote's main. Only a push the remote rejects as non-fast-forward, which lands nothing,
// deletes pm-cas and starts again, up to casAttempts in all. A push that times out or fails otherwise fetches: C1 in
// the remote's history means the write landed; anything else deletes pm-cas and fails hard, the outcome unknown,
// since a dropped push can land later and a second mint would make the item twice. Once the push landed, pm-cas
// merges into main in one write transaction (a fast-forward when no session wrote since C0) and is deleted. With the
// remote unreachable, the pull fails and the write refuses.
func (d *Dolt) shared(w casWrite) (Item, error) {
	var last error
	for range casAttempts {
		if _, err := d.pull(); err != nil {
			return Item{}, fmt.Errorf("work store: %s: %s is minted only against the remote: %w", w.verb, w.mints, err)
		}
		c0, err := d.hashOf(branch)
		if err != nil {
			return Item{}, err
		}
		if err := d.dropCAS(); err != nil {
			return Item{}, err
		}
		if _, err := d.conn.ExecContext(ctx, "CALL DOLT_BRANCH(?, ?)", casBranch, c0); err != nil {
			return Item{}, fmt.Errorf("work store: make %s: %w", casBranch, d.broken(err))
		}
		it, c1, err := d.onBranch(casBranch, w.write)
		if err != nil {
			return Item{}, errors.Join(err, d.dropCAS())
		}
		err = d.pushNow(casBranch)
		last = err
		switch {
		case err == nil:
		case errors.Is(err, errRemoteMoved):
			if err := d.dropCAS(); err != nil {
				return Item{}, err
			}
			continue
		default: // a timeout, or another failure: whether the push landed is unknown, and may stay so
			// The push the client dropped may still land on the remote (the server finishes the statement until
			// Dolt kills its git, and git can update the ref after that): C1 seen there means it landed, but C1 not
			// seen yet proves nothing, so the write never mints again, which could make the item twice.
			unknown := func(why string) (Item, error) {
				return Item{}, errors.Join(fmt.Errorf("work store: %s %s: %v; %s: the outcome is unknown, and %s",
					w.verb, it.ID, err, why, w.unknown(it)), d.dropCAS())
			}
			if _, ferr := d.fetch(); ferr != nil {
				return unknown(fmt.Sprintf("the fetch to check it failed too (%v)", ferr))
			}
			// Landed when the remote's history holds C1: another clone may have pushed on top since.
			var landed int
			if qerr := d.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM dolt_log(?) WHERE commit_hash = ?",
				remoteHead, c1).Scan(&landed); qerr != nil {
				return unknown(fmt.Sprintf("reading the remote's history failed (%v)", d.broken(qerr)))
			}
			if landed == 0 {
				return unknown("the remote's history does not hold it yet")
			}
		}
		if err := d.mergeCAS(); err != nil {
			return Item{}, fmt.Errorf("work store: %s %s: merging it into this store failed (%w); %s", w.verb, it.ID,
				err, w.landed(it))
		}
		return it, d.dropCAS()
	}
	return Item{}, fmt.Errorf("work store: %s: no push landed in %d attempts, the last: %v; nothing was written",
		w.verb, casAttempts, last)
}

// onBranch runs fn with the connection on the local branch b (the revision database work/<b>), and returns what fn
// returned and b's head after it.
func (d *Dolt) onBranch(b string, fn func() (Item, error)) (it Item, head string, err error) {
	if _, err := d.conn.ExecContext(ctx, "USE `"+dbName+"/"+b+"`"); err != nil {
		return Item{}, "", d.broken(err)
	}
	defer func() {
		if _, uerr := d.conn.ExecContext(ctx, "USE `"+dbName+"`"); uerr != nil && err == nil {
			err = d.broken(uerr)
		}
	}()
	if it, err = fn(); err != nil {
		return Item{}, "", err
	}
	head, err = d.hashOf("HEAD")
	return it, head, err
}

// mergeCAS merges pm-cas into main in one write transaction under the store's write lock, keeping this side's write
// stamp on a conflict and setting a fresh one.
func (d *Dolt) mergeCAS() error {
	return d.inTx(fmt.Sprintf("pm: merge %s", casBranch), true, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "SET @@dolt_allow_commit_conflicts = 1"); err != nil {
			return err
		}
		defer tx.ExecContext(ctx, "SET @@dolt_allow_commit_conflicts = 0")
		if _, err := tx.ExecContext(ctx, "CALL DOLT_MERGE('--no-commit', ?)", casBranch); err != nil {
			return err
		}
		if _, _, err := resolveConflicts(tx); err != nil {
			return err
		}
		_, err := loadChecked(tx)
		return err
	})
}
