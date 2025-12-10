-- +goose Up
-- +goose StatementBegin
CREATE TABLE commits (
    id SERIAL PRIMARY KEY,
    repository_id INTEGER NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    commit_hash VARCHAR(40) UNIQUE NOT NULL,
    commit_message TEXT NOT NULL,
    author_name VARCHAR(255),
    author_email VARCHAR(255),
    additions INTEGER DEFAULT 0,
    deletions INTEGER DEFAULT 0,
    changed_files INTEGER DEFAULT 0,
    commit_date TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_commits_repository ON commits(repository_id);
CREATE INDEX idx_commits_date ON commits(commit_date);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS commits;
-- +goose StatementEnd
