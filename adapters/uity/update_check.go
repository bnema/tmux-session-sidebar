package uity

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	coreversion "github.com/bnema/tmux-session-sidebar/core/version"
)

const updateCheckInterval = 5 * time.Minute

type updateAvailableMsg struct {
	available bool
}

type updateCheckTickMsg struct{}

type updateCheckTickFunc func(time.Duration) tea.Cmd

type updateCheckState struct {
	version              string
	checkUpdateAvailable func(currentVersion string) (bool, error)
	available            bool
	pending              bool
	interval             time.Duration
	tick                 updateCheckTickFunc
}

func newUpdateCheckState(version string, check func(currentVersion string) (bool, error)) updateCheckState {
	state := updateCheckState{
		version:              version,
		checkUpdateAvailable: check,
		available:            displayVersion(version) == "dev",
		interval:             updateCheckInterval,
		tick:                 defaultUpdateCheckTick,
	}
	// Bubble Tea's Init method returns only a command, so state changes made while
	// building the startup command are not persisted by the program. Mark the
	// initial check as pending at construction time instead, before Init runs.
	state.pending = state.shouldCheck()
	return state
}

func defaultUpdateCheckTick(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return updateCheckTickMsg{}
	})
}

func (s updateCheckState) initCmd() tea.Cmd {
	if !s.shouldCheck() {
		return nil
	}
	return batchCommands(s.checkCmd(), s.tickCmd())
}

func (s updateCheckState) handleTick() (updateCheckState, tea.Cmd) {
	if !s.shouldCheck() {
		return s, nil
	}
	if s.pending {
		return s, s.tickCmd()
	}
	var checkCmd tea.Cmd
	s, checkCmd = s.startCheckCmd()
	return s, batchCommands(checkCmd, s.tickCmd())
}

func (s updateCheckState) handleResult(msg updateAvailableMsg) updateCheckState {
	s.pending = false
	if msg.available {
		s.available = true
	}
	return s
}

func (s updateCheckState) startCheckCmd() (updateCheckState, tea.Cmd) {
	if !s.shouldCheck() || s.pending {
		return s, nil
	}
	s.pending = true
	return s, s.checkCmd()
}

func (s updateCheckState) checkCmd() tea.Cmd {
	return func() tea.Msg {
		available, err := s.checkUpdateAvailable(s.version)
		if err != nil {
			return updateAvailableMsg{}
		}
		return updateAvailableMsg{available: available}
	}
}

func (s updateCheckState) tickCmd() tea.Cmd {
	if !s.shouldCheck() || s.tick == nil {
		return nil
	}
	interval := s.interval
	if interval <= 0 {
		interval = updateCheckInterval
	}
	return s.tick(interval)
}

func (s updateCheckState) shouldCheck() bool {
	return !s.available && s.checkUpdateAvailable != nil && coreversion.CheckableReleaseVersion(strings.TrimSpace(s.version))
}
