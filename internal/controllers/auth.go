package controllers

import (
	"net/http"
	"strings"

	"github.com/rahul4469/github-analyzer/context"
	"github.com/rahul4469/github-analyzer/internal/models"
)

// AuthController handles signup/signin flows
type AuthController struct {
	UserService    *models.UserService
	SessionService *models.SessionService
	Template       Template
}

func NewAuthController(
	us *models.UserService,
	ss *models.SessionService,
	tpl Template,
) *AuthController {
	return &AuthController{
		UserService:    us,
		SessionService: ss,
		Template:       tpl,
	}
}

// Display signup form
func (ac *AuthController) GetSignUp(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"PageTitle": "Sign Up",
	}
	ac.Template.Execute(w, r, data)
}

// Create new user - post signup
func (ac *AuthController) PostSignUp(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ac.Template.Execute(w, r, map[string]interface{}{"Error": "Failed to parse form"})
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := strings.TrimSpace(r.FormValue("password"))
	confirmPassword := strings.TrimSpace(r.FormValue("confirm_password"))

	if email == "" {
		ac.Template.Execute(w, r, map[string]interface{}{"Error": "Email is required", "Email": email})
		return
	}
	if password == "" {
		ac.Template.Execute(w, r, map[string]interface{}{"Error": "Password is required", "Email": email})
		return
	}
	if password != confirmPassword {
		ac.Template.Execute(w, r, map[string]interface{}{"Error": "Passwords do not match", "Email": email})
		return
	}

	user, err := ac.UserService.Create(email, password)
	if err != nil {
		ac.Template.Execute(w, r, map[string]interface{}{"Error": err.Error(), "Email": email})
		return
	}

	session, err := ac.SessionService.Create(user.ID)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	ac.setCookie(w, CookieSession, session.Token)
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func (ac *AuthController) GetSignIn(w http.ResponseWriter, r *http.Request) {
	ac.Template.Execute(w, r, map[string]interface{}{"PageTitle": "Sign In"})
}

func (ac *AuthController) PostSignIn(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ac.Template.Execute(w, r, map[string]interface{}{"Error": "Failed to parse form"})
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := strings.TrimSpace(r.FormValue("password"))

	if email == "" {
		ac.Template.Execute(w, r, map[string]interface{}{"Error": "Email is required"})
		return
	}
	if password == "" {
		ac.Template.Execute(w, r, map[string]interface{}{"Error": "Password is required"})
		return
	}

	user, err := ac.UserService.Authenticate(email, password)
	if err != nil {
		ac.Template.Execute(w, r, map[string]interface{}{"Error": "Invalid email or password"})
		return
	}

	session, err := ac.SessionService.Create(user.ID)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	ac.setCookie(w, CookieSession, session.Token)
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

// Delete session/ process sign out
func (ac *AuthController) PostLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(CookieSession)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	_ = ac.SessionService.Delete(cookie.Value)
	ac.deleteCookie(w, CookieSession)
	http.Redirect(w, r, "/", http.StatusFound)
}

// Middlewares ------------------------------------------
// Uses session data from DB to fetch user data

type UserMiddleware struct {
	SessionService *models.SessionService
}

func (umw UserMiddleware) SetUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCookie, err := r.Cookie("session")
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		user, err := umw.SessionService.User(tokenCookie.Value)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		ctx := r.Context()
		ctx = context.WithUser(ctx, user)
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}

func (umw UserMiddleware) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := context.UserFromContext(r.Context())
		if user == nil {
			http.Redirect(w, r, "/signin", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}
