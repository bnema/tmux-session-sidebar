package logger

import (
	"fmt"
	"io"
	"strings"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

type Logger struct {
	Out io.Writer
}

type syncWriter interface {
	Sync() error
}

func (l Logger) Debug(msg string, fields []ports.LogField) { l.log("debug", msg, fields) }
func (l Logger) Info(msg string, fields []ports.LogField)  { l.log("info", msg, fields) }
func (l Logger) Error(msg string, fields []ports.LogField) { l.log("error", msg, fields) }

func (l Logger) log(level string, msg string, fields []ports.LogField) {
	if l.Out == nil {
		return
	}
	var line strings.Builder
	_, _ = fmt.Fprintf(&line, "%s: %s", level, msg)
	for _, field := range fields {
		_, _ = fmt.Fprintf(&line, " %s=%v", field.Key, field.Value)
	}
	line.WriteByte('\n')
	_, _ = io.WriteString(l.Out, line.String())
	if syncer, ok := l.Out.(syncWriter); ok {
		_ = syncer.Sync()
	}
}
