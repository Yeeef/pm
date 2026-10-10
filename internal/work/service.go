package work

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

// What a command asks the pm service for beside the Store interface: the remote the store syncs through, and the
// operations that run in the service (the pm-go page, "What runs in the service"), each a stored procedure on the
// command's one connection.

// Remote is the git remote URL the store syncs through, and whether it has one.
func (d *Dolt) Remote() (string, bool, error) { return d.remoteURL() }

// procError is a stored procedure's error as the operation gave it: the server sends it as MySQL error 1105, its
// text the operation's.
func (d *Dolt) procError(err error) error {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return errors.New(me.Message)
	}
	return d.broken(err)
}

// lines runs a procedure that answers one line per row.
func (d *Dolt) lines(call string, args ...any) ([]string, error) {
	rows, err := d.conn.QueryContext(ctx, call, args...)
	if err != nil {
		return nil, d.procError(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return nil, d.procError(err)
		}
		out = append(out, l)
	}
	return out, d.procError(rows.Err())
}

// CallSync asks the service to sync the store with the remote now (CALL pm_sync()): what it did, one line each, then
// a warning per claim a merge overrode.
func (d *Dolt) CallSync() ([]string, error) { return d.lines("CALL pm_sync()") }

// CallSetup asks the service to attach the clone's store to the remote (CALL pm_setup()): clone it, or create and
// push it, or point it at the remote; what it did, one line each.
func (d *Dolt) CallSetup() ([]string, error) { return d.lines("CALL pm_setup()") }

// callMoveSprint asks the service to move a sprint through the compare-and-swap (CALL pm_move_sprint(?)).
func (d *Dolt) callMoveSprint(id, to, reason string) (Item, error) {
	spec, err := json.Marshal(sprintMoveSpec{ID: id, To: to, Reason: reason})
	if err != nil {
		return Item{}, err
	}
	return d.callItem("pm_move_sprint", string(spec))
}

// callCreate asks the service to create n through the child-id compare-and-swap (CALL pm_create(?)).
func (d *Dolt) callCreate(n New) (Item, error) {
	spec, err := json.Marshal(n)
	if err != nil {
		return Item{}, err
	}
	return d.callItem("pm_create", string(spec))
}

// callItem runs a procedure that answers one item as JSON.
func (d *Dolt) callItem(proc, spec string) (Item, error) {
	out, err := d.lines("CALL "+proc+"(?)", spec)
	if err != nil {
		return Item{}, err
	}
	if len(out) != 1 {
		return Item{}, fmt.Errorf("work store: %s answered %d rows, not 1", proc, len(out))
	}
	var it Item
	if err := json.Unmarshal([]byte(out[0]), &it); err != nil {
		return Item{}, fmt.Errorf("work store: %s answered %q: %w", proc, out[0], err)
	}
	return it, nil
}
