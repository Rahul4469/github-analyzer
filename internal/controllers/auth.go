package controllers

import (
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/csrf"
	"github.com/rahul4469/github-analyzer/internal/models"
	"github.com/rahul4469/github-analyzer/internal/views"
)

// AuthController handles authentication-related routes.
type AuthController struct {
	userService     *models.UserService
	sessionService  *models.SessionService
	templates       AuthTemplates
	cookieName      string
	cookieSecure    bool
	sessionDuration time.Duration
	defaultQuota    int
}

// AuthTemplates holds the templates for auth pages.
type AuthTemplates struct {
	SignUp *views.Template
	SignIn *views.Template
}

// NewAuthController creates a new AuthController.
func NewAuthController(
	userService *models.UserService,
	sessionService *models.SessionService,
	templates AuthTemplates,
	cookieName string,
	cookieSecure bool,
	sessionDuration time.Duration,
	defaultQuota int,
) *AuthController {
	return &AuthController{
		userService:     userService,
		sessionService:  sessionService,
		templates:       templates,
		cookieName:      cookieName,
		cookieSecure:    cookieSecure,
		sessionDuration: sessionDuration,
		defaultQuota:    defaultQuota,
	}
}

// SignUpData holds data for the signup template.
type SignUpData struct {
	Email string
}

// GetSignUp renders the signup form.
func (c *AuthController) GetSignUp(w http.ResponseWriter, r *http.Request) {
	data := &views.TemplateData{
		Title:     "Sign Up",
		CSRFToken: csrf.Token(r),
		Data:      SignUpData{},
	}
	c.templates.SignUp.ExecuteHTTP(w, r, data)
}

// PostSignUp handles the signup form submission.
func (c *AuthController) PostSignUp(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		c.renderSignUpError(w, r, "", "Invalid form data")
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	// Validate password confirmation
	if password != confirmPassword {
		c.renderSignUpError(w, r, email, "Passwords do not match")
		return
	}

	// Create user
	user, err := c.userService.Create(r.Context(), email, password, c.defaultQuota)
	if err != nil {
		var errMsg string
		switch {
		case errors.Is(err, models.ErrEmailAlreadyExists):
			errMsg = "An account with this email already exists"
		case errors.Is(err, models.ErrInvalidEmail):
			errMsg = "Please enter a valid email address"
		case errors.Is(err, models.ErrPasswordTooShort):
			errMsg = "Password must be at least 8 characters"
		default:
			errMsg = "Failed to create account. Please try again."
		}
		c.renderSignUpError(w, r, email, errMsg)
		return
	}

	// Create session and login automatically
	token, _, err := c.sessionService.Create(r.Context(), user.ID)
	if err != nil {
		// User created but session failed - redirect to login
		http.Redirect(w, r, "/signin?msg=account_created", http.StatusSeeOther)
		return
	}

	// Set session cookie
	c.setSessionCookie(w, token)

	// Redirect to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// renderSignUpError renders the signup page with an error message.
func (c *AuthController) renderSignUpError(w http.ResponseWriter, r *http.Request, email, errMsg string) {
	data := &views.TemplateData{
		Title:     "Sign Up",
		CSRFToken: csrf.Token(r),
		Error:     errMsg,
		Data:      SignUpData{Email: email},
	}
	c.templates.SignUp.ExecuteHTTPWithStatus(w, r, http.StatusUnprocessableEntity, data)
}

// SignInData holds data for the signin template.
type SignInData struct {
	Email    string
	Redirect string
}

// GetSignIn renders the signin form.
func (c *AuthController) GetSignIn(w http.ResponseWriter, r *http.Request) {
	// Check for success message from signup
	var success string
	if r.URL.Query().Get("msg") == "account_created" {
		success = "Account created successfully! Please sign in."
	}

	data := &views.TemplateData{
		Title:     "Sign In",
		CSRFToken: csrf.Token(r),
		Success:   success,
		Data: SignInData{
			Redirect: r.URL.Query().Get("redirect"),
		},
	}
	c.templates.SignIn.ExecuteHTTP(w, r, data)
}

// PostSignIn handles the signin form submission.
func (c *AuthController) PostSignIn(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		c.renderSignInError(w, r, "", "", "Invalid form data")
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")
	redirect := r.FormValue("redirect")

	// Authenticate user
	user, err := c.userService.Authenticate(r.Context(), email, password)
	if err != nil {
		if errors.Is(err, models.ErrInvalidCredentials) {
			c.renderSignInError(w, r, email, redirect, "Invalid email or password")
			return
		}
		c.renderSignInError(w, r, email, redirect, "An error occurred. Please try again.")
		return
	}

	// Create session
	token, _, err := c.sessionService.Create(r.Context(), user.ID)
	if err != nil {
		c.renderSignInError(w, r, email, redirect, "Failed to create session. Please try again.")
		return
	}

	// Set session cookie
	c.setSessionCookie(w, token)

	// Redirect to original destination or dashboard
	if redirect != "" && isValidRedirect(redirect) {
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// renderSignInError renders the signin page with an error message.
func (c *AuthController) renderSignInError(w http.ResponseWriter, r *http.Request, email, redirect, errMsg string) {
	data := &views.TemplateData{
		Title:     "Sign In",
		CSRFToken: csrf.Token(r),
		Error:     errMsg,
		Data: SignInData{
			Email:    email,
			Redirect: redirect,
		},
	}
	c.templates.SignIn.ExecuteHTTPWithStatus(w, r, http.StatusUnprocessableEntity, data)
}

// PostLogout handles user logout.
func (c *AuthController) PostLogout(w http.ResponseWriter, r *http.Request) {
	// Get session cookie
	cookie, err := r.Cookie(c.cookieName)
	if err == nil && cookie.Value != "" {
		// Delete session from database
		_ = c.sessionService.Delete(r.Context(), cookie.Value)
	}

	// Clear the cookie
	http.SetCookie(w, &http.Cookie{
		Name:     c.cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   c.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	// Redirect to home
	http.Redirect(w, r, "/?msg=logged_out", http.StatusSeeOther)
}

// setSessionCookie sets the session cookie with secure settings.
func (c *AuthController) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     c.cookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(c.sessionDuration.Seconds()),
		HttpOnly: true,                 // Not accessible via JavaScript
		Secure:   c.cookieSecure,       // HTTPS only in production
		SameSite: http.SameSiteLaxMode, // CSRF protection
	})
}

// isValidRedirect checks if a redirect URL is safe (internal only).
func isValidRedirect(redirect string) bool {
	// Only allow internal redirects (starting with /)
	// Prevent open redirect vulnerabilities
	if redirect == "" {
		return false
	}
	if redirect[0] != '/' {
		return false
	}
	// Don't allow protocol-relative URLs
	if len(redirect) > 1 && redirect[1] == '/' {
		return false
	}
	return true
}
