package models

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type User struct {
	ID            int        `json:"id"`
	Username      string     `json:"username"`
	Email         string     `json:"email"`
	PasswordHash  string     `json:"-"`
	GitHubToken   string     `json:"-"`
	APIQuotaUsed  int        `json:"api_quota_used"`
	APIQuotaLimit int        `json:"api_quota_limit"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	LastLogin     *time.Time `json:"last_login,omitempty"`
}

type UserService struct {
	DB *sql.DB
}

// Create creates a new user in the database
func (us *UserService) Create(ctx context.Context, user *User) (*User, error) {
	// Validate input
	if user.Username == "" || user.Email == "" || user.GitHubToken == "" {
		return nil, fmt.Errorf("username, email, and github_token are required")
	}

	// Insert user
	err := us.DB.QueryRowContext(
		ctx,
		`INSERT INTO users (username, email, password_hash, github_token, api_quota_used, api_quota_limit)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at, updated_at`,
		user.Username, user.Email, user.PasswordHash, user.GitHubToken,
		0, 1000, // Default quota
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
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
func (us *UserService) GetByUsername(ctx context.Context, username string) (*User, error) {
	var user User

	err := us.DB.QueryRowContext(
		ctx,
		`SELECT id, username, email, password_hash, github_token,
		        api_quota_used, api_quota_limit, created_at, updated_at, last_login
		 FROM users WHERE username = $1`,
		username,
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
func (us *UserService) UpdateGitHubToken(ctx context.Context, userID int, githubToken string) error {
	if githubToken == "" {
		return fmt.Errorf("github token cannot be empty")
	}

	result, err := us.DB.ExecContext(
		ctx,
		`UPDATE users SET github_token = $1, updated_at = NOW()
		 WHERE id = $2`,
		githubToken, userID,
	)

	if err != nil {
		return fmt.Errorf("failed to update github token: %w", err)
	}

	// to check if user not found in db
	rows, err := result.RowsAffected()
	if err != nil {
		return err
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
