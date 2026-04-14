package handler

import (
	"context"

	"github.com/heartgryphon/dsp/internal/auth"
)

// ctxWithAdvertiser is the single entry point tests use to build a request
// context that looks like it came through APIKeyMiddleware. Kept in one
// place so every security test agrees on the shape of authenticated state.
func ctxWithAdvertiser(parent context.Context, id int64) context.Context {
	return auth.WithAdvertiserForTest(parent, id)
}
