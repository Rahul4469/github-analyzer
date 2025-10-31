-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
    id SERIAL PRIMARY KEY,                      -- Auto-incrementing ID
    email TEXT UNIQUE NOT NULL,         -- Unique email
    password_hash TEXT NOT NULL,        -- Hashed password (never store plain text!)
    github_username TEXT,               -- GitHub username (nullable)
    github_token TEXT,                          -- GitHub personal access token
    created_at TIMESTAMP NOT NULL DEFAULT NOW(), -- Creation timestamp
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()  -- Last update timestamp
);

-- Index for fast email lookups (authentication)
CREATE INDEX idx_users_email ON users(email);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS users;
-- +goose StatementEnd