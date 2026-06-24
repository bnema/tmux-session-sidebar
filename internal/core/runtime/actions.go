package runtime

import (
	"context"
	"fmt"

	coreerrors "github.com/bnema/tmux-session-sidebar/internal/core/errors"
	"github.com/bnema/tmux-session-sidebar/internal/core/projects"
	"github.com/bnema/tmux-session-sidebar/internal/core/quickswitch"
	"github.com/bnema/tmux-session-sidebar/internal/core/sessions"
	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

func (s *Service) SwitchSession(ctx context.Context, clientID string, sessionName string) error {
	if s.control == nil {
		return ErrMissingControl
	}
	if err := sessions.ValidateName(sessionName); err != nil {
		return err
	}
	return s.control.SwitchClientSession(ctx, clientID, sessionName)
}

func (s *Service) SessionViews(ctx context.Context) ([]sessions.View, error) {
	if s.query == nil {
		return nil, ErrMissingQuery
	}
	snapshots, err := s.query.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]sessions.View, 0, len(snapshots))
	for _, snapshot := range snapshots {
		views = append(views, sessions.View{SessionID: snapshot.ID, Name: snapshot.Name, Visible: true})
	}
	return views, nil
}

func (s *Service) CreateProjectSession(ctx context.Context, clientID string, existing []sessions.View, candidate projects.Candidate) error {
	if s.control == nil {
		return ErrMissingControl
	}
	plan := ProjectSessionDecision(existing, candidate)
	if err := s.CreateDetachedProjectSession(ctx, existing, plan); err != nil {
		return err
	}
	return s.control.SwitchClientSession(ctx, clientID, plan.SessionName)
}

func (s *Service) CreateDetachedProjectSession(ctx context.Context, existing []sessions.View, plan ProjectSessionPlan) error {
	if !plan.Create {
		return sessions.ValidateName(plan.SessionName)
	}
	if s.control == nil {
		return ErrMissingControl
	}
	if err := ValidateCreateSession(existing, plan.SessionName); err != nil {
		return err
	}
	if err := s.control.CreateSession(ctx, plan.SessionName, plan.ProjectPath); err != nil {
		return err
	}
	if s.meta != nil {
		if err := s.meta.SaveSessionMetadata(ctx, plan.SessionName, plan.Metadata()); err != nil {
			return s.rollbackCreatedSession(ctx, plan.SessionName, fmt.Errorf("save project metadata for %s: %w", plan.SessionName, err))
		}
	}
	return nil
}

func (s *Service) CreateAdhocSession(ctx context.Context, clientID string, existing []sessions.View, name string, path string) error {
	plan := AdhocSessionDecision(existing, name, path)
	if err := s.CreateDetachedAdhocSession(ctx, existing, plan); err != nil {
		return err
	}
	return s.SwitchSession(ctx, clientID, name)
}

func (s *Service) CreateDetachedAdhocSession(ctx context.Context, existing []sessions.View, plan AdhocSessionPlan) error {
	if !plan.Create {
		return sessions.ValidateName(plan.SessionName)
	}
	if s.control == nil {
		return ErrMissingControl
	}
	if err := ValidateCreateSession(existing, plan.SessionName); err != nil {
		return err
	}
	if err := s.control.CreateSession(ctx, plan.SessionName, plan.Path); err != nil {
		return err
	}
	if s.meta != nil {
		if err := s.meta.SaveSessionMetadata(ctx, plan.SessionName, plan.Metadata()); err != nil {
			return s.rollbackCreatedSession(ctx, plan.SessionName, fmt.Errorf("save adhoc metadata for %s: %w", plan.SessionName, err))
		}
	}
	return nil
}

func (s *Service) RenameSession(ctx context.Context, existing []sessions.View, oldName string, newName string) error {
	if s.control == nil {
		return ErrMissingControl
	}
	if err := ValidateRenameSession(existing, oldName, newName); err != nil {
		return err
	}
	if !sessionNameExists(existing, oldName) {
		return fmt.Errorf("%w: %s", coreerrors.ErrMissingSession, oldName)
	}
	return s.control.RenameSession(ctx, oldName, newName)
}

func (s *Service) rollbackCreatedSession(ctx context.Context, sessionName string, original error) error {
	if err := s.control.KillSession(ctx, sessionName); err != nil {
		return fmt.Errorf("%w (rollback failed for %s: %v)", original, sessionName, err)
	}
	return original
}

func sessionNameExists(existing []sessions.View, name string) bool {
	for _, session := range existing {
		if session.Name == name {
			return true
		}
	}
	return false
}

func (s *Service) KillSession(ctx context.Context, existing []sessions.View, target string) error {
	if s.control == nil {
		return ErrMissingControl
	}
	if err := ValidateKillSession(existing, target); err != nil {
		return err
	}
	return s.control.KillSession(ctx, target)
}

type ProjectSessionPlan struct {
	SessionName string
	ProjectPath string
	Create      bool
}

func (p ProjectSessionPlan) Metadata() ports.SessionMetadata {
	return ports.SessionMetadata{Kind: "project", ProjectPath: p.ProjectPath, LastPath: p.ProjectPath}
}

type AdhocSessionPlan struct {
	SessionName string
	Path        string
	Create      bool
}

func (p AdhocSessionPlan) Metadata() ports.SessionMetadata {
	return ports.SessionMetadata{Kind: "adhoc", LastPath: p.Path}
}

func ValidateCreateSession(existing []sessions.View, name string) error {
	if err := sessions.ValidateName(name); err != nil {
		return err
	}
	for _, session := range existing {
		if session.Name == name {
			return coreerrors.ErrDuplicateSession
		}
	}
	return nil
}

func ProjectSessionDecision(existing []sessions.View, candidate projects.Candidate) ProjectSessionPlan {
	plan := ProjectSessionPlan{SessionName: candidate.SessionName, ProjectPath: candidate.Path, Create: true}
	for _, session := range existing {
		if session.Name == candidate.SessionName {
			plan.Create = false
			return plan
		}
	}
	return plan
}

func AdhocSessionDecision(existing []sessions.View, name string, path string) AdhocSessionPlan {
	plan := AdhocSessionPlan{SessionName: name, Path: path, Create: true}
	for _, session := range existing {
		if session.Name == name {
			plan.Create = false
			return plan
		}
	}
	return plan
}

func ValidateRenameSession(existing []sessions.View, oldName string, newName string) error {
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

func ValidateKillSession(existing []sessions.View, target string) error {
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
