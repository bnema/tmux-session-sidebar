package runtime

import (
	"context"
	"errors"
	"testing"

	coreerrors "github.com/bnema/tmux-session-sidebar/core/errors"
	"github.com/bnema/tmux-session-sidebar/core/projects"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
	"github.com/bnema/tmux-session-sidebar/ports/mocks"
	"github.com/stretchr/testify/mock"
)

func TestRenameSessionValidation(t *testing.T) {
	existing := []sessions.View{{Name: "alpha"}, {Name: "beta"}}
	tests := []struct {
		name    string
		oldName string
		newName string
		wantErr error
	}{
		{name: "valid", oldName: "alpha", newName: "gamma"},
		{name: "same name", oldName: "alpha", newName: "alpha"},
		{name: "duplicate", oldName: "alpha", newName: "beta", wantErr: coreerrors.ErrDuplicateSession},
		{name: "invalid", oldName: "alpha", newName: "bad name", wantErr: coreerrors.ErrInvalidSessionName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRenameSession(existing, tt.oldName, tt.newName)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestKillSessionValidation(t *testing.T) {
	tests := []struct {
		name     string
		existing []sessions.View
		target   string
		wantErr  error
	}{
		{name: "valid", existing: []sessions.View{{Name: "alpha"}, {Name: "beta"}}, target: "alpha"},
		{name: "last session", existing: []sessions.View{{Name: "alpha"}}, target: "alpha", wantErr: coreerrors.ErrLastSessionKill},
		{name: "missing", existing: []sessions.View{{Name: "alpha"}, {Name: "beta"}}, target: "gamma", wantErr: coreerrors.ErrMissingSession},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKillSession(tt.existing, tt.target)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestServiceActionsRequireTmuxControl(t *testing.T) {
	ctx := context.Background()
	existing := []sessions.View{{Name: "alpha"}, {Name: "beta"}}
	tests := []struct {
		name string
		act  func(*Service) error
	}{
		{name: "switch", act: func(s *Service) error { return s.SwitchSession(ctx, "%1", "alpha") }},
		{name: "create project", act: func(s *Service) error {
			return s.CreateProjectSession(ctx, "%1", existing, projects.Candidate{Path: "/tmp/gamma", SessionName: "gamma"})
		}},
		{name: "create adhoc", act: func(s *Service) error { return s.CreateAdhocSession(ctx, "%1", existing, "gamma", "/tmp") }},
		{name: "rename", act: func(s *Service) error { return s.RenameSession(ctx, existing, "alpha", "gamma") }},
		{name: "kill", act: func(s *Service) error { return s.KillSession(ctx, existing, "alpha") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.act(&Service{}); !errors.Is(err, ErrMissingTmuxControl) {
				t.Fatalf("action error = %v, want %v", err, ErrMissingTmuxControl)
			}
		})
	}
}

func TestCreateSessionRollsBackWhenMetadataSaveFails(t *testing.T) {
	ctx := context.Background()
	saveErr := errors.New("save metadata")
	tests := []struct {
		name string
		act  func(*Service) error
	}{
		{
			name: "project",
			act: func(s *Service) error {
				return s.CreateProjectSession(ctx, "%1", []sessions.View{{Name: "alpha"}}, projects.Candidate{Path: "/tmp/project", SessionName: "project"})
			},
		},
		{
			name: "adhoc",
			act: func(s *Service) error {
				return s.CreateAdhocSession(ctx, "%1", []sessions.View{{Name: "alpha"}}, "adhoc", "/tmp")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			control := mocks.NewMockTmuxControlPort(t)
			meta := mocks.NewMockTmuxMetadataPort(t)
			name := tt.name
			path := "/tmp"
			if tt.name == "project" {
				path = "/tmp/project"
			}
			control.EXPECT().CreateSession(ctx, name, path).Return(nil)
			meta.EXPECT().SaveSessionMetadata(ctx, name, mock.Anything).Return(saveErr)
			control.EXPECT().KillSession(ctx, name).Return(nil)

			err := tt.act(NewService(nil, nil, control, nil).WithMetadata(meta))
			if !errors.Is(err, saveErr) {
				t.Fatalf("error = %v, want wrapped save error", err)
			}
		})
	}
}

func TestRenameSessionRequiresExistingOldName(t *testing.T) {
	ctx := context.Background()
	control := mocks.NewMockTmuxControlPort(t)
	service := NewService(nil, nil, control, nil)
	err := service.RenameSession(ctx, []sessions.View{{Name: "alpha"}, {Name: "beta"}}, "missing", "gamma")
	if !errors.Is(err, coreerrors.ErrMissingSession) {
		t.Fatalf("RenameSession() error = %v, want %v", err, coreerrors.ErrMissingSession)
	}
}

func TestValidateCreateSession(t *testing.T) {
	existing := []sessions.View{{Name: "alpha"}, {Name: "beta"}}
	tests := []struct {
		name    string
		newName string
		wantErr error
	}{
		{name: "valid new session", newName: "gamma"},
		{name: "invalid session name", newName: "bad name", wantErr: coreerrors.ErrInvalidSessionName},
		{name: "duplicate session", newName: "alpha", wantErr: coreerrors.ErrDuplicateSession},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateSession(existing, tt.newName)
			if tt.wantErr == nil && err != nil {
				t.Fatalf("ValidateCreateSession() error = %v, want nil", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateCreateSession() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestProjectSessionDecision(t *testing.T) {
	existing := []sessions.View{{Name: "alpha"}, {Name: "project"}}
	tests := []struct {
		name       string
		candidate  projects.Candidate
		wantName   string
		wantCreate bool
	}{
		{name: "existing project switches", candidate: projects.Candidate{Path: "/tmp/project", SessionName: "project"}, wantName: "project", wantCreate: false},
		{name: "new project creates", candidate: projects.Candidate{Path: "/tmp/new", SessionName: "new"}, wantName: "new", wantCreate: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProjectSessionDecision(existing, tt.candidate)
			if got.SessionName != tt.wantName || got.Create != tt.wantCreate {
				t.Fatalf("ProjectSessionDecision() = %#v, want name %q create %v", got, tt.wantName, tt.wantCreate)
			}
		})
	}
}

func TestQuickSwitchTarget(t *testing.T) {
	visible := []sessions.View{
		{Name: "123", Visible: true}, {Name: "alpha", Visible: true}, {Name: "beta", Visible: true},
		{Name: "gamma", Visible: true}, {Name: "delta", Visible: true}, {Name: "epsilon", Visible: true},
		{Name: "zeta", Visible: true}, {Name: "eta", Visible: true}, {Name: "theta", Visible: true},
		{Name: "iota", Visible: true}, {Name: "kappa", Visible: true}, {Name: "lambda", Visible: true},
	}
	tests := []struct {
		name    string
		slot    int
		want    string
		wantErr error
	}{
		{name: "slot one skips numeric", slot: 1, want: "alpha"},
		{name: "slot two", slot: 2, want: "beta"},
		{name: "slot eleven", slot: 11, want: "lambda"},
		{name: "missing slot", slot: 12, wantErr: coreerrors.ErrNoQuickSwitchSlot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := QuickSwitchTarget(visible, tt.slot)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("target = %q, want %q", got, tt.want)
			}
		})
	}
}
