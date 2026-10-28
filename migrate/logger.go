package migrate

import (
	"io"
	"log"
	"os"
)

// Logger receives migration and SQL log messages.
type Logger interface {
	Printf(format string, args ...any)
}

type discardLogger struct{}

func (discardLogger) Printf(format string, args ...any) {}

func defaultLogger() Logger {
	return log.New(os.Stdout, "", 0)
}

func writerLogger(w io.Writer) Logger {
	return log.New(w, "", 0)
}
