package runtime

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/bnema/tmux-session-sidebar/core/heat"
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

func TestReconcilePinColorsKeepsNilMapNil(t *testing.T) {
	if got := reconcilePinColors(nil, []ports.TmuxSessionSnapshot{{Name: "alpha"}}); got != nil {
		t.Fatalf("reconcilePinColors(nil) = %#v, want nil", got)
	}
}

func TestCaptureLiveSessionsPrunesOrphanSessionColors(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockTmuxQueryPort(t)

	initial := ports.PersistedState{
		Sessions:       map[string]ports.SessionMetadata{},
		PinnedSessions: []string{"missing"},
		PinColors:      map[string]string{"alpha": "#38bdf8", "missing": "#f87171"},
	}
	store.EXPECT().Load(ctx, serverID).Return(initial, nil)
	query.EXPECT().ListSessions(ctx).Return([]ports.TmuxSessionSnapshot{{Name: "alpha"}}, nil)
	query.EXPECT().SessionPath(ctx, "alpha").Return(t.TempDir(), nil)
	query.EXPECT().ListClients(ctx).Return(nil, nil)
	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		return len(state.PinnedSessions) == 0 && reflect.DeepEqual(state.PinColors, map[string]string{"alpha": "#38bdf8"})
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

func TestApplyRecentSessionOrderUsesLastActiveAt(t *testing.T) {
	now := time.Date(2026, 5, 26, 9, 0, 0, 0, time.UTC)
	state := ports.PersistedState{
		SessionOrder: []string{"gamma", "alpha", "beta"},
		Heat: encodeHeatStateMap(map[string]heat.State{
			"alpha": {LastVisitedAt: now, LastActiveAt: now.Add(-30 * time.Minute)},
			"beta":  {LastActiveAt: now.Add(-5 * time.Minute)},
			"gamma": {LastVisitedAt: now},
		}),
	}
	live := []ports.TmuxSessionSnapshot{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}

	applyRecentSessionOrder(&state, live, ports.ConfigSnapshot{AutoSortRecentInterval: 24 * time.Hour}, now)

	if want := []string{"beta", "alpha", "gamma"}; !reflect.DeepEqual(state.SessionOrder, want) {
		t.Fatalf("SessionOrder = %#v, want %#v", state.SessionOrder, want)
	}
	if state.Sidebar == nil || state.Sidebar.AutoSortRecentRunAt != now.Format(time.RFC3339Nano) {
		t.Fatalf("AutoSortRecentRunAt = %#v, want %s", state.Sidebar, now.Format(time.RFC3339Nano))
	}
}

func TestApplyRecentSessionOrderReordersSidebarLayoutCategories(t *testing.T) {
	now := time.Date(2026, 5, 26, 9, 0, 0, 0, time.UTC)
	state := ports.PersistedState{
		SessionOrder: []string{"alpha", "beta", "gamma", "delta"},
		SidebarLayout: &ports.SidebarLayout{Items: []ports.SidebarLayoutItem{
			{ID: "category:work", Kind: "category", Category: &ports.SidebarLayoutCategory{ID: "category:work", Name: "Work", Sessions: []ports.SidebarLayoutSessionRef{{Name: "alpha"}, {Name: "beta"}}}},
			{ID: "category:personal", Kind: "category", Category: &ports.SidebarLayoutCategory{ID: "category:personal", Name: "Personal", Sessions: []ports.SidebarLayoutSessionRef{{Name: "gamma"}, {Name: "delta"}}}},
		}},
		Heat: encodeHeatStateMap(map[string]heat.State{
			"alpha": {LastActiveAt: now.Add(-30 * time.Minute)},
			"beta":  {LastActiveAt: now.Add(-5 * time.Minute)},
			"gamma": {LastActiveAt: now.Add(-40 * time.Minute)},
			"delta": {LastActiveAt: now.Add(-10 * time.Minute)},
		}),
	}
	live := []ports.TmuxSessionSnapshot{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}, {Name: "delta"}}

	applyRecentSessionOrder(&state, live, ports.ConfigSnapshot{AutoSortRecentInterval: 24 * time.Hour}, now)

	if got, want := sidebarLayoutSessionNames(state.SidebarLayout.Items[0].Category.Sessions), []string{"beta", "alpha"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("work category sessions = %#v, want %#v", got, want)
	}
	if got, want := sidebarLayoutSessionNames(state.SidebarLayout.Items[1].Category.Sessions), []string{"delta", "gamma"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("personal category sessions = %#v, want %#v", got, want)
	}
}

func TestApplyRecentSessionOrderKeepsPinnedSessionsAtCategoryPositions(t *testing.T) {
	now := time.Date(2026, 5, 26, 9, 0, 0, 0, time.UTC)
	state := ports.PersistedState{
		SessionOrder:   []string{"alpha", "beta", "gamma", "delta"},
		PinnedSessions: []string{"beta"},
		SidebarLayout: &ports.SidebarLayout{Items: []ports.SidebarLayoutItem{
			{ID: "category:work", Kind: "category", Category: &ports.SidebarLayoutCategory{ID: "category:work", Name: "Work", Sessions: []ports.SidebarLayoutSessionRef{{Name: "alpha"}, {Name: "beta"}}}},
			{ID: "category:personal", Kind: "category", Category: &ports.SidebarLayoutCategory{ID: "category:personal", Name: "Personal", Sessions: []ports.SidebarLayoutSessionRef{{Name: "gamma"}, {Name: "delta"}}}},
		}},
		Heat: encodeHeatStateMap(map[string]heat.State{
			"alpha": {LastActiveAt: now.Add(-30 * time.Minute)},
			"beta":  {LastActiveAt: now.Add(-4 * time.Hour)},
			"gamma": {LastActiveAt: now.Add(-40 * time.Minute)},
			"delta": {LastActiveAt: now.Add(-10 * time.Minute)},
		}),
	}
	live := []ports.TmuxSessionSnapshot{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}, {Name: "delta"}}

	applyRecentSessionOrder(&state, live, ports.ConfigSnapshot{AutoSortRecentInterval: 24 * time.Hour}, now)

	if got, want := sidebarLayoutSessionNames(state.SidebarLayout.Items[0].Category.Sessions), []string{"alpha", "beta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("work category sessions = %#v, want pinned beta preserved at category index 1: %#v", got, want)
	}
	if got, want := sidebarLayoutSessionNames(state.SidebarLayout.Items[1].Category.Sessions), []string{"delta", "gamma"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("personal category sessions = %#v, want %#v", got, want)
	}
}

func TestApplyRecentSessionOrderKeepsPinnedSessionsAtTheirPositions(t *testing.T) {
	now := time.Date(2026, 5, 26, 9, 0, 0, 0, time.UTC)
	state := ports.PersistedState{
		SessionOrder:   []string{"alpha", "beta", "gamma", "delta"},
		PinnedSessions: []string{"beta"},
		Heat: encodeHeatStateMap(map[string]heat.State{
			"alpha": {LastActiveAt: now.Add(-30 * time.Minute)},
			"beta":  {LastActiveAt: now.Add(-4 * time.Hour)},
			"gamma": {LastActiveAt: now.Add(-10 * time.Minute)},
			"delta": {LastActiveAt: now.Add(-5 * time.Minute)},
		}),
	}
	live := []ports.TmuxSessionSnapshot{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}, {Name: "delta"}}

	applyRecentSessionOrder(&state, live, ports.ConfigSnapshot{AutoSortRecentInterval: 24 * time.Hour}, now)

	if want := []string{"delta", "beta", "gamma", "alpha"}; !reflect.DeepEqual(state.SessionOrder, want) {
		t.Fatalf("SessionOrder = %#v, want %#v", state.SessionOrder, want)
	}
}

func TestApplyRecentSessionOrderHonorsConfiguredInterval(t *testing.T) {
	now := time.Date(2026, 5, 26, 9, 0, 0, 0, time.UTC)
	live := []ports.TmuxSessionSnapshot{{Name: "alpha"}, {Name: "beta"}}
	heatState := encodeHeatStateMap(map[string]heat.State{
		"alpha": {LastActiveAt: now},
		"beta":  {LastActiveAt: now.Add(-time.Hour)},
	})

	t.Run("skips before interval", func(t *testing.T) {
		state := ports.PersistedState{
			SessionOrder: []string{"beta", "alpha"},
			Sidebar:      &ports.SidebarState{AutoSortRecentRunAt: now.Add(-9 * time.Minute).Format(time.RFC3339Nano)},
			Heat:         heatState,
		}

		applyRecentSessionOrder(&state, live, ports.ConfigSnapshot{AutoSortRecentInterval: 10 * time.Minute}, now)

		if want := []string{"beta", "alpha"}; !reflect.DeepEqual(state.SessionOrder, want) {
			t.Fatalf("SessionOrder = %#v, want unchanged %#v", state.SessionOrder, want)
		}
	})

	t.Run("skips using legacy run date", func(t *testing.T) {
		state := ports.PersistedState{
			SessionOrder: []string{"beta", "alpha"},
			Sidebar:      &ports.SidebarState{AutoSortRecentRunDate: "2026-05-26"},
			Heat:         heatState,
		}

		applyRecentSessionOrder(&state, live, ports.ConfigSnapshot{AutoSortRecentInterval: 24 * time.Hour}, now)

		if want := []string{"beta", "alpha"}; !reflect.DeepEqual(state.SessionOrder, want) {
			t.Fatalf("SessionOrder = %#v, want unchanged %#v", state.SessionOrder, want)
		}
	})

	t.Run("runs after interval and clears legacy run date", func(t *testing.T) {
		state := ports.PersistedState{
			SessionOrder: []string{"beta", "alpha"},
			Sidebar:      &ports.SidebarState{AutoSortRecentRunAt: now.Add(-10 * time.Minute).Format(time.RFC3339Nano), AutoSortRecentRunDate: "2026-05-25"},
			Heat:         heatState,
		}

		applyRecentSessionOrder(&state, live, ports.ConfigSnapshot{AutoSortRecentInterval: 10 * time.Minute}, now)

		if want := []string{"alpha", "beta"}; !reflect.DeepEqual(state.SessionOrder, want) {
			t.Fatalf("SessionOrder = %#v, want %#v", state.SessionOrder, want)
		}
		if state.Sidebar.AutoSortRecentRunDate != "" {
			t.Fatalf("AutoSortRecentRunDate = %q, want cleared", state.Sidebar.AutoSortRecentRunDate)
		}
	})
}

func TestApplyRecentSessionOrderSkipsWhenDisabled(t *testing.T) {
	now := time.Date(2026, 5, 26, 9, 0, 0, 0, time.UTC)
	state := ports.PersistedState{
		SessionOrder: []string{"beta", "alpha"},
		Heat: encodeHeatStateMap(map[string]heat.State{
			"alpha": {LastActiveAt: now},
		}),
	}
	live := []ports.TmuxSessionSnapshot{{Name: "alpha"}, {Name: "beta"}}

	applyRecentSessionOrder(&state, live, ports.ConfigSnapshot{}, now)

	if want := []string{"beta", "alpha"}; !reflect.DeepEqual(state.SessionOrder, want) {
		t.Fatalf("SessionOrder = %#v, want unchanged %#v", state.SessionOrder, want)
	}
	if state.Sidebar != nil {
		t.Fatalf("Sidebar = %#v, want nil when disabled", state.Sidebar)
	}
}

func TestCaptureLiveSessionsWithConfigReconcilesLiveSessionsBeforeRecentOrder(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	alphaPath := t.TempDir()
	betaPath := t.TempDir()
	deltaPath := t.TempDir()
	now := time.Now()
	store := mocks.NewMockStateStorePort(t)
	query := runtimeTestQuery{
		live: []ports.TmuxSessionSnapshot{
			{ID: "$1", Name: "alpha"},
			{ID: "$2", Name: "beta"},
			{ID: "$3", Name: "delta"},
		},
		sessionPaths: map[string]string{"alpha": alphaPath, "beta": betaPath, "delta": deltaPath},
	}

	initial := ports.PersistedState{
		Sessions:     map[string]ports.SessionMetadata{"gamma": {Kind: "captured", LastPath: t.TempDir()}},
		SessionOrder: []string{"gamma", "alpha", "beta"},
		Heat: encodeHeatStateMap(map[string]heat.State{
			"alpha": {LastActiveAt: now.Add(-30 * time.Minute)},
			"beta":  {LastActiveAt: now.Add(-5 * time.Minute)},
			"delta": {},
		}),
	}
	store.EXPECT().Load(ctx, serverID).Return(initial, nil)
	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		wantOrder := []string{"beta", "alpha", "delta"}
		wantSessions := map[string]ports.SessionMetadata{
			"alpha": {Kind: "captured", LastPath: alphaPath},
			"beta":  {Kind: "captured", LastPath: betaPath},
			"delta": {Kind: "captured", LastPath: deltaPath},
		}
		return reflect.DeepEqual(state.SessionOrder, wantOrder) &&
			reflect.DeepEqual(state.Sessions, wantSessions) &&
			state.Sidebar != nil && state.Sidebar.AutoSortRecentRunAt != ""
	})).Return(nil)

	if err := NewService(nil, query, nil, store).CaptureLiveSessionsWithConfig(ctx, serverID, ports.ConfigSnapshot{AutoSortRecentInterval: 24 * time.Hour}); err != nil {
		t.Fatalf("CaptureLiveSessionsWithConfig error: %v", err)
	}
}

func TestCaptureSessionHeatWithConfigDoesNotConsumeRecentOrderWhenHeatCaptureFails(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	boom := errors.New("list clients failed")
	store := mocks.NewMockStateStorePort(t)
	query := mocks.NewMockTmuxQueryPort(t)

	initial := ports.PersistedState{
		SessionOrder: []string{"beta", "alpha"},
		Heat: encodeHeatStateMap(map[string]heat.State{
			"alpha": {LastActiveAt: time.Now().Add(-5 * time.Minute)},
			"beta":  {LastActiveAt: time.Now().Add(-time.Hour)},
		}),
	}
	store.EXPECT().Load(ctx, serverID).Return(initial, nil)
	query.EXPECT().ListSessions(ctx).Return([]ports.TmuxSessionSnapshot{{Name: "alpha"}, {Name: "beta"}}, nil)
	query.EXPECT().ListClients(ctx).Return(nil, boom)
	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		return reflect.DeepEqual(state.SessionOrder, []string{"beta", "alpha"}) && state.Sidebar == nil
	})).Return(nil)

	if err := NewService(nil, query, nil, store).CaptureSessionHeatWithConfig(ctx, serverID, ports.ConfigSnapshot{AutoSortRecentInterval: 24 * time.Hour}); err != nil {
		t.Fatalf("CaptureSessionHeatWithConfig error: %v", err)
	}
}

func TestCaptureSessionHeatWithConfigDoesNotConsumeRecentOrderWhenPaneSampleFails(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	now := time.Now()
	store := mocks.NewMockStateStorePort(t)
	query := runtimeTestQuery{
		live:          []ports.TmuxSessionSnapshot{{ID: "$1", Name: "alpha"}, {ID: "$2", Name: "beta"}},
		clients:       []ports.TmuxClientSnapshot{},
		panes:         []ports.TmuxPaneSnapshot{{PaneID: "%1", SessionID: "$1", SessionName: "alpha"}},
		captureErrors: map[string]error{"%1": errors.New("capture-pane failed")},
	}

	initial := ports.PersistedState{
		SessionOrder: []string{"beta", "alpha"},
		Heat: encodeHeatStateMap(map[string]heat.State{
			"alpha": {LastActiveAt: now.Add(-5 * time.Minute)},
			"beta":  {LastActiveAt: now.Add(-time.Hour)},
		}),
	}
	store.EXPECT().Load(ctx, serverID).Return(initial, nil)
	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		return reflect.DeepEqual(state.SessionOrder, []string{"beta", "alpha"}) && state.Sidebar == nil
	})).Return(nil)

	if err := NewService(nil, query, nil, store).CaptureSessionHeatWithConfig(ctx, serverID, ports.ConfigSnapshot{AutoSortRecentInterval: 24 * time.Hour}); err != nil {
		t.Fatalf("CaptureSessionHeatWithConfig error: %v", err)
	}
}

func TestCaptureSessionHeatWithConfigAutoSortsCompleteCategoriesWhenAnotherCategoryPaneSampleFails(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	now := time.Now()
	store := mocks.NewMockStateStorePort(t)
	query := runtimeTestQuery{
		live: []ports.TmuxSessionSnapshot{
			{ID: "$1", Name: "alpha"},
			{ID: "$2", Name: "beta"},
			{ID: "$3", Name: "gamma"},
		},
		clients: []ports.TmuxClientSnapshot{},
		panes: []ports.TmuxPaneSnapshot{
			{PaneID: "%1", SessionID: "$1", SessionName: "alpha"},
			{PaneID: "%2", SessionID: "$2", SessionName: "beta"},
			{PaneID: "%3", SessionID: "$3", SessionName: "gamma"},
		},
		captureText:   "stable pane text",
		captureErrors: map[string]error{"%3": errors.New("capture-pane failed")},
	}

	initial := ports.PersistedState{
		SessionOrder: []string{"alpha", "beta", "gamma"},
		SidebarLayout: &ports.SidebarLayout{Items: []ports.SidebarLayoutItem{
			{ID: "category:work", Kind: "category", Category: &ports.SidebarLayoutCategory{ID: "category:work", Name: "Work", Sessions: []ports.SidebarLayoutSessionRef{{Name: "alpha"}, {Name: "beta"}}}},
			{ID: "category:personal", Kind: "category", Category: &ports.SidebarLayoutCategory{ID: "category:personal", Name: "Personal", Sessions: []ports.SidebarLayoutSessionRef{{Name: "gamma"}}}},
		}},
		Heat: encodeHeatStateMap(map[string]heat.State{
			"alpha": {LastActiveAt: now.Add(-30 * time.Minute)},
			"beta":  {LastActiveAt: now.Add(-5 * time.Minute)},
			"gamma": {LastActiveAt: now.Add(-10 * time.Minute)},
		}),
	}
	store.EXPECT().Load(ctx, serverID).Return(initial, nil)
	store.EXPECT().Save(ctx, serverID, mock.MatchedBy(func(state ports.PersistedState) bool {
		work := sidebarLayoutSessionNames(state.SidebarLayout.Items[0].Category.Sessions)
		personal := sidebarLayoutSessionNames(state.SidebarLayout.Items[1].Category.Sessions)
		return reflect.DeepEqual(work, []string{"beta", "alpha"}) &&
			reflect.DeepEqual(personal, []string{"gamma"}) &&
			reflect.DeepEqual(state.SessionOrder, []string{"alpha", "beta", "gamma"}) &&
			state.Sidebar == nil
	})).Return(nil)

	if err := NewService(nil, query, nil, store).CaptureSessionHeatWithConfig(ctx, serverID, ports.ConfigSnapshot{AutoSortRecentInterval: 24 * time.Hour}); err != nil {
		t.Fatalf("CaptureSessionHeatWithConfig error: %v", err)
	}
}

func TestResetTransientHeatStateWrapsInfrastructureErrors(t *testing.T) {
	ctx := context.Background()
	serverID := "server"
	boom := errors.New("boom")

	t.Run("load", func(t *testing.T) {
		store := mocks.NewMockStateStorePort(t)
		store.EXPECT().Load(ctx, serverID).Return(ports.PersistedState{}, boom)

		err := NewService(nil, nil, nil, store).ResetTransientHeatState(ctx, serverID)
		if !errors.Is(err, boom) {
			t.Fatalf("ResetTransientHeatState error = %v, want %v", err, boom)
		}
		if !strings.Contains(err.Error(), "reset transient heat state: load") {
			t.Fatalf("ResetTransientHeatState error = %q, want load context", err)
		}
	})

	t.Run("save", func(t *testing.T) {
		store := mocks.NewMockStateStorePort(t)
		store.EXPECT().Load(ctx, serverID).Return(ports.PersistedState{Heat: map[string][]byte{"alpha": []byte(`{"Score":600}`)}}, nil)
		store.EXPECT().Save(ctx, serverID, mock.Anything).Return(boom)

		err := NewService(nil, nil, nil, store).ResetTransientHeatState(ctx, serverID)
		if !errors.Is(err, boom) {
			t.Fatalf("ResetTransientHeatState error = %v, want %v", err, boom)
		}
		if !strings.Contains(err.Error(), "reset transient heat state: save") {
			t.Fatalf("ResetTransientHeatState error = %q, want save context", err)
		}
	})
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

func sidebarLayoutSessionNames(refs []ports.SidebarLayoutSessionRef) []string {
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		names = append(names, ref.Name)
	}
	return names
}

type runtimeTestQuery struct {
	live          []ports.TmuxSessionSnapshot
	clients       []ports.TmuxClientSnapshot
	panes         []ports.TmuxPaneSnapshot
	sessionPaths  map[string]string
	captureText   string
	captureError  error
	captureErrors map[string]error
}

func (q runtimeTestQuery) ServerID(context.Context) (string, error) {
	return "server", nil
}

func (q runtimeTestQuery) ListSessions(context.Context) ([]ports.TmuxSessionSnapshot, error) {
	return q.live, nil
}

func (q runtimeTestQuery) ListClients(context.Context) ([]ports.TmuxClientSnapshot, error) {
	return q.clients, nil
}

func (q runtimeTestQuery) CurrentPanePath(context.Context, string) (string, error) {
	return "", nil
}

func (q runtimeTestQuery) SessionPath(_ context.Context, sessionName string) (string, error) {
	return q.sessionPaths[sessionName], nil
}

func (q runtimeTestQuery) PaneSize(context.Context, string) (ports.PaneSize, error) {
	return ports.PaneSize{}, nil
}

func (q runtimeTestQuery) ListPanes(context.Context) ([]ports.TmuxPaneSnapshot, error) {
	return q.panes, nil
}

func (q runtimeTestQuery) CapturePaneText(_ context.Context, paneID string, _ int) (string, error) {
	if q.captureErrors != nil && q.captureErrors[paneID] != nil {
		return "", q.captureErrors[paneID]
	}
	return q.captureText, q.captureError
}
