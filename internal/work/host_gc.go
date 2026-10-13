package work

import (
	"fmt"
	"reflect"
	"sync"
	"unsafe"

	"github.com/dolthub/dolt/go/libraries/doltcore/doltdb/gcctx"
)

// A collection's visits to the other sessions can outlive it, a Dolt bug (gcctx/gc_safepoint_controller.go, at the
// Dolt go.mod pins and at Dolt's main on 2026-10-13). DOLT_GC visits each idle session's GC roots in a goroutine of
// its own: BeginOutstandingVisitCall stores a new channel in the session's state, and the goroutine, once the visit
// returns, counts down the collection's wait group, then closes the channel the state holds at that moment
// (EndOutstandingVisitCall: close(state.QuiesceCallbackDone.Load())). The collection returns as soon as the wait
// group is done, so the next one can store a channel of its own in that state before the first goroutine closes:
// both goroutines then close the second channel, and the process dies, "panic: close of closed channel" (pm CI,
// race (linux-amd64), PR #61). pm runs one collection at a time (gcMu); it waits, before it lets the next one begin,
// for every session's visit to end: blockOnVisits.
//
// The controller keeps its sessions unexported, so gcVisits reads them by reflection. A Dolt whose controller no
// longer has those fields fails the host's start (newGCVisits), so a Dolt upgrade re-checks the bug.
type gcVisits struct {
	mu       *sync.Mutex
	sessions *map[gcctx.GCRootsProvider]*gcctx.GCSafepointSessionState
}

func newGCVisits(c *gcctx.GCSafepointController) (gcVisits, error) {
	v := reflect.ValueOf(c).Elem()
	mu, sessions := v.FieldByName("mu"), v.FieldByName("sessions")
	if !mu.IsValid() || mu.Type() != reflect.TypeFor[sync.Mutex]() || !sessions.IsValid() ||
		sessions.Type() != reflect.TypeFor[map[gcctx.GCRootsProvider]*gcctx.GCSafepointSessionState]() {
		return gcVisits{}, fmt.Errorf("work store: Dolt's GC safepoint controller is no longer a mutex and a map " +
			"of sessions; check whether a collection still returns before its visits end (internal/work/host_gc.go)")
	}
	return gcVisits{mu: (*sync.Mutex)(unsafe.Pointer(mu.UnsafeAddr())),
		sessions: (*map[gcctx.GCRootsProvider]*gcctx.GCSafepointSessionState)(unsafe.Pointer(sessions.UnsafeAddr()))}, nil
}

// blockOnVisits returns once every session's last visit has ended. Run after a collection returned, under gcMu: no
// collection begins a visit meanwhile, and each visit the last one began has returned (its wait group is done), so
// what is left of each is its channel's close.
func (g gcVisits) blockOnVisits() {
	for _, s := range g.states() {
		s.BlockForOutstandingVisitCall()
	}
}

// states are the states of the sessions the controller knows.
func (g gcVisits) states() []*gcctx.GCSafepointSessionState {
	g.mu.Lock()
	defer g.mu.Unlock()
	states := make([]*gcctx.GCSafepointSessionState, 0, len(*g.sessions))
	for _, s := range *g.sessions {
		states = append(states, s)
	}
	return states
}
