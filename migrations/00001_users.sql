-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
    id              BIGSERIAL PRIMARY KEY,
    email           VARCHAR(255) UNIQUE NOT NULL,
    password_hash   VARCHAR(255) NOT NULL,
    github_token_hash VARCHAR(64),  -- SHA256 = 64 hex chars
    api_quota_used  INTEGER DEFAULT 0,
    api_quota_limit INTEGER DEFAULT 100000,  -- Token limit
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index for fast email lookups (authentication)
CREATE INDEX idx_users_email ON users(email);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE users;
-- +goose StatementEnd