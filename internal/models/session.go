package models

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"
)

type Session struct {
	ID     int `json:"id"`
	UserID int `json:"user_id"`
	//Token is only set when creating a new session.when looking up a session
	//this will be left empy, as we only store the hash of a session token
	//in our database a we cannot reverse it into a raw token.
	Token     string
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
}

const (
	// MinBytesPerToken is the minimum number of bytes for a session token
	MinBytesPerToken = 32
	// DefaultTokenLength is the default token length (32 bytes = 256 bits)
	DefaultTokenLength = 32
	// SessionDuration is how long a session lasts (24 hours)
	SessionDuration = 24 * time.Hour
)

type SessionService struct {
	DB *sql.DB

	BytesPerToken   int
	SessionDuration time.Duration // to set time duration to 24 hr
}

func NewSessionService(db *sql.DB) *SessionService {
	return &SessionService{
		DB:              db,
		BytesPerToken:   DefaultTokenLength,
		SessionDuration: SessionDuration,
	}
}

// Create new session for user
func (ss *SessionService) Create(userID int) (*Session, error) {
	// check token length
	bytesPerToken := ss.BytesPerToken
	if bytesPerToken < MinBytesPerToken {
		bytesPerToken = MinBytesPerToken
	}
	token, err := ss.generateToken(bytesPerToken)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	session := Session{
		UserID:    userID,
		Token:     token,
		TokenHash: ss.hash(token),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(ss.SessionDuration),
	}

	row := ss.DB.QueryRow(`
	INSERT INTO sessions (user_id, token_hash, created_at, expires_at)
	VALUES($1, $2, NOW(), NOW() + INTERVAL '24 hours')
	ON CONFLICT (user_id)
	DO UPDATE
	SET token_hash = $2
	RETURNING id, created_at, expires_at
	`, session.UserID, session.TokenHash)

	err = row.Scan(&session.ID, &session.CreatedAt, &session.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	return &session, nil
}

// Validate token and return user
func (ss *SessionService) User(token string) (*User, error) {
	tokenHash := ss.hash(token)
	var user User
	row := ss.DB.QueryRow(`
	SELECT users.id,
		users.email,
		users.password_hash
	FROM sessions
	JOIN users ON users.id = sessions.user_id
	WHERE sessions.token_hash = $1 AND sessions.expires_at > NOW();`, tokenHash)
	err := row.Scan(&user.ID, &user.Email, &user.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("users: %w", err)
	}

	return &user, nil
}

func (ss *SessionService) Delete(token string) error {
	tokenHash := ss.hash(token)
	result, err := ss.DB.Exec(`
	DELETE FROM sessions
	WHERE token_hash = $1`, tokenHash)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("session not found")
	}
	return nil
}

func (ss *SessionService) generateToken(length int) (string, error) {
	// Create byte slice
	b := make([]byte, length)

	// Fill with random bytes
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("failed to read random: %w", err)
	}

	// Encode to base64 for URL-safe string
	token := base64.URLEncoding.EncodeToString(b)
	return token, nil
}

// Store hash in database
func (ss *SessionService) hash(token string) string {
	// Create SHA256 hash
	hash := sha256.Sum256([]byte(token))

	// Encode to base64
	tokenHash := base64.URLEncoding.EncodeToString(hash[:])
	return tokenHash
}
