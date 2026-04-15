//go:build e2e
// +build e2e

package handler_test

import (
	"context"
	"net/http"

	"github.com/heartgryphon/dsp/internal/auth"
)

// authMiddlewareImpl is a thin wrapper around auth.APIKeyMiddleware so test
// files can compose their own auth chain without re-importing the auth
// package in each file.
func authMiddlewareImpl(lookup func(ctx context.Context, key string) (int64, string, string, error), next http.Handler) http.Handler {
	return auth.APIKeyMiddleware(lookup)(next)
}
