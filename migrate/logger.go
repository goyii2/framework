package migrate

import (
	"log"
	"os"
)

// Logger receives migration and SQL log messages.
type Logger interface {
	Printf(format string, args ...any)
}

func defaultLogger() Logger {
	return log.New(os.Stdout, "", 0)
}
