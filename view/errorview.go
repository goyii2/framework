package view

// ErrorData is the data shape the unified error handler passes to the
// reserved error views when an HTML response is due. Register a factory
// under "error/<status>" (e.g. "error/404") to override one status code, or
// under "error" as the fallback for every HTML error; without a
// registration the handler falls back to its built-in minimal page.
type ErrorData struct {
	Status  int
	Name    string // HTTP status text, e.g. "Not Found"
	Message string // client-presentable explanation
}
