package logctx

import (
	"context"

	"go.uber.org/zap"
)

type ctxKey struct{}

// todo: change with zerolog
func From(ctx context.Context) *zap.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*zap.Logger); ok {
		return l
	}
	return zap.NewNop()
}

func With(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, logger)
}

func Named(ctx context.Context, name string) context.Context {
	return With(ctx, From(ctx).Named(name))
}
