package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID              int64     `json:"id"`
	Email           string    `json:"email"`
	PasswordHash    string    `json:"-"`
	GitHubTokenHash *string   `json:"-"`
	APIQuotaUsed    int       `json:"api_quota_used"`
	APIQuotaLimit   int       `json:"api_quota_limit"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type UserService struct {
	pool       *pgxpool.Pool
	bcryptCost int
}

// NewUserService creates a new UserService.
// bcryptCost should be 12-14 for production (higher = slower but more secure).
func NewUserService(pool *pgxpool.Pool, bcryptCost int) *UserService {
	return &UserService{
		pool:       pool,
		bcryptCost: bcryptCost,
	}
}

func (s *UserService) Create(ctx context.Context, email, password string, defaultQuota int) (*User, error) {
	// Validate inputs
	email = strings.TrimSpace(strings.ToLower(email))
	if !isValidEmail(email) {
		return nil, ErrInvalidEmail
	}

	if len(password) < 8 {
		return nil, ErrPasswordTooShort
	}

	// Hash password with bcrypt
	// bcrypt automatically generates a salt and includes it in the hash
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Insert user into database
	user := &User{}
	query := `
		INSERT INTO users (email, password_hash, api_quota_limit)
		VALUES ($1, $2, $3)
		RETURNING id, email, password_hash, github_token_hash, api_quota_used, api_quota_limit, created_at, updated_at
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	err = s.pool.QueryRow(ctx, query, email, string(hashedPassword), defaultQuota).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.GitHubTokenHash,
		&user.APIQuotaUsed,
		&user.APIQuotaLimit,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		// Check for unique constraint violation (duplicate email)
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, ErrEmailAlreadyExists
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

func (s *UserService) Authenticate(ctx context.Context, email, password string) (*User, error) {
	email = strings.TrimSpace(strings.ToLower(email))

	user, err := s.ByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// Don't reveal that email doesn't exist
			// Also do a dummy bcrypt compare to prevent timing attacks
			_ = bcrypt.CompareHashAndPassword([]byte("$2a$12$dummy.hash.to.prevent.timing.attacks"), []byte(password))
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	// Compare password with stored hash
	// bcrypt.CompareHashAndPassword is constant-time
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	return user, nil
}

// ByID retrieves a user by their ID.
// Returns ErrUserNotFound if no user exists with that ID.
func (s *UserService) ByID(ctx context.Context, id int64) (*User, error) {
	query := `
		SELECT id, email, password_hash, github_token_hash, api_quota_used, api_quota_limit, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	user := &User{}
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.GitHubTokenHash,
		&user.APIQuotaUsed,
		&user.APIQuotaLimit,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by ID: %w", err)
	}

	return user, nil
}

// ByEmail retrieves a user by their email address.
// Returns ErrUserNotFound if no user exists with that email.
func (s *UserService) ByEmail(ctx context.Context, email string) (*User, error) {
	email = strings.TrimSpace(strings.ToLower(email))

	query := `
		SELECT id, email, password_hash, github_token_hash, api_quota_used, api_quota_limit, created_at, updated_at
		FROM users
		WHERE email = $1
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	user := &User{}
	err := s.pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.GitHubTokenHash,
		&user.APIQuotaUsed,
		&user.APIQuotaLimit,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return user, nil
}

// SetGitHubToken stores a hashed GitHub personal access token for the user.
// The token is hashed with SHA256 before storage.
func (s *UserService) SetGitHubToken(ctx context.Context, userID int64, token string) error {
	tokenHash := hashToken(token)

	query := `
		UPDATE users
		SET github_token_hash = $1, updated_at = NOW()
		WHERE id = $2
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	result, err := s.pool.Exec(ctx, query, tokenHash, userID)
	if err != nil {
		return fmt.Errorf("failed to set GitHub token: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// ClearGitHubToken removes the stored GitHub token for a user.
func (s *UserService) ClearGitHubToken(ctx context.Context, userID int64) error {
	query := `
		UPDATE users
		SET github_token_hash = NULL, updated_at = NOW()
		WHERE id = $1
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	_, err := s.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to clear GitHub token: %w", err)
	}

	return nil
}

// UpdateAPIQuota adds the specified number of tokens to the user's usage.
// Returns an error if this would exceed their quota limit.
func (s *UserService) UpdateAPIQuota(ctx context.Context, userID int64, tokensUsed int) error {
	// First check if user has enough quota
	user, err := s.ByID(ctx, userID)
	if err != nil {
		return err
	}

	if user.APIQuotaUsed+tokensUsed > user.APIQuotaLimit {
		return fmt.Errorf("quota exceeded: would use %d tokens but limit is %d (currently used: %d)",
			user.APIQuotaUsed+tokensUsed, user.APIQuotaLimit, user.APIQuotaUsed)
	}

	query := `
		UPDATE users
		SET api_quota_used = api_quota_used + $1, updated_at = NOW()
		WHERE id = $2
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	_, err = s.pool.Exec(ctx, query, tokensUsed, userID)
	if err != nil {
		return fmt.Errorf("failed to update API quota: %w", err)
	}

	return nil
}

// ResetAPIQuota resets the user's API quota usage to zero.
// Typically called at the start of a billing period.
func (s *UserService) ResetAPIQuota(ctx context.Context, userID int64) error {
	query := `
		UPDATE users
		SET api_quota_used = 0, updated_at = NOW()
		WHERE id = $1
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	_, err := s.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to reset API quota: %w", err)
	}

	return nil
}

// HELPRE FUNCTIONS ----------------------

// HasGitHubToken returns true if the user has stored a GitHub token.
func (u *User) HasGitHubToken() bool {
	return u.GitHubTokenHash != nil && *u.GitHubTokenHash != ""
}

// RemainingQuota returns how many API tokens the user can still use.
func (u *User) RemainingQuota() int {
	remaining := u.APIQuotaLimit - u.APIQuotaUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

// QuotaPercentUsed returns the percentage of quota consumed (0-100).
func (u *User) QuotaPercentUsed() int {
	if u.APIQuotaLimit == 0 {
		return 100
	}
	return (u.APIQuotaUsed * 100) / u.APIQuotaLimit
}

// hashToken creates a SHA256 hash of a token.
// Used for GitHub tokens and session tokens.
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func isValidEmail(email string) bool {
	// Basic validation: must contain @ and have parts before and after
	if len(email) < 3 || len(email) > 255 {
		return false
	}

	atIndex := strings.LastIndex(email, "@")
	if atIndex < 1 || atIndex >= len(email)-1 {
		return false
	}

	// Check for dot in domain part
	domain := email[atIndex+1:]
	if !strings.Contains(domain, ".") {
		return false
	}

	// No spaces allowed
	if strings.Contains(email, " ") {
		return false
	}

	return true
}
