package runtime

import (
	"context"
	"fmt"

	coreerrors "github.com/bnema/tmux-session-sidebar/core/errors"
	"github.com/bnema/tmux-session-sidebar/core/quickswitch"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
)

func (s *Service) SwitchSession(ctx context.Context, clientID string, sessionName string) error {
	if err := sessions.ValidateName(sessionName); err != nil {
		return err
	}
	return s.tmuxCtl.SwitchClientSession(ctx, clientID, sessionName)
}

func RenameSession(existing []sessions.View, oldName string, newName string) error {
	if err := sessions.ValidateName(newName); err != nil {
		return err
	}
	for _, session := range existing {
		if session.Name == newName && session.Name != oldName {
			return coreerrors.ErrDuplicateSession
		}
	}
	return nil
}

func KillSession(existing []sessions.View, target string) error {
	if len(existing) <= 1 {
		return coreerrors.ErrLastSessionKill
	}
	for _, session := range existing {
		if session.Name == target {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", coreerrors.ErrMissingSession, target)
}

func QuickSwitchTarget(visible []sessions.View, slot int) (string, error) {
	target := quickswitch.BuildSlotMap(visible)[slot]
	if target == "" {
		return "", coreerrors.ErrNoQuickSwitchSlot
	}
	return target, nil
}
