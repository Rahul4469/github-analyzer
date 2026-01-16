package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AnalysisStatus string

const (
	StatusPending    AnalysisStatus = "pending"
	StatusProcessing AnalysisStatus = "processing"
	StatusCompleted  AnalysisStatus = "completed"
	StatusFailed     AnalysisStatus = "failed"
)

type FileContent struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Language string `json:"language"`
	Size     int    `json:"size"`
}

type CodeStructure struct {
	TotalFiles        int            `json:"total_files"`
	TotalSize         int            `json:"total_size"`
	Directories       []string       `json:"directories"`
	Files             []string       `json:"files"`
	LanguageBreakdown map[string]int `json:"language_breakdown"`
}

type Issue struct {
	Severity    string `json:"severity"` // HIGH, MEDIUM, LOW
	Category    string `json:"category"`
	Title       string `json:"title"`
	Description string `json:"description"`
	File        string `json:"file,omitempty"`
	Line        int    `json:"line,omitempty"`
	Suggestion  string `json:"suggestion,omitempty"`
}

type AnalysisSummary struct {
	TotalIssues      int            `json:"total_issues"`
	IssuesBySeverity map[string]int `json:"issues_by_severity"`
	IssuesByCategory map[string]int `json:"issues_by_category"`
	OverallScore     int            `json:"overall_score"`
	KeyFindings      []string       `json:"key_findings"`
}

type Analysis struct {
	ID           int64          `json:"id"`
	UserID       int64          `json:"user_id"`
	RepositoryID int64          `json:"repository_id"`
	Status       AnalysisStatus `json:"status"`

	// Data fetched from GitHub, jsonb
	CodeStructure *CodeStructure `json:"code_structure,omitempty"`
	CodeFiles     []FileContent  `json:"code_files,omitempty"`
	READMEContent *string        `json:"readme_content,omitempty"`

	// AI analysis results
	AIAnalysis *string          `json:"ai_analysis,omitempty"`
	Summary    *AnalysisSummary `json:"summary,omitempty"`
	Issues     []Issue          `json:"issues,omitempty"`

	// Usage tracking
	TokensUsed   int     `json:"tokens_used"`
	ErrorMessage *string `json:"error_message,omitempty"`

	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Joined data
	Repository *Repository `json:"repository,omitempty"`
}

type AnalysisService struct {
	pool *pgxpool.Pool
}

func NewAnalysisService(pool *pgxpool.Pool) *AnalysisService {
	return &AnalysisService{pool: pool}
}

func (s *AnalysisService) Create(ctx context.Context, userID, repositoryID int64) (*Analysis, error) {
	query := `
		INSERT INTO analyses (user_id, repository_id, status)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, repository_id, status, code_structure, readme_content, 
		          ai_analysis, tokens_used, error_message, created_at, started_at, completed_at
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	analysis := &Analysis{}
	var codeStructureJSON []byte

	err := s.pool.QueryRow(ctx, query, userID, repositoryID, StatusPending).Scan(
		&analysis.ID,
		&analysis.UserID,
		&analysis.RepositoryID,
		&analysis.Status,
		&codeStructureJSON,
		&analysis.READMEContent,
		&analysis.AIAnalysis,
		&analysis.TokensUsed,
		&analysis.ErrorMessage,
		&analysis.CreatedAt,
		&analysis.StartedAt,
		&analysis.CompletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create analysis: %w", err)
	}

	return analysis, nil
}

func (s *AnalysisService) MarkProcessing(ctx context.Context, analysisID int64) error {
	query := `
		UPDATE analyses 
		SET status = $1, started_at = NOW()
		WHERE id = $2
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	_, err := s.pool.Exec(ctx, query, StatusProcessing, analysisID)
	if err != nil {
		return fmt.Errorf("failed to mark analysis as processing: %w", err)
	}

	return nil
}

func (s *AnalysisService) UpdateGitHubData(ctx context.Context, analysisID int64, codeStructure *CodeStructure, codeFiles []FileContent, readme string) error {
	// Combine code structure and files into a single JSONB structure
	combinedData := struct {
		Structure *CodeStructure `json:"structure"`
		Files     []FileContent  `json:"files"`
	}{
		Structure: codeStructure,
		Files:     codeFiles,
	}

	combinedJSON, err := json.Marshal(combinedData)
	if err != nil {
		return fmt.Errorf("failed to marshal combined data: %w", err)
	}

	query := `
        UPDATE analyses 
        SET code_structure = $1, readme_content = $2
        WHERE id = $3
    `

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	_, err = s.pool.Exec(ctx, query, combinedJSON, readme, analysisID)
	if err != nil {
		return fmt.Errorf("failed to update GitHub data: %w", err)
	}

	return nil
}

func (s *AnalysisService) Complete(ctx context.Context, analysisID int64, aiAnalysis string, summary *AnalysisSummary, issues []Issue, tokensUsed int) error {
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("failed to marshal summary: %w", err)
	}

	// Store issues within the AI analysis or as a separate field
	fullResult := struct {
		RawAnalysis string           `json:"raw_analysis"`
		Summary     *AnalysisSummary `json:"summary"`
		Issues      []Issue          `json:"issues"`
	}{
		RawAnalysis: aiAnalysis,
		Summary:     summary,
		Issues:      issues,
	}

	fullResultJSON, err := json.Marshal(fullResult)
	if err != nil {
		return fmt.Errorf("failed to marshal full result: %w", err)
	}

	query := `
		UPDATE analyses 
		SET status = $1, ai_analysis = $2, tokens_used = $3, completed_at = NOW()
		WHERE id = $4
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	_, err = s.pool.Exec(ctx, query, StatusCompleted, string(fullResultJSON), tokensUsed, analysisID)
	if err != nil {
		return fmt.Errorf("failed to complete analysis: %w", err)
	}

	_ = summaryJSON // We stored it in fullResultJSON instead

	return nil
}

// Fail marks the analysis as failed with an error message.
func (s *AnalysisService) Fail(ctx context.Context, analysisID int64, errorMsg string) error {
	query := `
		UPDATE analyses 
		SET status = $1, error_message = $2, completed_at = NOW()
		WHERE id = $3
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	_, err := s.pool.Exec(ctx, query, StatusFailed, errorMsg, analysisID)
	if err != nil {
		return fmt.Errorf("failed to mark analysis as failed: %w", err)
	}

	return nil
}

func (s *AnalysisService) ByID(ctx context.Context, id int64) (*Analysis, error) {
	query := `
		SELECT a.id, a.user_id, a.repository_id, a.status, a.code_structure, a.readme_content,
		       a.ai_analysis, a.tokens_used, a.error_message, a.created_at, a.started_at, a.completed_at,
		       r.id, r.github_url, r.owner, r.name, r.description, r.primary_language, r.stars_count, r.forks_count
		FROM analyses a
		JOIN repositories r ON a.repository_id = r.id
		WHERE a.id = $1
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	analysis := &Analysis{Repository: &Repository{}}
	var codeStructureJSON []byte
	var aiAnalysisJSON *string

	err := s.pool.QueryRow(ctx, query, id).Scan(
		&analysis.ID,
		&analysis.UserID,
		&analysis.RepositoryID,
		&analysis.Status,
		&codeStructureJSON,
		&analysis.READMEContent,
		&aiAnalysisJSON,
		&analysis.TokensUsed,
		&analysis.ErrorMessage,
		&analysis.CreatedAt,
		&analysis.StartedAt,
		&analysis.CompletedAt,
		&analysis.Repository.ID,
		&analysis.Repository.GitHubURL,
		&analysis.Repository.Owner,
		&analysis.Repository.Name,
		&analysis.Repository.Description,
		&analysis.Repository.PrimaryLanguage,
		&analysis.Repository.StarsCount,
		&analysis.Repository.ForksCount,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAnalysisNotFound
		}
		return nil, fmt.Errorf("failed to get analysis: %w", err)
	}

	// Parse JSON fields
	if len(codeStructureJSON) > 0 {
		var combined struct {
			Structure *CodeStructure `json:"structure"`
			Files     []FileContent  `json:"files"`
		}
		if err := json.Unmarshal(codeStructureJSON, &combined); err == nil {
			analysis.CodeStructure = combined.Structure
			analysis.CodeFiles = combined.Files
		}
	}

	if aiAnalysisJSON != nil && *aiAnalysisJSON != "" {
		var fullResult struct {
			RawAnalysis string           `json:"raw_analysis"`
			Summary     *AnalysisSummary `json:"summary"`
			Issues      []Issue          `json:"issues"`
		}
		if err := json.Unmarshal([]byte(*aiAnalysisJSON), &fullResult); err == nil {
			analysis.AIAnalysis = &fullResult.RawAnalysis
			analysis.Summary = fullResult.Summary
			analysis.Issues = fullResult.Issues
		} else {
			// Fallback: treat as raw text
			analysis.AIAnalysis = aiAnalysisJSON
		}
	}

	return analysis, nil
}

func (s *AnalysisService) ByUserID(ctx context.Context, userID int64, limit int) ([]*Analysis, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT a.id, a.user_id, a.repository_id, a.status, a.tokens_used, a.error_message,
		       a.created_at, a.started_at, a.completed_at,
		       r.id, r.github_url, r.owner, r.name, r.description, r.primary_language, r.stars_count, r.forks_count
		FROM analyses a
		JOIN repositories r ON a.repository_id = r.id
		WHERE a.user_id = $1
		ORDER BY a.created_at DESC
		LIMIT $2
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	rows, err := s.pool.Query(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list analyses: %w", err)
	}
	defer rows.Close()

	var analyses []*Analysis
	for rows.Next() {
		analysis := &Analysis{Repository: &Repository{}}
		err := rows.Scan(
			&analysis.ID,
			&analysis.UserID,
			&analysis.RepositoryID,
			&analysis.Status,
			&analysis.TokensUsed,
			&analysis.ErrorMessage,
			&analysis.CreatedAt,
			&analysis.StartedAt,
			&analysis.CompletedAt,
			&analysis.Repository.ID,
			&analysis.Repository.GitHubURL,
			&analysis.Repository.Owner,
			&analysis.Repository.Name,
			&analysis.Repository.Description,
			&analysis.Repository.PrimaryLanguage,
			&analysis.Repository.StarsCount,
			&analysis.Repository.ForksCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan analysis: %w", err)
		}
		analyses = append(analyses, analysis)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating analyses: %w", err)
	}

	return analyses, nil
}

// CountByUser returns the number of analyses for a user.
func (s *AnalysisService) CountByUser(ctx context.Context, userID int64) (int, error) {
	query := `SELECT COUNT(*) FROM analyses WHERE user_id = $1`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	var count int
	err := s.pool.QueryRow(ctx, query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count analyses: %w", err)
	}

	return count, nil
}

// CountByStatus returns counts of analyses grouped by status for a user.
func (s *AnalysisService) CountByStatus(ctx context.Context, userID int64) (map[AnalysisStatus]int, error) {
	query := `
		SELECT status, COUNT(*) 
		FROM analyses 
		WHERE user_id = $1 
		GROUP BY status
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to count analyses by status: %w", err)
	}
	defer rows.Close()

	counts := make(map[AnalysisStatus]int)
	for rows.Next() {
		var status AnalysisStatus
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan status count: %w", err)
		}
		counts[status] = count
	}

	return counts, nil
}

func (s *AnalysisService) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM analyses WHERE id = $1`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	result, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete analysis: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrAnalysisNotFound
	}

	return nil
}

// GetPendingAnalyses retrieves analyses that are waiting to be processed.
// Useful for background job processing.
func (s *AnalysisService) GetPendingAnalyses(ctx context.Context, limit int) ([]*Analysis, error) {
	query := `
		SELECT id, user_id, repository_id, status, created_at
		FROM analyses
		WHERE status = $1
		ORDER BY created_at ASC
		LIMIT $2
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	rows, err := s.pool.Query(ctx, query, StatusPending, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending analyses: %w", err)
	}
	defer rows.Close()

	var analyses []*Analysis
	for rows.Next() {
		analysis := &Analysis{}
		err := rows.Scan(
			&analysis.ID,
			&analysis.UserID,
			&analysis.RepositoryID,
			&analysis.Status,
			&analysis.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan analysis: %w", err)
		}
		analyses = append(analyses, analysis)
	}

	return analyses, nil
}

// HELPER FUNCS --------------------------------

// Duration returns how long the analysis took.
// Returns 0 if not completed.
func (a *Analysis) Duration() time.Duration {
	if a.StartedAt == nil || a.CompletedAt == nil {
		return 0
	}
	return a.CompletedAt.Sub(*a.StartedAt)
}

func (a *Analysis) IsPending() bool {
	return a.Status == StatusPending
}

func (a *Analysis) IsProcessing() bool {
	return a.Status == StatusProcessing
}

func (a *Analysis) IsCompleted() bool {
	return a.Status == StatusCompleted
}

func (a *Analysis) IsFailed() bool {
	return a.Status == StatusFailed
}

// HighSeverityCount returns the number of high severity issues.
func (a *Analysis) HighSeverityCount() int {
	if a.Summary == nil {
		return 0
	}
	return a.Summary.IssuesBySeverity["HIGH"]
}
