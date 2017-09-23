package compactor

import (
	"context"
	"time"

	"github.com/go-kit/kit/log"
)

// Middleware describes a service (as opposed to endpoint) middleware.
type Middleware func(Service) Service

func LoggingMiddleware(logger log.Logger) Middleware {
	return func(next Service) Service {
		return &loggingMiddleware{
			next:   next,
			logger: logger,
		}
	}
}

type loggingMiddleware struct {
	next   Service
	logger log.Logger
}

func (mw loggingMiddleware) Process(ctx context.Context, p Profile) (err error) {
	defer func(begin time.Time) {
		mw.logger.Log("method", "Process", "id", p.ID, "took", time.Since(begin), "err", err)
	}(time.Now())
	return mw.next.Process(ctx, p)
}