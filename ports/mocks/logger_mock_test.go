package mocks

import (
	"testing"

	"github.com/bnema/tmux-session-sidebar/ports"
)

func TestLoggerMockAcceptsStructuredFields(t *testing.T) {
	fields := []ports.LogField{{Key: "k", Value: "v"}}
	tests := []struct {
		name string
		call func(*MockLoggerPort)
	}{
		{name: "debug", call: func(logger *MockLoggerPort) { logger.EXPECT().Debug("m", fields).Once(); logger.Debug("m", fields) }},
		{name: "info", call: func(logger *MockLoggerPort) { logger.EXPECT().Info("m", fields).Once(); logger.Info("m", fields) }},
		{name: "error", call: func(logger *MockLoggerPort) { logger.EXPECT().Error("m", fields).Once(); logger.Error("m", fields) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewMockLoggerPort(t)
			tt.call(logger)
		})
	}
}
