-- +goose Up
-- +goose StatementBegin
CREATE TABLE languages (
    id SERIAL PRIMARY KEY,
    repository_id INTEGER NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    language VARCHAR(100) NOT NULL,
    percentage DECIMAL(5, 2),
    bytes_of_code BIGINT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_languages_repository ON languages(repository_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS languages;
-- +goose StatementEnd
