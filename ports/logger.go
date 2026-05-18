package ports

type LogField struct {
	Key   string
	Value any
}

type LoggerPort interface {
	Debug(msg string, fields []LogField)
	Info(msg string, fields []LogField)
	Error(msg string, fields []LogField)
}
