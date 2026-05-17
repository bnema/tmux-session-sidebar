package runtime

import (
	"errors"
	"testing"

	coreerrors "github.com/bnema/tmux-session-sidebar/core/errors"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
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

func TestQuickSwitchTarget(t *testing.T) {
	visible := []sessions.View{{Name: "123", Visible: true}, {Name: "alpha", Visible: true}, {Name: "beta", Visible: true}}
	tests := []struct {
		name    string
		slot    int
		want    string
		wantErr error
	}{
		{name: "slot one skips numeric", slot: 1, want: "alpha"},
		{name: "slot two", slot: 2, want: "beta"},
		{name: "missing slot", slot: 3, wantErr: coreerrors.ErrNoQuickSwitchSlot},
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
