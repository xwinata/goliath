package logger

import "log/slog"

// Field is a single structured key/value pair attached to a log entry. Using an
// ordered slice of Fields (instead of a map) keeps field order deterministic
// and avoids a map allocation per call.
type Field struct {
	Key   string
	Value any
}

// F constructs a Field. It is the concise way to attach data to a log call:
//
//	logger.Info("processed", logger.F("user_id", 123), logger.F("op", "save"))
func F(key string, value any) Field {
	return Field{Key: key, Value: value}
}

func (f Field) attr() slog.Attr {
	return slog.Any(f.Key, f.Value)
}
