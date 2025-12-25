-- +goose Up
-- +goose StatementBegin
CREATE TABLE repositories (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    github_url       VARCHAR(500) NOT NULL,
    owner            VARCHAR(255) NOT NULL,
    name             VARCHAR(255) NOT NULL,
    description      TEXT,
    primary_language VARCHAR(100),
    stars_count      INTEGER DEFAULT 0,
    created_at       TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at       TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(user_id, github_url)  -- One entry per user per repo
);

CREATE INDEX idx_repositories_user_id ON repositories(user_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS repositories;
-- +goose StatementEnd
