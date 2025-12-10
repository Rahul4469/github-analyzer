-- +goose Up
-- +goose StatementBegin
CREATE TABLE analyses (
    id SERIAL PRIMARY KEY,
    repository_id INTEGER NOT NULL UNIQUE REFERENCES repositories(id) ON DELETE CASCADE,
    code_quality_score INTEGER DEFAULT 0,
    security_score INTEGER DEFAULT 0,
    complexity_score INTEGER DEFAULT 0,
    maintainability_score INTEGER DEFAULT 0,
    performance_score INTEGER DEFAULT 0,
    total_issues INTEGER DEFAULT 0,
    critical_issues INTEGER DEFAULT 0,
    high_issues INTEGER DEFAULT 0,
    medium_issues INTEGER DEFAULT 0,
    low_issues INTEGER DEFAULT 0,
    summary TEXT,
    raw_analysis TEXT,
    analyzed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_analyses_repository ON analyses(repository_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS analyses;
-- +goose StatementEnd
