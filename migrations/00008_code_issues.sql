-- +goose Up
-- +goose StatementBegin
CREATE TABLE code_issues (
    id SERIAL PRIMARY KEY,
    analysis_id INTEGER NOT NULL REFERENCES analyses(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    issue_type VARCHAR(50),
    severity VARCHAR(50),
    affected_file VARCHAR(500),
    line_number INTEGER,
    suggested_fix TEXT,
    code_snippet TEXT,
    is_resolved BOOLEAN DEFAULT FALSE,
    resolved_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_code_issues_analysis ON code_issues(analysis_id);
CREATE INDEX idx_code_issues_severity ON code_issues(severity);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS code_issues;
-- +goose StatementEnd
