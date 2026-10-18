package stdwrapper

import "context"

// ctxKey is an unexported context key type so request-ID values stored by this
// package cannot collide with keys from other packages.
type ctxKey struct{}

// requestIDKey is the context key under which the request ID is stored.
var requestIDKey = ctxKey{}

// withRequestID returns a copy of ctx carrying id.
func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFromContext returns the request ID stored on ctx by the wrapper, and
// reports whether one was present. Handlers invoked through Wrap can use it to
// include the request ID in their own logs or downstream calls.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(requestIDKey).(string)
	return id, ok
}
