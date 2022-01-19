package common

import (
	"context"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/bcmmacro/bridging-go/library/log"
)

type corrIDKey struct{}

func (corrIDKey) String() string {
	return "CorrID"
}

func CtxWithCorrID(ctx context.Context, corrID string) context.Context {
	return context.WithValue(ctx, corrIDKey{}, corrID)
}

func CorrIDCtx(ctx context.Context) (context.Context, string) {
	if v := ctx.Value(corrIDKey{}); v != nil {
		return ctx, v.(string)
	}
	corrID := uuid.New().String()
	return CtxWithCorrID(ctx, corrID), corrID
}

func CorrIDCtxLogger(ctx context.Context) (context.Context, *logrus.Entry) {
	ctx, corrID := CorrIDCtx(ctx)
	return log.WithField(ctx, corrIDKey{}.String(), corrID)
}
