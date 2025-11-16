package models

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"

	"github.com/rahul4469/github-analyzer/rand"
)

const MinBytesPerToken = 32

type Session struct {
	ID     int
	UserID int
	//Token is only set when creating a new session.when looking up a session
	//this will be left empy, as we only store the hash of a session token
	//in our database a we cannot reverse it into a raw token.
	Token     string
	TokenHash string
}

type SessionService struct {
	DB *sql.DB

	BytesPerToken int
}

func (ss *SessionService) Create(userID int) (*Session, error) {
	bytesPerToken := ss.BytesPerToken
	if bytesPerToken < MinBytesPerToken {
		bytesPerToken = MinBytesPerToken
	}
	token, err := rand.String(bytesPerToken)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	session := Session{
		UserID:    userID,
		Token:     token,
		TokenHash: ss.hash(token),
	}

	row := ss.DB.QueryRow(`
	INSERT INTO sessions (user_id, token_hash)
	VALUES($1, $2) ON CONFLICT (user_id)
	DO UPDATE
	SET token_hash = $2
	RETURNING id`, session.UserID, session.TokenHash)

	err = row.Scan(&session.ID)
	if err != nil {
		return nil, fmt.Errorf("Create: %w", err)
	}
	return &session, nil
}

func (ss *SessionService) hash(token string) string {
	tokenHash := sha256.Sum256([]byte(token))
	return base64.URLEncoding.EncodeToString(tokenHash[:])
}
