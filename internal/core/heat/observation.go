package heat

import (
	"strings"
	"time"
)

// PaneObservation is a domain observation of a tmux pane sample. A first
// observation establishes a baseline; only later fingerprint changes count as
// user activity.
type PaneObservation struct {
	PaneID      string
	Fingerprint string
	Sampled     bool
}

// ApplyPaneObservations folds pane samples into State and reports whether the
// samples prove new activity. Unknown panes in an otherwise unbaselined state do
// not count as activity because that is also what tmux restore/reboot looks like.
func ApplyPaneObservations(state *State, observations []PaneObservation) bool {
	if state == nil {
		return false
	}
	if state.Panes == nil {
		state.Panes = map[string]PaneState{}
	}
	if len(observations) == 0 {
		return false
	}

	nextPanes := make(map[string]PaneState, len(observations))
	active := false
	baselineOnly := len(state.Panes) == 0
	for _, observation := range observations {
		if strings.TrimSpace(observation.PaneID) == "" {
			continue
		}
		previous, hadPrevious := state.Panes[observation.PaneID]
		if !observation.Sampled {
			if hadPrevious {
				nextPanes[observation.PaneID] = previous
			}
			continue
		}
		if !baselineOnly && (!hadPrevious || previous.Fingerprint != observation.Fingerprint) {
			active = true
		}
		nextPanes[observation.PaneID] = PaneState{Fingerprint: observation.Fingerprint}
	}
	state.Panes = nextPanes
	return active
}

func RecentActivity(state State, now time.Time, window time.Duration) bool {
	_, ok := RecentActivityAge(state, now, window)
	return ok
}
