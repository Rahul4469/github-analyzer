-- +goose Up
-- +goose StatementBegin
CREATE TYPE analysis_status AS ENUM ('pending', 'processing', 'completed', 'failed');

CREATE TABLE analyses (
    id             BIGSERIAL PRIMARY KEY,
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    repository_id  BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    status         VARCHAR(20) DEFAULT 'pending',  -- pending, processing, completed, failed
    code_structure JSONB,                           -- File tree as JSON
    code_files     JSONB ,                        -- store top 10 code files to send for analysis
    readme_content TEXT,                           -- Fetched README
    ai_analysis    TEXT,                           -- Perplexity response
    tokens_used    INTEGER DEFAULT 0,
    error_message  TEXT,                           -- If failed
    created_at     TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at   TIMESTAMP WITH TIME ZONE
);

-- Index for listing user's analyses (dashboard)
CREATE INDEX idx_analyses_user_id ON analyses(user_id);

-- Index for finding analyses by repository
CREATE INDEX idx_analyses_repository_id ON analyses(repository_id);

-- Index for filtering by status (e.g., finding pending jobs)
CREATE INDEX idx_analyses_status ON analyses(status);

-- Index for sorting by creation time (most recent first)
CREATE INDEX idx_analyses_created_at ON analyses(created_at DESC);

-- Composite index for common query: user's analyses sorted by date
CREATE INDEX idx_analyses_user_created ON analyses(user_id, created_at DESC);

COMMENT ON TABLE analyses IS 'AI analysis results for GitHub repositories';
COMMENT ON COLUMN analyses.code_structure IS 'JSON array of file paths in the repository';
COMMENT ON COLUMN analyses.readme_content IS 'Raw README.md content fetched from GitHub';
COMMENT ON COLUMN analyses.ai_analysis IS 'Complete analysis response from Perplexity AI';
COMMENT ON COLUMN analyses.tokens_used IS 'Perplexity API tokens consumed for this analysis';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS analyses;
DROP TYPE IF EXISTS analysis_status;
-- +goose StatementEnd
