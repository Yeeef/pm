package install

import "testing"

// TestCodexModeUninstallUndoesInit: on a file of the user's that holds none of pm's values, pm uninstall's Remove of
// pm init's Apply gives the file back byte for byte (nil for no file), and a key the user set keeps its value.
func TestCodexModeUninstallUndoesInit(t *testing.T) {
	for _, text := range []*string{nil, sp("# mine\nmodel = \"o3\"\n"), sp("# mine\nmodel = \"o3\"\n\n\n"),
		sp("model = \"o3\"\napproval_policy = \"never\"\n\n[profiles.deep]\nmodel = \"o3-pro\"\n"),
		sp("sandbox_mode = \"read-only\"\n")} {
		applied, err := codexModeApply(text)
		if err != nil {
			t.Fatal(err)
		}
		if ok, err := codexModePresent(&applied); err != nil || !ok {
			t.Errorf("%q: not present after Apply: %v", applied, err)
		}
		removed, err := codexModeRemove(applied)
		if err != nil {
			t.Fatal(err)
		}
		if (removed == nil) != (text == nil) || (text != nil && *removed != *text) {
			t.Errorf("Remove(Apply(%q)) = %v", deref(text), deref(removed))
		}
	}
}

func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
