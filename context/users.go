package context

import (
	"context"

	"github.com/rahul4469/github-analyzer/internal/models"
)

type key string

const (
	userKey key = "user"
)

// Takes built-in context type and User type,
// binds the User object/data to ctx and return the data
// as ctx of context type to use later
func WithUser(ctx context.Context, user *models.User) context.Context {
	return context.WithValue(ctx, userKey, user)
}

func UserFromContext(ctx context.Context) *models.User {
	val := ctx.Value(userKey)
	user, ok := val.(*models.User)
	if !ok {
		return nil
	}
	return user
}
