package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/internal/core/attention"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
	"github.com/bnema/tmux-session-sidebar/internal/ports/mocks"
	"github.com/stretchr/testify/mock"
)

type testPersistedClientSession struct {
	CurrentSessionID string `json:"currentSessionId,omitempty"`
}

func TestCaptureVisitedAgentAttentionPersistsAttachedClientBaselineWithoutAttention(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockQueryPort(t)

	store.EXPECT().Load(ctx, serverID).Return(ports.PersistedState{}, nil)
	query.EXPECT().ListSessions(ctx).Return([]ports.SessionSnapshot{{ID: "$1", Name: "alpha"}}, nil)
	query.EXPECT().ListClients(ctx).Return([]ports.ClientSnapshot{{ID: "%client", CurrentSessionID: "$1", Attached: true}}, nil)
	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		return decodePersistedClientSessionsForTest(t, state.Clients)["%client"] == "$1" && len(state.AgentAttention) == 0
	})).Return(nil)

	if err := NewService(nil, query, nil, store).CaptureVisitedAgentAttention(ctx, serverID); err != nil {
		t.Fatalf("CaptureVisitedAgentAttention error: %v", err)
	}
}

func TestCaptureVisitedAgentAttentionDoesNotClearAlreadyCurrentSession(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockQueryPort(t)
	initial := ports.PersistedState{
		AgentAttention: attention.EncodeStateMap(map[string]attention.State{
			"$1": {Attention: true, UpdatedAt: time.Now().UTC()},
		}),
		Clients: encodePersistedClientSessionsForTest(t, map[string]string{"%client": "$1"}),
	}

	store.EXPECT().Load(ctx, serverID).Return(initial, nil)
	query.EXPECT().ListSessions(ctx).Return([]ports.SessionSnapshot{{ID: "$1", Name: "alpha"}}, nil)
	query.EXPECT().ListClients(ctx).Return([]ports.ClientSnapshot{{ID: "%client", CurrentSessionID: "$1", Attached: true}}, nil)

	if err := NewService(nil, query, nil, store).CaptureVisitedAgentAttention(ctx, serverID); err != nil {
		t.Fatalf("CaptureVisitedAgentAttention error: %v", err)
	}
}

func TestCaptureVisitedAgentAttentionClearsAfterLaterRevisit(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockQueryPort(t)
	initial := ports.PersistedState{AgentAttention: attention.EncodeStateMap(map[string]attention.State{
		"$live": {Attention: true},
	}), Clients: encodePersistedClientSessionsForTest(t, map[string]string{"%client": "$other"})}
	store.EXPECT().Load(ctx, serverID).Return(initial, nil)
	query.EXPECT().ListSessions(ctx).Return([]ports.SessionSnapshot{{ID: "$live", Name: "alpha"}, {ID: "$other", Name: "beta"}}, nil)
	query.EXPECT().ListClients(ctx).Return([]ports.ClientSnapshot{{ID: "%client", CurrentSessionID: "$live", Attached: true}}, nil)
	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		decoded := attention.DecodeStateMap(state.AgentAttention)
		sessionState, ok := decoded["$live"]
		return ok && !sessionState.Attention && !sessionState.LastVisitedAt.IsZero() && decodePersistedClientSessionsForTest(t, state.Clients)["%client"] == "$live"
	})).Return(nil)

	if err := NewService(nil, query, nil, store).CaptureVisitedAgentAttention(ctx, serverID); err != nil {
		t.Fatalf("CaptureVisitedAgentAttention error: %v", err)
	}
}

func TestRecordAgentAttentionEventKeysBySessionIDAndLatchesWhileViewedInitially(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockQueryPort(t)
	query.EXPECT().ListSessions(ctx).Return([]ports.SessionSnapshot{{ID: "$1", Name: "alpha"}}, nil)
	store.EXPECT().Load(ctx, serverID).Return(ports.PersistedState{}, nil)
	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		decoded := attention.DecodeStateMap(state.AgentAttention)
		sessionState, ok := decoded["$1"]
		return ok && sessionState.Attention && sessionState.Panes["%2"].Agent == "codex"
	})).Return(nil)

	event := AgentAttentionEvent{
		Action:      attention.ActionAttention,
		Agent:       "codex",
		SessionName: "alpha",
		SessionID:   "$1",
		PaneID:      "%2",
		OccurredAt:  time.Now().UTC(),
	}
	if changed, err := NewService(nil, query, nil, store).RecordAgentAttentionEvent(ctx, serverID, event); err != nil {
		t.Fatalf("RecordAgentAttentionEvent error: %v", err)
	} else if !changed {
		t.Fatal("RecordAgentAttentionEvent changed = false, want true")
	}
}

func TestRecordAgentAttentionEventDoesNotRelatchAfterRevisitWhileCurrent(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockQueryPort(t)
	visitedAt := time.Now().UTC().Add(-time.Minute)
	updatedAt := visitedAt.Add(30 * time.Second)
	initial := ports.PersistedState{AgentAttention: attention.EncodeStateMap(map[string]attention.State{
		"$1": {
			UpdatedAt:           updatedAt,
			LastVisitedAt:       visitedAt,
			Attention:           false,
			CurrentAcknowledged: true,
			Panes:               map[string]attention.PaneState{"%2": {Agent: "codex", UpdatedAt: visitedAt}},
		},
	})}
	query.EXPECT().ListSessions(ctx).Return([]ports.SessionSnapshot{{ID: "$1", Name: "alpha"}}, nil)
	query.EXPECT().ListClients(ctx).Return([]ports.ClientSnapshot{{ID: "%client", CurrentSessionID: "$1", Attached: true}}, nil)
	store.EXPECT().Load(ctx, serverID).Return(initial, nil)
	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		decoded := attention.DecodeStateMap(state.AgentAttention)
		sessionState, ok := decoded["$1"]
		return ok && !sessionState.Attention && sessionState.CurrentAcknowledged && !sessionState.LastVisitedAt.Before(updatedAt) && sessionState.Panes["%2"].Agent == "codex"
	})).Return(nil)

	event := AgentAttentionEvent{
		Action:      attention.ActionAttention,
		Agent:       "codex",
		SessionName: "alpha",
		SessionID:   "$1",
		PaneID:      "%2",
		OccurredAt:  time.Now().UTC(),
	}
	if changed, err := NewService(nil, query, nil, store).RecordAgentAttentionEvent(ctx, serverID, event); err != nil {
		t.Fatalf("RecordAgentAttentionEvent error: %v", err)
	} else if changed {
		t.Fatal("RecordAgentAttentionEvent changed = true, want false")
	}
}

func TestCaptureVisitedAgentAttentionRearmsCurrentAcknowledgedOnLaterRevisit(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockQueryPort(t)
	initial := ports.PersistedState{AgentAttention: attention.EncodeStateMap(map[string]attention.State{
		"$1": {CurrentAcknowledged: false},
	}), Clients: encodePersistedClientSessionsForTest(t, map[string]string{"%client": "$2"})}
	store.EXPECT().Load(ctx, serverID).Return(initial, nil)
	query.EXPECT().ListSessions(ctx).Return([]ports.SessionSnapshot{{ID: "$1", Name: "alpha"}, {ID: "$2", Name: "beta"}}, nil)
	query.EXPECT().ListClients(ctx).Return([]ports.ClientSnapshot{{ID: "%client", CurrentSessionID: "$1", Attached: true}}, nil)
	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		decoded := attention.DecodeStateMap(state.AgentAttention)
		sessionState := decoded["$1"]
		return sessionState.CurrentAcknowledged && !sessionState.LastVisitedAt.IsZero() && decodePersistedClientSessionsForTest(t, state.Clients)["%client"] == "$1"
	})).Return(nil)

	if err := NewService(nil, query, nil, store).CaptureVisitedAgentAttention(ctx, serverID); err != nil {
		t.Fatalf("CaptureVisitedAgentAttention error: %v", err)
	}
}

func TestCaptureVisitedAgentAttentionClearsClientBaselineOnListClientsError(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockQueryPort(t)
	initial := ports.PersistedState{Clients: encodePersistedClientSessionsForTest(t, map[string]string{"%client": "$1"})}
	store.EXPECT().Load(ctx, serverID).Return(initial, nil)
	query.EXPECT().ListSessions(ctx).Return([]ports.SessionSnapshot{{ID: "$1", Name: "alpha"}}, nil)
	query.EXPECT().ListClients(ctx).Return(nil, errors.New("tmux unavailable"))
	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		return state.Clients == nil
	})).Return(nil)

	if err := NewService(nil, query, nil, store).CaptureVisitedAgentAttention(ctx, serverID); err != nil {
		t.Fatalf("CaptureVisitedAgentAttention error: %v", err)
	}
}

func TestCaptureVisitedAgentAttentionPrunesMissingSessions(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockQueryPort(t)
	initial := ports.PersistedState{AgentAttention: attention.EncodeStateMap(map[string]attention.State{
		"$live": {Attention: true},
		"$gone": {Attention: true},
	})}
	store.EXPECT().Load(ctx, serverID).Return(initial, nil)
	query.EXPECT().ListSessions(ctx).Return([]ports.SessionSnapshot{{ID: "$live", Name: "alpha"}}, nil)
	query.EXPECT().ListClients(ctx).Return(nil, nil)
	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		decoded := attention.DecodeStateMap(state.AgentAttention)
		_, hasGone := decoded["$gone"]
		_, hasLive := decoded["$live"]
		return !hasGone && hasLive
	})).Return(nil)

	if err := NewService(nil, query, nil, store).CaptureVisitedAgentAttention(ctx, serverID); err != nil {
		t.Fatalf("CaptureVisitedAgentAttention error: %v", err)
	}
}

func encodePersistedClientSessionsForTest(t *testing.T, sessions map[string]string) map[string][]byte {
	t.Helper()
	encoded := make(map[string][]byte, len(sessions))
	for clientID, sessionID := range sessions {
		data, err := json.Marshal(testPersistedClientSession{CurrentSessionID: sessionID})
		if err != nil {
			t.Fatalf("marshal persisted client session: %v", err)
		}
		encoded[clientID] = data
	}
	return encoded
}

func decodePersistedClientSessionsForTest(t *testing.T, raw map[string][]byte) map[string]string {
	t.Helper()
	decoded := make(map[string]string, len(raw))
	for clientID, data := range raw {
		var state testPersistedClientSession
		if err := json.Unmarshal(data, &state); err != nil {
			t.Fatalf("unmarshal persisted client session: %v", err)
		}
		decoded[clientID] = state.CurrentSessionID
	}
	return decoded
}
