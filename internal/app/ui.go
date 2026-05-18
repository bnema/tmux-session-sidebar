package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bnema/tmux-session-sidebar/adapters/uity"
	"github.com/bnema/tmux-session-sidebar/core/sessions"
)

func loadSessionItems(ctx context.Context) ([]uity.SessionItem, error) {
	current, err := tmux(ctx, "display-message", "-p", "#{session_name}")
	if err != nil {
		return nil, fmt.Errorf("getting current tmux session: %w", err)
	}
	views, err := runtimeService().SessionViews(ctx)
	if err != nil {
		return nil, err
	}
	current = strings.TrimSpace(current)
	names := sessions.ApplyOrder(sessionNames(sessions.FilterVisible(views, true)), loadSessionOrder(ctx))
	items := make([]uity.SessionItem, 0, len(names))
	slot := 1
	for _, name := range names {
		item := uity.SessionItem{Name: name, Current: name == current, Heat: "cool"}
		if item.Current {
			item.Heat = "current"
		}
		if !sessions.IsNumericName(name) && slot <= 10 {
			item.Slot = slot
			slot++
		}
		items = append(items, item)
	}
	return items, nil
}

func loadProjectItems(ctx context.Context) []uity.ProjectItem {
	candidates, err := projectCandidates(ctx)
	if err != nil {
		return []uity.ProjectItem{}
	}
	items := make([]uity.ProjectItem, 0, len(candidates))
	for _, candidate := range candidates {
		items = append(items, uity.ProjectItem{Name: filepath.Base(candidate.Path), Path: candidate.Path})
	}
	return items
}

func sessionNames(views []sessions.View) []string {
	names := make([]string, 0, len(views))
	for _, view := range views {
		names = append(names, view.Name)
	}
	return names
}
