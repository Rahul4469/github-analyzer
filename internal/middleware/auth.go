package middleware

import (
	"net/http"

	"github.com/rahul4469/github-analyzer/context"
	"github.com/rahul4469/github-analyzer/internal/models"
)

type AuthMiddleware struct {
	sessionService *models.SessionService
	cookieName     string
}

func NewAuthMiddleware(sessionService *models.SessionService, cookieName string) *AuthMiddleware {
	return &AuthMiddleware{
		sessionService: sessionService,
		cookieName:     cookieName,
	}
}

// SetUser middleware loads the authenticated user from the session cookie
// and stores it in the request context.
// This middleware should run on ALL routes. It does not block requests,
// it simply populates the user context if a valid session exists.
func (m *AuthMiddleware) SetUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get session cookie
		cookie, err := r.Cookie(m.cookieName)
		if err != nil {
			// No cookie - proceed without user (anonymous request)
			next.ServeHTTP(w, r)
			return
		}

		// Validate session and get user from session
		user, err := m.sessionService.User(r.Context(), cookie.Value)
		if err != nil {
			// Invalid or expired session - clear the cookie and proceed
			http.SetCookie(w, &http.Cookie{
				Name:     m.cookieName,
				Value:    "",
				Path:     "/",
				MaxAge:   -1, // Delete cookie
				HttpOnly: true,
			})
			next.ServeHTTP(w, r)
			return
		}

		// Store user in request context
		ctx := context.ContextSetUser(r.Context(), user)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

// RequireUser middleware ensures the request is authenticated.
// If no user is in context, redirects to the signin page.
func (m *AuthMiddleware) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := context.ContextGetUser(r.Context())
		if user == nil {
			// Store the original URL to redirect back after login
			// We'll use a query parameter for simplicity
			redirectURL := "/signin"
			if r.URL.Path != "/" {
				redirectURL = "/signin?redirect=" + r.URL.Path
			}
			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequireNoUser middleware ensures the request is NOT authenticated.
// Useful for login/signup pages that shouldn't be accessible when logged in.
func (m *AuthMiddleware) RequireNoUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := context.ContextGetUser(r.Context())
		if user != nil {
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequireQuota middleware checks if the user has remaining API quota.
// If quota is exceeded, shows an error page.
func (m *AuthMiddleware) RequireQuota(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := context.ContextGetUser(r.Context())
		if user == nil {
			http.Redirect(w, r, "/signin", http.StatusSeeOther)
			return
		}

		if user.RemainingQuota() <= 0 {
			http.Error(w, "API quota exceeded. Please contact support.", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// HELPER FUNCS --------------------------------------------

// CurrentUser is a helper function to get the current user from any handler.
// Returns nil if not authenticated.
func CurrentUser(r *http.Request) *models.User {
	return context.ContextGetUser(r.Context())
}

// MustCurrentUser is like CurrentUser but panics if no user is found.
// Only use this in handlers protected by RequireUser middleware.
func MustCurrentUser(r *http.Request) *models.User {
	user := context.ContextGetUser(r.Context())
	if user == nil {
		panic("MustCurrentUser called without RequireUser middleware")
	}
	return user
}
