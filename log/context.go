package log

import "context"

type loggerKey string // used type to store logger in context

func AddToContext(ctx context.Context, logger *Logger) context.Context {
	return context.WithValue(ctx, loggerKey("logger"), logger)
}

func GetFromContext(ctx context.Context) *Logger {
	if ctx == nil {
		return nil
	}
	if logger, ok := ctx.Value(loggerKey("logger")).(*Logger); ok {
		return logger
	}
	return nil
}
