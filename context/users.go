package context

import (
	"context"

	"github.com/rahul4469/github-analyzer/internal/models"
)

type contextkey string

const (
	userKey contextkey = "user"
)

// Takes built-in context type and User type,
// binds the User object/data to ctx and return the data
// as ctx of context type to use later
func ContextSetUser(ctx context.Context, user *models.User) context.Context {
	return context.WithValue(ctx, userKey, user)
}

// ContextGetUser retrieves the authenticated user from request context.
// Returns nil if no user is set (unauthenticated request).
func ContextGetUser(ctx context.Context) *models.User {
	val := ctx.Value(userKey)
	user, ok := val.(*models.User)
	if !ok {
		return nil
	}
	return user
}
