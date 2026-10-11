package work

// holding is the operation in the slot, "" when none is.
func (h *Host) holding() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.slot) == 0 {
		return ""
	}
	return h.holder
}

// head is the store's HEAD commit.
func (d *Dolt) head() (string, error) { return d.hashOf("HEAD") }
