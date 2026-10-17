package cli

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Yeeef/pm/internal/cli/fsutil"
	"github.com/Yeeef/pm/internal/records"
)

// showPosix is Python's Path(p).as_posix() for a relative path: empty and "." parts dropped.
func showPosix(p string) string {
	var parts []string
	for _, s := range strings.Split(p, "/") {
		if s != "" && s != "." {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return "."
	}
	return strings.Join(parts, "/")
}

// linkTarget is the record a target names: a record path (records/ and .md optional), a sprint or project id, a
// sprint name (<project>-<n>), a project name or a design slug; the old path of a sprint pm sprint move moved names
// its record now. A target that fits several records is refused, never guessed.
func linkTarget(e *env, recs []*records.Record, target string) (*records.Record, error) {
	var found []*records.Record
	store := e.records
	if strings.Contains(target, "/") || strings.HasSuffix(target, ".md") {
		given := target
		if filepath.IsAbs(given) {
			rel, err := filepath.Rel(fsutil.Resolved(store), fsutil.Resolved(given))
			if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
				return nil, refuse("%s is not in the records store %s", target, store)
			}
			given = filepath.ToSlash(rel)
		}
		rel := strings.TrimSuffix(strings.TrimPrefix(showPosix(given), "records/"), ".md")
		for _, r := range recs {
			if r.Rel == rel {
				found = append(found, r)
			}
		}
		if len(found) == 0 && strings.HasPrefix(rel, "sprints/") { // a moved sprint's old path: the work store knows
			ws, err := e.work()
			if err != nil {
				return nil, err
			}
			items, err := ws.Items()
			if err != nil {
				return nil, err
			}
			if r := records.MovedSprint(recs, records.NewItems(items), rel); r != nil {
				found = append(found, r)
			}
		}
	} else {
		for _, r := range recs {
			bead, typed := r.ID("bead")
			if ((r.Type() == "sprint" || r.Type() == "project") && typed && bead == target) ||
				((r.Type() == "project" || r.Type() == "sprint" || r.Type() == "design") && r.Name() == target) {
				found = append(found, r)
			}
		}
	}
	if len(found) == 0 {
		return nil, refuse("no record matches %s; give a sprint id or name, a project name, a design slug or a record path "+
			"such as records/sprints/<name>.md", strconv.Quote(target))
	}
	if len(found) > 1 {
		names := make([]string, len(found))
		for i, r := range found {
			names[i] = "records/" + r.Rel + ".md"
		}
		return nil, refuse("%s names %s; give the record path instead", strconv.Quote(target), strings.Join(names, " and "))
	}
	return found[0], nil
}
