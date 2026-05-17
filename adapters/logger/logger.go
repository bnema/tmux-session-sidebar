package logger

import (
	"fmt"
	"io"

	"github.com/bnema/tmux-session-sidebar/ports"
)

type Logger struct {
	Out io.Writer
}

func (l Logger) Debug(msg string, fields []ports.LogField) { l.log("debug", msg, fields) }
func (l Logger) Info(msg string, fields []ports.LogField)  { l.log("info", msg, fields) }
func (l Logger) Error(msg string, fields []ports.LogField) { l.log("error", msg, fields) }

func (l Logger) log(level string, msg string, fields []ports.LogField) {
	if l.Out == nil {
		return
	}
	_, _ = fmt.Fprintf(l.Out, "%s: %s", level, msg)
	for _, field := range fields {
		_, _ = fmt.Fprintf(l.Out, " %s=%v", field.Key, field.Value)
	}
	_, _ = fmt.Fprintln(l.Out)
}
