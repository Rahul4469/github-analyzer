package models

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	MinBytesPerToken = 32
	TokenLength      = 32
	SessionDuration  = 24 * time.Hour
)

type Session struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	TokenHash string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IsExpired returns true if the session has expired.
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// TimeUntilExpiry returns the duration until the session expires.
// Returns 0 if already expired.
func (s *Session) TimeUntilExpiry() time.Duration {
	remaining := time.Until(s.ExpiresAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

type SessionService struct {
	pool            *pgxpool.Pool
	sessionDuration time.Duration
}

func NewSessionService(pool *pgxpool.Pool, sessionDuration time.Duration) *SessionService {
	return &SessionService{
		pool:            pool,
		sessionDuration: sessionDuration,
	}
}

func (s *SessionService) Create(ctx context.Context, userID int64) (token string, session *Session, err error) {
	// Generate cryptographically secure random bytes
	tokenBytes := make([]byte, TokenLength)
	_, err = rand.Read(tokenBytes)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate session token: %w", err)
	}

	// Encode as base64 for cookie storage (URL-safe encoding)
	token = base64.URLEncoding.EncodeToString(tokenBytes)

	// Hash the token for database storage
	tokenHash := hashSessionToken(token)

	// Calculate expiration
	expiresAt := time.Now().Add(s.sessionDuration)

	// Insert session into database
	query := `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, token_hash, created_at, expires_at
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	session = &Session{}
	err = s.pool.QueryRow(ctx, query, userID, tokenHash, expiresAt).Scan(
		&session.ID,
		&session.UserID,
		&session.TokenHash,
		&session.CreatedAt,
		&session.ExpiresAt,
	)

	if err != nil {
		return "", nil, fmt.Errorf("failed to create session: %w", err)
	}

	return token, session, nil
}

// User retrieves the user associated with a session token.
// 1. Hash the provided token
// 2. Look up session by hash
// 3. Check if expired
// 4. Return associated user
func (s *SessionService) User(ctx context.Context, token string) (*User, error) {
	tokenHash := hashSessionToken(token)

	// Join sessions with users to get user data in one query
	query := `
		SELECT 
			u.id, u.email, u.password_hash, u.github_token_hash, 
			u.api_quota_used, u.api_quota_limit, u.created_at, u.updated_at,
			s.expires_at
		FROM sessions s
		JOIN users u ON s.user_id = u.id
		WHERE s.token_hash = $1
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	user := &User{}
	var expiresAt time.Time

	err := s.pool.QueryRow(ctx, query, tokenHash).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.GitHubTokenHash,
		&user.APIQuotaUsed,
		&user.APIQuotaLimit,
		&user.CreatedAt,
		&user.UpdatedAt,
		&expiresAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to get user from session: %w", err)
	}

	// Check if session has expired
	if time.Now().After(expiresAt) {
		// Optionally delete the expired session
		go s.deleteByHash(context.Background(), tokenHash)
		return nil, ErrSessionExpired
	}

	return user, nil
}

// Delete removes a session by its raw token.
// Called during logout.
func (s *SessionService) Delete(ctx context.Context, token string) error {
	tokenHash := hashSessionToken(token)
	return s.deleteByHash(ctx, tokenHash)
}

// deleteByHash removes a session by its hash.
// Internal method used by Delete and cleanup routines.
func (s *SessionService) deleteByHash(ctx context.Context, tokenHash string) error {
	query := `DELETE FROM sessions WHERE token_hash = $1`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	_, err := s.pool.Exec(ctx, query, tokenHash)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// DeleteAllForUser removes all sessions for a specific user.
// Use this for "logout from all devices" functionality or account deletion.
func (s *SessionService) DeleteAllForUser(ctx context.Context, userID int64) error {
	query := `DELETE FROM sessions WHERE user_id = $1`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	_, err := s.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user sessions: %w", err)
	}

	return nil
}

// DeleteExpired removes all expired sessions from the database.
// Should be called periodically (e.g., via cron job or background goroutine).
//
// Returns the number of sessions deleted.
func (s *SessionService) DeleteExpired(ctx context.Context) (int64, error) {
	query := `DELETE FROM sessions WHERE expires_at < NOW()`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	result, err := s.pool.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired sessions: %w", err)
	}

	return result.RowsAffected(), nil
}

// CountActiveSessions returns the number of active sessions for a user.
// Useful for security monitoring or limiting concurrent sessions.
func (s *SessionService) CountActiveSessions(ctx context.Context, userID int64) (int, error) {
	query := `
		SELECT COUNT(*) 
		FROM sessions 
		WHERE user_id = $1 AND expires_at > NOW()
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	var count int
	err := s.pool.QueryRow(ctx, query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count sessions: %w", err)
	}

	return count, nil
}

// Extend updates the expiration time of a session.
// Use this for "remember me" functionality or session refresh.
func (s *SessionService) Extend(ctx context.Context, token string, duration time.Duration) error {
	tokenHash := hashSessionToken(token)
	newExpiry := time.Now().Add(duration)

	query := `
		UPDATE sessions 
		SET expires_at = $1 
		WHERE token_hash = $2 AND expires_at > NOW()
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	result, err := s.pool.Exec(ctx, query, newExpiry, tokenHash)
	if err != nil {
		return fmt.Errorf("failed to extend session: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrSessionNotFound
	}

	return nil
}

// HELPER FUNCTIONS -------------------------------------

func hashSessionToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// StartCleanupRoutine starts a background goroutine that periodically
// cleans up expired sessions. Returns a channel that can be closed to stop cleanup.
func (s *SessionService) StartCleanupRoutine(interval time.Duration) chan struct{} {
	stop := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				count, err := s.DeleteExpired(ctx)
				cancel()

				if err != nil {
					// Log error but continue
					fmt.Printf("Session cleanup error: %v\n", err)
				} else if count > 0 {
					fmt.Printf("Cleaned up %d expired sessions\n", count)
				}

			case <-stop:
				return
			}
		}
	}()

	return stop
}
