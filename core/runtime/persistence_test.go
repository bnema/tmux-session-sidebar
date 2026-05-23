package runtime

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
	"github.com/bnema/tmux-session-sidebar/ports/mocks"
	"github.com/stretchr/testify/mock"
)

func TestRestorePersistedSessions(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	root := t.TempDir()
	home := t.TempDir()
	boom := errors.New("boom")

	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockTmuxQueryPort(t)
	control := mocks.NewMockTmuxControlPort(t)

	store.EXPECT().Load(ctx, serverID).Return(ports.PersistedState{Sessions: map[string]ports.SessionMetadata{
		"alpha":    {ProjectPath: root},
		"beta":     {LastPath: root},
		"123":      {LastPath: root},
		"__hidden": {LastPath: root},
		"bad:name": {LastPath: root},
		"bad name": {LastPath: root},
		"live":     {LastPath: root},
		"fallback": {LastPath: root + "/missing"},
		"fail":     {LastPath: root},
	}}, nil)
	query.EXPECT().ListSessions(ctx).Return([]ports.TmuxSessionSnapshot{{Name: "live"}}, nil)
	control.EXPECT().CreateSession(ctx, "alpha", root).Return(nil)
	control.EXPECT().CreateSession(ctx, "beta", root).Return(nil)
	control.EXPECT().CreateSession(ctx, "fallback", home).Return(nil)
	control.EXPECT().CreateSession(ctx, "fail", root).Return(boom)

	report := NewService(nil, query, control, store).RestorePersistedSessions(ctx, serverID, home)

	assertStringSet(t, report.Restored, []string{"alpha", "beta", "fallback"})
	assertStringSet(t, report.Skipped, []string{"123", "__hidden", "bad:name", "bad name", "live"})
	if !errors.Is(report.Failed["fail"], boom) {
		t.Fatalf("failed[fail] = %v, want %v", report.Failed["fail"], boom)
	}
	if len(report.Failed) != 1 {
		t.Fatalf("failed = %#v, want only fail", report.Failed)
	}
}

func TestRestorePersistedSessionsFallsBackToDotWhenNoUsefulPath(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockTmuxQueryPort(t)
	control := mocks.NewMockTmuxControlPort(t)

	store.EXPECT().Load(ctx, serverID).Return(ports.PersistedState{Sessions: map[string]ports.SessionMetadata{
		"alpha": {ProjectPath: "relative", LastPath: ""},
	}}, nil)
	query.EXPECT().ListSessions(ctx).Return(nil, nil)
	control.EXPECT().CreateSession(ctx, "alpha", ".").Return(nil)

	report := NewService(nil, query, control, store).RestorePersistedSessions(ctx, serverID, "")
	assertStringSet(t, report.Restored, []string{"alpha"})
	if len(report.Failed) != 0 {
		t.Fatalf("failed = %#v, want empty", report.Failed)
	}
}

func TestRestorePersistedSessionsReportsInfrastructureFailures(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	boom := errors.New("boom")

	t.Run("store load", func(t *testing.T) {
		store := mocks.NewMockStateStorePort(t)
		query := mocks.NewMockTmuxQueryPort(t)
		control := mocks.NewMockTmuxControlPort(t)
		store.EXPECT().Load(ctx, serverID).Return(ports.PersistedState{}, boom)

		report := NewService(nil, query, control, store).RestorePersistedSessions(ctx, serverID, t.TempDir())
		if !errors.Is(report.SystemFailures["load"], boom) {
			t.Fatalf("system failure load = %v, want %v", report.SystemFailures["load"], boom)
		}
		if len(report.Failed) != 0 {
			t.Fatalf("session failures = %#v, want empty", report.Failed)
		}
	})

	t.Run("list sessions", func(t *testing.T) {
		store := mocks.NewMockStateStorePort(t)
		query := mocks.NewMockTmuxQueryPort(t)
		control := mocks.NewMockTmuxControlPort(t)
		store.EXPECT().Load(ctx, serverID).Return(ports.PersistedState{Sessions: map[string]ports.SessionMetadata{"alpha": {LastPath: t.TempDir()}}}, nil)
		query.EXPECT().ListSessions(ctx).Return(nil, boom)

		report := NewService(nil, query, control, store).RestorePersistedSessions(ctx, serverID, t.TempDir())
		if !errors.Is(report.SystemFailures["list"], boom) {
			t.Fatalf("system failure list = %v, want %v", report.SystemFailures["list"], boom)
		}
		if len(report.Failed) != 0 {
			t.Fatalf("session failures = %#v, want empty", report.Failed)
		}
	})
}

func TestCaptureLiveSessions(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	alphaPath := t.TempDir()
	betaPath := t.TempDir()
	missingPath := t.TempDir() + "/missing"

	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockTmuxQueryPort(t)

	initial := ports.PersistedState{
		Sessions: map[string]ports.SessionMetadata{
			"beta":   {Kind: "project", ProjectPath: "/projects/beta"},
			"absent": {Kind: "captured", LastPath: "/still/there"},
		},
		SessionOrder: []string{"absent", "beta"},
	}
	store.EXPECT().Load(ctx, serverID).Return(initial, nil)
	query.EXPECT().ListSessions(ctx).Return([]ports.TmuxSessionSnapshot{
		{Name: "alpha"},
		{Name: "beta"},
		{Name: "123"},
		{Name: "__hidden"},
		{Name: "relative"},
		{Name: "empty"},
		{Name: "missing"},
	}, nil)
	query.EXPECT().SessionPath(ctx, "alpha").Return(alphaPath, nil)
	query.EXPECT().SessionPath(ctx, "beta").Return(betaPath, nil)
	query.EXPECT().SessionPath(ctx, "relative").Return("relative/path", nil)
	query.EXPECT().SessionPath(ctx, "empty").Return("  ", nil)
	query.EXPECT().SessionPath(ctx, "missing").Return(missingPath, nil)
	query.EXPECT().ListClients(ctx).Return(nil, nil)

	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		wantSessions := map[string]ports.SessionMetadata{
			"alpha": {Kind: "captured", LastPath: alphaPath},
			"beta":  {Kind: "project", ProjectPath: "/projects/beta", LastPath: betaPath},
		}
		wantOrder := []string{"beta", "alpha", "123", "__hidden", "relative", "empty", "missing"}
		return reflect.DeepEqual(state.Sessions, wantSessions) && reflect.DeepEqual(state.SessionOrder, wantOrder)
	})).Return(nil)

	if err := NewService(nil, query, nil, store).CaptureLiveSessions(ctx, serverID); err != nil {
		t.Fatalf("CaptureLiveSessions error: %v", err)
	}
}

func TestCaptureLiveSessionsPreservesOrderForNonPersistableLiveSessions(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	alphaPath := t.TempDir()
	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockTmuxQueryPort(t)

	initial := ports.PersistedState{
		Sessions:     map[string]ports.SessionMetadata{"alpha": {Kind: "project", ProjectPath: "/projects/alpha", LastPath: "/projects/alpha"}},
		SessionOrder: []string{"2", "alpha", "__scratch", "1"},
	}
	store.EXPECT().Load(ctx, serverID).Return(initial, nil)
	query.EXPECT().ListSessions(ctx).Return([]ports.TmuxSessionSnapshot{{Name: "1"}, {Name: "alpha"}, {Name: "2"}, {Name: "__scratch"}}, nil)
	query.EXPECT().SessionPath(ctx, "alpha").Return(alphaPath, nil)
	query.EXPECT().ListClients(ctx).Return(nil, nil)
	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		wantSessions := map[string]ports.SessionMetadata{"alpha": {Kind: "project", ProjectPath: "/projects/alpha", LastPath: alphaPath}}
		wantOrder := []string{"2", "alpha", "__scratch", "1"}
		return reflect.DeepEqual(state.Sessions, wantSessions) && reflect.DeepEqual(state.SessionOrder, wantOrder)
	})).Return(nil)

	if err := NewService(nil, query, nil, store).CaptureLiveSessions(ctx, serverID); err != nil {
		t.Fatalf("CaptureLiveSessions error: %v", err)
	}
}

func TestCaptureLiveSessionsIgnoresSessionPathErrors(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	alphaPath := t.TempDir()
	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockTmuxQueryPort(t)

	store.EXPECT().Load(ctx, serverID).Return(ports.PersistedState{}, nil)
	query.EXPECT().ListSessions(ctx).Return([]ports.TmuxSessionSnapshot{{Name: "alpha"}, {Name: "fail"}}, nil)
	query.EXPECT().SessionPath(ctx, "alpha").Return(alphaPath, nil)
	query.EXPECT().ListClients(ctx).Return(nil, nil)
	query.EXPECT().SessionPath(ctx, "fail").Return("", errors.New("pane gone"))
	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		return reflect.DeepEqual(state.Sessions, map[string]ports.SessionMetadata{"alpha": {Kind: "captured", LastPath: alphaPath}})
	})).Return(nil)

	if err := NewService(nil, query, nil, store).CaptureLiveSessions(ctx, serverID); err != nil {
		t.Fatalf("CaptureLiveSessions error: %v", err)
	}
}

func TestCaptureLiveSessionsStillSavesMetadataWhenHeatCollectionFails(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	alphaPath := t.TempDir()
	boom := errors.New("list clients failed")
	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockTmuxQueryPort(t)

	initial := ports.PersistedState{Heat: map[string][]byte{"alpha": []byte(`{"Score":600}`)}}
	store.EXPECT().Load(ctx, serverID).Return(initial, nil)
	query.EXPECT().ListSessions(ctx).Return([]ports.TmuxSessionSnapshot{{Name: "alpha"}}, nil)
	query.EXPECT().SessionPath(ctx, "alpha").Return(alphaPath, nil)
	query.EXPECT().ListClients(ctx).Return(nil, boom)
	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		return reflect.DeepEqual(state.Sessions, map[string]ports.SessionMetadata{"alpha": {Kind: "captured", LastPath: alphaPath}}) && reflect.DeepEqual(state.Heat, initial.Heat)
	})).Return(nil)

	if err := NewService(nil, query, nil, store).CaptureLiveSessions(ctx, serverID); err != nil {
		t.Fatalf("CaptureLiveSessions error: %v", err)
	}
}

func TestCaptureLiveSessionsReturnsSaveError(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	alphaPath := t.TempDir()
	boom := errors.New("boom")
	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockTmuxQueryPort(t)

	store.EXPECT().Load(ctx, serverID).Return(ports.PersistedState{}, nil)
	query.EXPECT().ListSessions(ctx).Return([]ports.TmuxSessionSnapshot{{Name: "alpha"}}, nil)
	query.EXPECT().SessionPath(ctx, "alpha").Return(alphaPath, nil)
	query.EXPECT().ListClients(ctx).Return(nil, nil)
	store.EXPECT().Save(ctx, serverID, mock.Anything).Return(boom)

	err := NewService(nil, query, nil, store).CaptureLiveSessions(ctx, serverID)
	if !errors.Is(err, boom) {
		t.Fatalf("CaptureLiveSessions error = %v, want %v", err, boom)
	}
}

func assertStringSet(t *testing.T, got []string, want []string) {
	t.Helper()
	got = append([]string(nil), got...)
	want = append([]string(nil), want...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("strings = %#v, want %#v", got, want)
	}
}
