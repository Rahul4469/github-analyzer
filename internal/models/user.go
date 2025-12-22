package models

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           int            `json:"id"`
	Username     string         `json:"username"`
	Email        string         `json:"email"`
	PasswordHash string         `json:"-"`
	GitHubToken  sql.NullString `json:"-"`
	// GitHub Info
	GitHubUsername sql.NullString
	AvatarURL      sql.NullString
	APIQuotaUsed   int        `json:"api_quota_used"`
	APIQuotaLimit  int        `json:"api_quota_limit"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastLogin      *time.Time `json:"last_login,omitempty"`
}

type UserService struct {
	DB *sql.DB
}

func NewUserService(db *sql.DB) *UserService {
	return &UserService{DB: db}
}

// Create user from Auth, Email/password signup
func (us *UserService) Create(email, password string) (*User, error) {
	// Validate input
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}
	if password == "" {
		return nil, fmt.Errorf("password is required")
	}
	if len(password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}

	// Hash password using bcrypt
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user object
	user := &User{
		Email:        email,
		Username:     email, // Default username to email
		PasswordHash: string(hashedBytes),
	}

	// Insert into database
	query := `
		INSERT INTO users (email, username, password_hash, api_quota_used, api_quota_limit, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		RETURNING id, created_at, updated_at
	`

	err = us.DB.QueryRow(
		query,
		user.Email,
		user.Username,
		user.PasswordHash,
		0,    // api_quota_used
		1000, // api_quota_limit (default)
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		// Check for duplicate email
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("email already registered")
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// Create user from GitHub API response
func (us *UserService) CreateFromGithub(ctx context.Context, user *User) (*User, error) {
	// Validate input
	if user.Username == "" || user.Email == "" {
		return nil, fmt.Errorf("username, email are required")
	}
	if !user.GitHubToken.Valid || user.GitHubToken.String == "" {
		return nil, fmt.Errorf("github_token is required")
	}
	// Normalize email
	user.Email = strings.ToLower(strings.TrimSpace(user.Email))

	// Insert user
	err := us.DB.QueryRowContext(
		ctx,
		`INSERT INTO users (
			username, email, password_hash, github_token,
			github_username, avatar_url, api_quota_used, api_quota_limit,
			created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING id, created_at, updated_at`,
		user.Username, user.Email, "", user.GitHubToken.String,
		user.GitHubUsername.String,
		user.AvatarURL.String, 0, 1000, // Default quota
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("user already exists")
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// For both Auth methods, Retrieve user from database by ID
func (us *UserService) ByID(id int) (*User, error) {
	query := `
		SELECT id, email, username, password_hash, github_token,
		       github_username, avatar_url, api_quota_used, api_quota_limit,
		       created_at, updated_at
		FROM users
		WHERE id = $1
	`

	user := &User{}
	err := us.DB.QueryRow(query, id).Scan(
		&user.ID, &user.Email, &user.Username, &user.PasswordHash,
		&user.GitHubToken, &user.GitHubUsername, &user.AvatarURL,
		&user.APIQuotaUsed, &user.APIQuotaLimit,
		&user.CreatedAt, &user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	return user, nil
}

// GetByID retrieves a user by their ID
func (us *UserService) GetByID(ctx context.Context, id int) (*User, error) {
	var user User

	err := us.DB.QueryRowContext(
		ctx,
		`SELECT id, username, email, password_hash, github_token,
		        api_quota_used, api_quota_limit, created_at, updated_at, last_login
		 FROM users WHERE id = $1`,
		id,
	).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash,
		&user.GitHubToken, &user.APIQuotaUsed, &user.APIQuotaLimit,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLogin,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to fetch user: %w", err)
	}

	return &user, nil
}

// GetByUsername retrieves a user by their username
func (us *UserService) GetByGitHubUsername(ctx context.Context, username string) (*User, error) {
	query := `
		SELECT id, email, username, password_hash, github_token,
		       github_username, avatar_url, api_quota_used, api_quota_limit,
		       created_at, updated_at
		FROM users
		WHERE github_username = $1
	`

	user := &User{}
	err := us.DB.QueryRowContext(ctx, query, username).Scan(
		&user.ID, &user.Email, &user.Username, &user.PasswordHash,
		&user.GitHubToken, &user.GitHubUsername, &user.AvatarURL,
		&user.APIQuotaUsed, &user.APIQuotaLimit,
		&user.CreatedAt, &user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Not found is not an error for OAuth
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	return user, nil
}

// Email/password login
func (us *UserService) Authenticate(email, password string) (*User, error) {
	// Normalize email
	email = strings.TrimSpace(strings.ToLower(email))

	// Get user from database
	query := `
		SELECT id, email, username, password_hash, api_quota_used, api_quota_limit, created_at, updated_at
		FROM users
		WHERE email = $1
	`

	user := &User{}
	err := us.DB.QueryRow(query, email).Scan(
		&user.ID, &user.Email, &user.Username, &user.PasswordHash,
		&user.APIQuotaUsed, &user.APIQuotaLimit,
		&user.CreatedAt, &user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid email or password")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	// Verify password
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	return user, nil
}

// GetByEmail retrieves a user by their email
func (us *UserService) GetByEmail(ctx context.Context, email string) (*User, error) {
	var user User

	err := us.DB.QueryRowContext(
		ctx,
		`SELECT id, username, email, password_hash, github_token,
		        api_quota_used, api_quota_limit, created_at, updated_at, last_login
		 FROM users WHERE email = $1`,
		email,
	).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash,
		&user.GitHubToken, &user.APIQuotaUsed, &user.APIQuotaLimit,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLogin,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to fetch user: %w", err)
	}

	return &user, nil
}

// UpdateGitHubToken updates a user's GitHub token
func (us *UserService) UpdateGitHubToken(id int, token string) error {
	query := `
		UPDATE users
		SET github_token = $1, updated_at = NOW()
		WHERE id = $2
	`

	result, err := us.DB.Exec(query, token, id)
	if err != nil {
		return fmt.Errorf("failed to update token: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// UpdateLastLogin updates user's last login time
func (us *UserService) UpdateLastLogin(ctx context.Context, userID int) error {
	_, err := us.DB.ExecContext(
		ctx,
		`UPDATE users SET last_login = NOW()
		 WHERE id = $1`,
		userID,
	)

	return err
}

// Delete deletes a user from the database
func (us *UserService) Delete(ctx context.Context, userID int) error {
	result, err := us.DB.ExecContext(
		ctx,
		`DELETE FROM users WHERE id = $1`,
		userID,
	)

	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// UpdateQuota updates user's API quota usage
func (us *UserService) UpdateQuota(ctx context.Context, userID int, used int) error {
	result, err := us.DB.ExecContext(
		ctx,
		`UPDATE users SET api_quota_used = $1, updated_at = NOW()
		 WHERE id = $2`,
		used, userID,
	)

	if err != nil {
		return fmt.Errorf("failed to update quota: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// CheckQuotaAvailable checks if user has API quota available
func (us *UserService) CheckQuotaAvailable(ctx context.Context, userID int) (bool, error) {
	var used, limit int

	err := us.DB.QueryRowContext(
		ctx,
		`SELECT api_quota_used, api_quota_limit FROM users WHERE id = $1`,
		userID,
	).Scan(&used, &limit)

	if err != nil {
		return false, fmt.Errorf("failed to check quota: %w", err)
	}

	return used < limit, nil
}
