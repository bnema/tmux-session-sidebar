package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/bnema/tmux-session-sidebar/core/attention"
	"github.com/bnema/tmux-session-sidebar/ports"
)

type AgentAttentionEvent struct {
	Action      attention.Action
	Agent       string
	SessionName string
	SessionID   string
	PaneID      string
	OccurredAt  time.Time
}

type persistedClientSession struct {
	CurrentSessionID string `json:"currentSessionId,omitempty"`
}

func (s *Service) RecordAgentAttentionEvent(ctx context.Context, serverID string, event AgentAttentionEvent) (bool, error) {
	if s.store == nil {
		return false, ErrMissingStateStore
	}
	if strings.TrimSpace(event.SessionID) == "" {
		return false, fmt.Errorf("record agent attention event: missing session id")
	}
	if strings.TrimSpace(event.PaneID) == "" {
		return false, fmt.Errorf("record agent attention event: missing pane id")
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}

	state, err := s.store.Load(ctx, serverID)
	if err != nil {
		return false, err
	}
	decoded := attention.DecodeStateMap(state.AgentAttention)
	if liveSessionIDs, ok := s.liveSessionIDs(ctx); ok {
		pruneAgentAttentionSessions(decoded, liveSessionIDs)
	}
	sessionID := strings.TrimSpace(event.SessionID)
	previousSessionState := decoded[sessionID]
	previousAttention := previousSessionState.Attention
	visited := shouldSuppressEventAttentionWhileSessionIsCurrent(previousSessionState) && s.sessionCurrentlyAttached(ctx, sessionID)
	sessionState := attention.ApplyEvent(previousSessionState, attention.Event{
		Action:     event.Action,
		Agent:      event.Agent,
		PaneID:     event.PaneID,
		OccurredAt: event.OccurredAt,
	}, visited)
	if emptyAgentAttentionState(sessionState) {
		delete(decoded, sessionID)
	} else {
		decoded[sessionID] = sessionState
	}
	state.AgentAttention = attention.EncodeStateMap(decoded)
	if err := s.store.Save(ctx, serverID, state); err != nil {
		return false, err
	}
	return previousAttention != sessionState.Attention, nil
}

func (s *Service) CaptureVisitedAgentAttention(ctx context.Context, serverID string) error {
	if s.store == nil {
		return ErrMissingStateStore
	}
	if s.tmuxQuery == nil {
		return ErrMissingTmuxQuery
	}

	state, err := s.store.Load(ctx, serverID)
	if err != nil {
		return err
	}

	live, err := s.tmuxQuery.ListSessions(ctx)
	if err != nil {
		return err
	}
	liveSessionIDs := make(map[string]bool, len(live))
	for _, session := range live {
		liveSessionIDs[session.ID] = true
	}

	decoded := attention.DecodeStateMap(state.AgentAttention)
	changed := pruneAgentAttentionSessions(decoded, liveSessionIDs)
	clients, err := s.tmuxQuery.ListClients(ctx)
	if err != nil {
		if !changed && len(state.Clients) == 0 {
			return nil
		}
		state.AgentAttention = attention.EncodeStateMap(decoded)
		state.Clients = nil
		return s.store.Save(ctx, serverID, state)
	}
	previousClientSessions := decodePersistedClientSessions(state.Clients)
	currentClientSessions := attachedClientSessions(clients)
	now := time.Now().UTC()
	for clientID, sessionID := range currentClientSessions {
		previousSessionID := previousClientSessions[clientID]
		if previousSessionID == sessionID {
			continue
		}
		sessionState, ok := decoded[sessionID]
		if previousSessionID == "" && (!ok || !sessionState.Attention) {
			continue
		}
		decoded[sessionID] = attention.AcknowledgeVisit(sessionState, now)
		changed = true
	}
	if len(previousClientSessions) == 0 {
		previousClientSessions = nil
	}
	if len(currentClientSessions) == 0 {
		currentClientSessions = nil
	}
	clientsChanged := !reflect.DeepEqual(previousClientSessions, currentClientSessions)
	if !changed && !clientsChanged {
		return nil
	}
	state.AgentAttention = attention.EncodeStateMap(decoded)
	state.Clients = encodePersistedClientSessions(currentClientSessions)
	return s.store.Save(ctx, serverID, state)
}

func (s *Service) liveSessionIDs(ctx context.Context) (map[string]bool, bool) {
	if s == nil || s.tmuxQuery == nil {
		return nil, false
	}
	live, err := s.tmuxQuery.ListSessions(ctx)
	if err != nil {
		return nil, false
	}
	ids := make(map[string]bool, len(live))
	for _, session := range live {
		ids[session.ID] = true
	}
	return ids, true
}

func pruneAgentAttentionSessions(states map[string]attention.State, liveSessionIDs map[string]bool) bool {
	if len(states) == 0 {
		return false
	}
	changed := false
	for sessionID := range states {
		if liveSessionIDs[sessionID] {
			continue
		}
		delete(states, sessionID)
		changed = true
	}
	return changed
}

func emptyAgentAttentionState(state attention.State) bool {
	return state.LastVisitedAt.IsZero() && !state.Attention && !state.CurrentAcknowledged && len(state.Panes) == 0
}

func shouldSuppressEventAttentionWhileSessionIsCurrent(state attention.State) bool {
	return !state.Attention && state.CurrentAcknowledged
}

func (s *Service) sessionCurrentlyAttached(ctx context.Context, sessionID string) bool {
	if s == nil || s.tmuxQuery == nil || strings.TrimSpace(sessionID) == "" {
		return false
	}
	clients, err := s.tmuxQuery.ListClients(ctx)
	if err != nil {
		return true
	}
	for _, client := range clients {
		if client.Attached && client.CurrentSessionID == sessionID {
			return true
		}
	}
	return false
}

func attachedClientSessions(clients []ports.TmuxClientSnapshot) map[string]string {
	attached := make(map[string]string, len(clients))
	for _, client := range clients {
		if !client.Attached || strings.TrimSpace(client.ID) == "" || strings.TrimSpace(client.CurrentSessionID) == "" {
			continue
		}
		attached[client.ID] = client.CurrentSessionID
	}
	return attached
}

func decodePersistedClientSessions(raw map[string][]byte) map[string]string {
	decoded := make(map[string]string, len(raw))
	for clientID, data := range raw {
		if len(data) == 0 {
			continue
		}
		var state persistedClientSession
		if err := json.Unmarshal(data, &state); err != nil {
			continue
		}
		if strings.TrimSpace(state.CurrentSessionID) == "" {
			continue
		}
		decoded[clientID] = state.CurrentSessionID
	}
	return decoded
}

func encodePersistedClientSessions(sessions map[string]string) map[string][]byte {
	if len(sessions) == 0 {
		return nil
	}
	encoded := make(map[string][]byte, len(sessions))
	for clientID, sessionID := range sessions {
		data, err := json.Marshal(persistedClientSession{CurrentSessionID: sessionID})
		if err != nil {
			continue
		}
		encoded[clientID] = data
	}
	return encoded
}
