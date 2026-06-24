package ports_test

import (
	"testing"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
	portmocks "github.com/bnema/tmux-session-sidebar/internal/ports/mocks"
)

func TestLoggerMockAcceptsStructuredFields(t *testing.T) {
	fields := []ports.LogField{{Key: "k", Value: "v"}}
	tests := []struct {
		name string
		call func(*portmocks.MockLoggerPort)
	}{
		{name: "debug", call: func(logger *portmocks.MockLoggerPort) {
			logger.EXPECT().Debug("m", fields).Once()
			logger.Debug("m", fields)
		}},
		{name: "info", call: func(logger *portmocks.MockLoggerPort) {
			logger.EXPECT().Info("m", fields).Once()
			logger.Info("m", fields)
		}},
		{name: "error", call: func(logger *portmocks.MockLoggerPort) {
			logger.EXPECT().Error("m", fields).Once()
			logger.Error("m", fields)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := portmocks.NewMockLoggerPort(t)
			tt.call(logger)
		})
	}
}
