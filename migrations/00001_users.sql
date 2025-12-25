-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
    id              BIGSERIAL PRIMARY KEY,
    email           VARCHAR(255) UNIQUE NOT NULL,
    password_hash   VARCHAR(255) NOT NULL,
    github_id BIGINT UNIQUE;
    github_username VARCHAR(255);
    github_access_token_encrypted TEXT;
    github_token_expires_at TIMESTAMP WITH TIME ZONE;
    github_connected_at TIMESTAMP WITH TIME ZONE;
    api_quota_used  INTEGER DEFAULT 0,
    api_quota_limit INTEGER DEFAULT 100000,  -- Token limit
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index for fast email lookups (authentication)
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_github_id ON users(github_id) WHERE github_id IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE users;
-- +goose StatementEnd