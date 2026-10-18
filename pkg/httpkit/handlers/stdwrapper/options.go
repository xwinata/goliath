package stdwrapper

// Config customizes a wrapper created by Wrap. The zero value is valid: empty
// fields fall back to the package defaults (defaultSuccessMessage,
// defaultErrorMessage, HeaderRequestID, HeaderErrorCode), so Config{} yields a
// wrapper with standard behavior.
type Config struct {
	// SuccessMessage is the envelope message sent on successful responses.
	// Empty selects defaultSuccessMessage.
	SuccessMessage string

	// ErrorMessage is the fallback envelope message sent on failures when the
	// error carries no client-facing message of its own. Empty selects
	// defaultErrorMessage.
	ErrorMessage string

	// RequestIDHeader is the header read for the incoming request ID and echoed
	// back on the response. Empty selects HeaderRequestID.
	RequestIDHeader string

	// ErrorCodeHeader is the response header carrying the application error
	// code. Empty selects HeaderErrorCode.
	ErrorCodeHeader string
}

// resolvedConfig holds the effective settings after defaults are applied. It is
// unexported so the resolved form never leaks into the public API.
type resolvedConfig struct {
	successMessage  string
	errorMessage    string
	requestIDHeader string
	errorCodeHeader string
}

// resolved returns c with empty fields replaced by their defaults. It performs
// the defensive validation the constructor relies on, so the rest of the
// package can assume every field is set.
func (c Config) resolved() resolvedConfig {
	r := resolvedConfig{
		successMessage:  c.SuccessMessage,
		errorMessage:    c.ErrorMessage,
		requestIDHeader: c.RequestIDHeader,
		errorCodeHeader: c.ErrorCodeHeader,
	}
	if r.successMessage == "" {
		r.successMessage = defaultSuccessMessage
	}
	if r.errorMessage == "" {
		r.errorMessage = defaultErrorMessage
	}
	if r.requestIDHeader == "" {
		r.requestIDHeader = HeaderRequestID
	}
	if r.errorCodeHeader == "" {
		r.errorCodeHeader = HeaderErrorCode
	}
	return r
}
