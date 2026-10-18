package stdwrapper

import "context"

// ctxKey is an unexported context key type so request-ID values stored by this
// package cannot collide with keys from other packages.
type ctxKey struct{}

// requestIDKey is the context key under which the request ID is stored.
var requestIDKey = ctxKey{}

// bodyLimitKey is a distinct context key type under which the wrapper records
// that it has already capped the request body. A separate type (rather than a
// second ctxKey value) keeps the two keys unambiguous.
type bodyLimitKey struct{}

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

// withBodyLimit returns a copy of ctx recording that the request body has
// already been capped to limit bytes (limit <= 0 means the wrapper left the
// body uncapped). It lets downstream body readers, such as httpkit.DecodeJSON,
// avoid imposing a second, redundant cap.
func withBodyLimit(ctx context.Context, limit int64) context.Context {
	return context.WithValue(ctx, bodyLimitKey{}, limit)
}

// BodyLimitFromContext returns the request-body byte cap the wrapper already
// applied, and reports whether a wrapper handled this request at all. A limit
// of 0 or less means the wrapper deliberately left the body uncapped. Body
// readers can use ok to decide whether they still need to apply their own cap:
// when ok is true the body is already bounded (or intentionally unbounded) and
// a second cap would be redundant.
func BodyLimitFromContext(ctx context.Context) (limit int64, ok bool) {
	limit, ok = ctx.Value(bodyLimitKey{}).(int64)
	return limit, ok
}
