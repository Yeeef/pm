package install

import "testing"

// TestCodexModeUninstallUndoesInit: pm uninstall's Remove of pm init's Apply gives the user's file back byte for byte
// (nil for no file): with or without a final newline, empty, CRLF or with a BOM, and with a key the user set, to their
// own value or to pm's, which stays.
func TestCodexModeUninstallUndoesInit(t *testing.T) {
	for _, text := range []*string{nil, sp(""), sp("\n"), sp("model = \"o3\""), sp("# mine\nmodel = \"o3\""),
		sp("# mine\nmodel = \"o3\"\n"), sp("# mine\nmodel = \"o3\"\n\n\n"),
		sp("model = \"o3\"\napproval_policy = \"never\"\n\n[profiles.deep]\nmodel = \"o3-pro\"\n"),
		sp("sandbox_mode = \"read-only\"\n"), sp("approval_policy = \"on-request\"\nmodel = \"o3\"\n"),
		sp("approval_policy = \"on-request\"\n[x]\na = 1\n"), sp("model = \"o3\"\nsandbox_mode = \"workspace-write\""),
		sp("# mine\r\nmodel = \"o3\"\r\n"), sp("model = \"o3\"\r\n\r\n[x]\r\na = 1\r\n"),
		sp("\ufeffmodel = \"o3\"\n")} {
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

// TestCodexModeBlock: pm upgrade adds a key missing from pm's block under pm's comment, and pm uninstall takes the block
// whole, the blank line pm put before or after it included; a key outside the block, or holding another value, stays.
func TestCodexModeBlock(t *testing.T) {
	block := codexModeComment + "approval_policy = \"on-request\"\napprovals_reviewer = \"auto_review\"\n" +
		"sandbox_mode = \"workspace-write\"\n"
	for _, c := range []struct{ text, applied, removed string }{
		{"model = \"o3\"\n\n" + codexModeComment + "approval_policy = \"on-request\"\nsandbox_mode = \"workspace-write\"\n",
			"model = \"o3\"\n\n" + codexModeComment + "approval_policy = \"on-request\"\nsandbox_mode = \"workspace-write\"\n" +
				"approvals_reviewer = \"auto_review\"\n", "model = \"o3\"\n"},
		{codexModeComment + "sandbox_mode = \"workspace-write\"\n\n[x]\na = 1\n",
			codexModeComment + "sandbox_mode = \"workspace-write\"\napproval_policy = \"on-request\"\n" +
				"approvals_reviewer = \"auto_review\"\n\n[x]\na = 1\n", "[x]\na = 1\n"},
		{"model = \"o3\"\n\n" + codexModeComment + "approval_policy = \"never\"\napprovals_reviewer = \"auto_review\"\n" +
			"sandbox_mode = \"workspace-write\"\n", "", "model = \"o3\"\n\napproval_policy = \"never\"\n"},
	} {
		from := c.text
		if c.applied != "" {
			applied, err := codexModeApply(&c.text)
			if err != nil || applied != c.applied {
				t.Errorf("Apply(%q) = %q, %v", c.text, applied, err)
			}
			from = c.applied
		}
		removed, err := codexModeRemove(from)
		if err != nil || removed == nil || *removed != c.removed {
			t.Errorf("Remove(%q) = %v, %v", from, deref(removed), err)
		}
	}
	if removed, err := codexModeRemove(block); err != nil || removed != nil {
		t.Errorf("Remove(pm's block alone) = %v, %v; want nil", deref(removed), err)
	}
}

func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
