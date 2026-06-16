package attention

import (
	"encoding/json"
	"strings"
	"time"
)

type Action string

const (
	ActionNoop       Action = "noop"
	ActionRunning    Action = "running"
	ActionAttention  Action = "attention"
	ActionSessionEnd Action = "session-end"
)

type Event struct {
	Action     Action
	Agent      string
	PaneID     string
	OccurredAt time.Time
}

type PaneState struct {
	Agent     string    `json:"agent,omitempty"`
	Active    bool      `json:"active,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

type State struct {
	UpdatedAt           time.Time            `json:"updatedAt,omitempty"`
	LastVisitedAt       time.Time            `json:"lastVisitedAt,omitempty"`
	Attention           bool                 `json:"attention,omitempty"`
	CurrentAcknowledged bool                 `json:"currentAcknowledged,omitempty"`
	Panes               map[string]PaneState `json:"panes,omitempty"`
}

func ApplyEvent(state State, event Event, visited bool) State {
	now := event.OccurredAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if state.Panes == nil {
		state.Panes = map[string]PaneState{}
	}
	if visited {
		state.LastVisitedAt = now
		state.Attention = false
		state.CurrentAcknowledged = true
	}
	if strings.TrimSpace(event.PaneID) == "" {
		state.UpdatedAt = now
		return state
	}

	pane := state.Panes[event.PaneID]
	pane.Agent = normalizeAgent(event.Agent)
	pane.UpdatedAt = now

	switch event.Action {
	case ActionRunning:
		pane.Active = true
		state.Panes[event.PaneID] = pane
	case ActionAttention:
		pane.Active = false
		state.Panes[event.PaneID] = pane
		if !visited {
			state.Attention = true
			state.CurrentAcknowledged = false
		}
	case ActionSessionEnd:
		delete(state.Panes, event.PaneID)
	case ActionNoop:
		state.Panes[event.PaneID] = pane
	default:
		state.Panes[event.PaneID] = pane
	}

	state.UpdatedAt = now
	return state
}

func AcknowledgeVisit(state State, now time.Time) State {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	state.LastVisitedAt = now
	state.Attention = false
	state.CurrentAcknowledged = true
	state.UpdatedAt = now
	if state.Panes == nil {
		state.Panes = map[string]PaneState{}
	}
	return state
}

func DecodeStateMap(raw map[string][]byte) map[string]State {
	decoded := make(map[string]State, len(raw))
	for name, data := range raw {
		if len(data) == 0 {
			continue
		}
		var state State
		if err := json.Unmarshal(data, &state); err != nil {
			continue
		}
		decoded[name] = state
	}
	return decoded
}

func EncodeStateMap(states map[string]State) map[string][]byte {
	encoded := make(map[string][]byte, len(states))
	for name, state := range states {
		data, err := json.Marshal(state)
		if err != nil {
			continue
		}
		encoded[name] = data
	}
	return encoded
}

func normalizeAgent(agent string) string {
	agent = strings.ToLower(strings.TrimSpace(agent))
	if agent == "" {
		return "agent"
	}
	return agent
}
