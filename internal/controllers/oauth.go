package controllers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/rahul4469/github-analyzer/internal/crypto"
	"github.com/rahul4469/github-analyzer/internal/middleware"
	"github.com/rahul4469/github-analyzer/internal/models"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

// OAuthController handles GitHub OAuth2 authentication.
type OAuthController struct {
	userService     *models.UserService
	sessionService  *models.SessionService
	encryptor       *crypto.Encryptor
	oauthConfig     *oauth2.Config
	cookieName      string
	cookieSecure    bool
	sessionDuration time.Duration
}

// OAuthConfig holds OAuth2 configuration.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
}

// NewOAuthController creates a new OAuthController.
func NewOAuthController(
	userService *models.UserService,
	sessionService *models.SessionService,
	encryptor *crypto.Encryptor,
	config OAuthConfig,
	cookieName string,
	cookieSecure bool,
	sessionDuration time.Duration,
) *OAuthController {
	// Create oauth2.Config using the library
	oauthConfig := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURL,
		Scopes:       config.Scopes,
		Endpoint:     github.Endpoint, // Pre-configured GitHub OAuth endpoints
	}

	return &OAuthController{
		userService:     userService,
		sessionService:  sessionService,
		encryptor:       encryptor,
		oauthConfig:     oauthConfig,
		cookieName:      cookieName,
		cookieSecure:    cookieSecure,
		sessionDuration: sessionDuration,
	}
}

// GitHubLogin initiates the GitHub OAuth2 flow.
// GET /auth/github/login
func (c *OAuthController) GitHubLogin(w http.ResponseWriter, r *http.Request) {
	// Generate state token to prevent CSRF
	state, err := generateState()
	if err != nil {
		log.Printf("Failed to generate state: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// Store state in cookie for validation on callback
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		Secure:   c.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	// Get authorization URL using oauth2 library (one-liner!)
	authURL := c.oauthConfig.AuthCodeURL(state)

	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// GitHubCallback handles the OAuth2 callback from GitHub.
// GET /auth/github/callback
func (c *OAuthController) GitHubCallback(w http.ResponseWriter, r *http.Request) {
	// Verify state to prevent CSRF
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil {
		log.Printf("Missing state cookie: %v", err)
		http.Redirect(w, r, "/signin?error=oauth_failed", http.StatusSeeOther)
		return
	}

	state := r.URL.Query().Get("state")
	if state == "" || state != stateCookie.Value {
		log.Printf("State mismatch: expected %s, got %s", stateCookie.Value, state)
		http.Redirect(w, r, "/signin?error=oauth_failed", http.StatusSeeOther)
		return
	}

	// Clear state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	// Check for error from GitHub
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		errDesc := r.URL.Query().Get("error_description")
		log.Printf("GitHub OAuth error: %s - %s", errParam, errDesc)
		http.Redirect(w, r, "/signin?error=github_denied", http.StatusSeeOther)
		return
	}

	// Get authorization code
	code := r.URL.Query().Get("code")
	if code == "" {
		log.Printf("Missing authorization code")
		http.Redirect(w, r, "/signin?error=oauth_failed", http.StatusSeeOther)
		return
	}

	// Exchange code for access token using oauth2 library (one-liner!)
	token, err := c.oauthConfig.Exchange(r.Context(), code)
	if err != nil {
		log.Printf("Failed to exchange code for token: %v", err)
		http.Redirect(w, r, "/signin?error=oauth_failed", http.StatusSeeOther)
		return
	}

	// Get GitHub user info
	githubUser, err := c.getGitHubUser(r.Context(), token.AccessToken)
	if err != nil {
		log.Printf("Failed to get GitHub user: %v", err)
		http.Redirect(w, r, "/signin?error=oauth_failed", http.StatusSeeOther)
		return
	}

	// Check if user is already logged in (connecting GitHub to existing account)
	currentUser := middleware.CurrentUser(r)
	if currentUser != nil {
		// Connect GitHub to existing account
		err = c.connectGitHubToUser(r.Context(), currentUser.ID, githubUser, token)
		if err != nil {
			log.Printf("Failed to connect GitHub: %v", err)
			http.Redirect(w, r, "/dashboard?error=github_connect_failed", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/dashboard?success=github_connected", http.StatusSeeOther)
		return
	}

	// Check if GitHub account is already linked to a user
	existingUser, err := c.userService.ByGitHubID(r.Context(), githubUser.ID)
	if err == nil && existingUser != nil {
		// Log in existing user
		sessionToken, _, err := c.sessionService.Create(r.Context(), existingUser.ID)
		if err != nil {
			log.Printf("Failed to create session: %v", err)
			http.Redirect(w, r, "/signin?error=session_failed", http.StatusSeeOther)
			return
		}
		c.setSessionCookie(w, sessionToken)
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// New user - redirect to complete registration
	// Store GitHub data temporarily in session for registration completion
	// For now, we'll require email/password signup first, then GitHub connection
	http.Redirect(w, r, "/signup?github=pending&username="+url.QueryEscape(githubUser.Login), http.StatusSeeOther)
}

// GitHubConnect connects GitHub to an authenticated user's account.
// GET /auth/github/connect (requires authentication)
func (c *OAuthController) GitHubConnect(w http.ResponseWriter, r *http.Request) {
	// This is the same as login, but for connecting to existing account
	c.GitHubLogin(w, r) // This initiates the OAuth2-github authorization
}

// GitHubDisconnect removes GitHub connection from user's account.
// POST /auth/github/disconnect (requires authentication)
func (c *OAuthController) GitHubDisconnect(w http.ResponseWriter, r *http.Request) {
	user := middleware.MustCurrentUser(r)

	err := c.userService.DisconnectGitHub(r.Context(), user.ID)
	if err != nil {
		log.Printf("Failed to disconnect GitHub: %v", err)
		http.Redirect(w, r, "/dashboard?error=disconnect_failed", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/dashboard?success=github_disconnected", http.StatusSeeOther)
}

// GitHubUser represents user data from GitHub API.
type GitHubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// getGitHubUser fetches the authenticated user's information from GitHub.
func (c *OAuthController) getGitHubUser(ctx context.Context, accessToken string) (*GitHubUser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "GitHub-Analyzer/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (%d): %s", resp.StatusCode, string(body))
	}

	var user GitHubUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to parse user response: %w", err)
	}

	// If email is empty, try to get primary email
	if user.Email == "" {
		user.Email, _ = c.getGitHubPrimaryEmail(ctx, accessToken)
	}

	return &user, nil
}

// getGitHubPrimaryEmail fetches the user's primary email from GitHub.
func (c *OAuthController) getGitHubPrimaryEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "GitHub-Analyzer/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", err
	}

	// Return primary verified email
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}

	// Fallback to first verified email
	for _, e := range emails {
		if e.Verified {
			return e.Email, nil
		}
	}

	return "", nil
}

// connectGitHubToUser links a GitHub account to an existing user.
func (c *OAuthController) connectGitHubToUser(ctx context.Context, userID int64, githubUser *GitHubUser, token *oauth2.Token) error {
	// Encrypt the access token
	encryptedToken, err := c.encryptor.Encrypt(token.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to encrypt token: %w", err)
	}

	// Handle token expiry (GitHub tokens typically don't expire, but the library supports it)
	var expiresAt *time.Time
	if !token.Expiry.IsZero() {
		expiresAt = &token.Expiry
	}

	data := models.GitHubOAuthData{
		GitHubID:       githubUser.ID,
		GitHubUsername: githubUser.Login,
		AccessToken:    token.AccessToken,
		ExpiresAt:      expiresAt,
	}

	return c.userService.ConnectGitHub(ctx, userID, data, encryptedToken)
}

// setSessionCookie sets the session cookie.
func (c *OAuthController) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     c.cookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(c.sessionDuration.Seconds()),
		HttpOnly: true,
		Secure:   c.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// generateState creates a random state string for CSRF protection.
func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
