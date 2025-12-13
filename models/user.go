package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"golang.org/x/crypto/bcrypt"
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

func (us *UserService) Create(email string, password string) (*User, error) {
	email = strings.ToLower(email)
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	passwordHash := string(hashedBytes)

	user := User{
		Email:        email,
		PasswordHash: passwordHash,
	}

	row := us.DB.QueryRow(`
	    INSERT INTO users (email, password_hash)
		VALUES ($1, $2) RETURNING id, created_at, updated_at`, email, passwordHash)
	err = row.Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		var pgError *pgconn.PgError
		if errors.As(err, &pgError) {
			if pgError.Code == pgerrcode.UniqueViolation {
				return nil, ErrEmailTaken
			}
		}
		fmt.Printf("type = %T\n", err)
		fmt.Printf("Error = %v\n", err)
		return nil, fmt.Errorf("create user: %w", err)
	}

	return &user, nil
}

func (us *UserService) Authenticate(email, password string) (*User, error) {
	email = strings.ToLower(email)
	user := User{
		Email: email,
	}

	row := us.DB.QueryRow(`
	SELECT id, password_hash, github_username, github_token, created_at, updated_at
	FROM users 
	WHERE email=$1`, email)
	err := row.Scan(&user.ID,
		&user.PasswordHash,
		&user.Username,
		&user.GitHubToken,
		&user.CreatedAt,
		&user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("authenticate: %w", err)
	}
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, fmt.Errorf("authenticate: %w", err)
	}
	return &user, nil
}
func (us *UserService) ByID(id int) (*User, error) {
	user := User{}

	row := us.DB.QueryRow(`
	SELECT id, password_hash
	FROM users 
	WHERE id=$1`, id)
	err := row.Scan(&user.Email,
		&user.PasswordHash,
		&user.Username,
		&user.GitHubToken,
		&user.CreatedAt,
		&user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("authenticate: %w", err)
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

// When user provides Github credentials
// UpdateGithubCredentials stored github username and token
func (us *UserService) UpdateGithubCredentials(userID int, username string, token string) error {
	_, err := us.DB.Exec(`
	UPDATE users
	SET github_username = $1, github_token = $2, updated_at = NOW()
    WHERE id = $3`, username, token, userID)

	if err != nil {
		return fmt.Errorf("update gitub credentials: %w", err)
	}

	return nil
}
