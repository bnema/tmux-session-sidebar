package ports

import "io"

type LogField struct {
	Key   string
	Value any
}

type LoggerPort interface {
	Debug(msg string, fields []LogField)
	Info(msg string, fields []LogField)
	Error(msg string, fields []LogField)
}

type SyncWriteCloser interface {
	io.WriteCloser
	Sync() error
}
