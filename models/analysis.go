package models

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Analysis represents code analysis results
type Analysis struct {
	ID                   int             `json:"id"`
	RepositoryID         int64           `json:"repository_id"`
	CodeQualityScore     int             `json:"code_quality_score"`
	SecurityScore        int             `json:"security_score"`
	ComplexityScore      int             `json:"complexity_score"`
	MaintainabilityScore int             `json:"maintainability_score"`
	PerformanceScore     int             `json:"performance_score"`
	TotalIssues          int             `json:"total_issues"`
	CriticalIssues       int             `json:"critical_issues"`
	HighIssues           int             `json:"high_issues"`
	MediumIssues         int             `json:"medium_issues"`
	LowIssues            int             `json:"low_issues"`
	Summary              string          `json:"summary"`
	RawAnalysis          string          `json:"raw_analysis"`
	Issues               []CodeIssue     `json:"issues,omitempty"`
	CodeStructures       []CodeStructure `json:"code_structures,omitempty"`
	AnalyzedAt           time.Time       `json:"analyzed_at"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

// CodeIssue represents a code issue found during analysis
type CodeIssue struct {
	ID           int        `json:"id"`
	AnalysisID   int        `json:"analysis_id"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	IssueType    string     `json:"issue_type"`
	Severity     string     `json:"severity"`
	AffectedFile string     `json:"affected_file"`
	LineNumber   int        `json:"line_number"`
	SuggestedFix string     `json:"suggested_fix"`
	CodeSnippet  string     `json:"code_snippet"`
	IsResolved   bool       `json:"is_resolved"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// CodeStructure represents file structure analysis
type CodeStructure struct {
	ID            int       `json:"id"`
	RepositoryID  int64     `json:"repository_id"`
	FilePath      string    `json:"file_path"`
	FileType      string    `json:"file_type"`
	LinesOfCode   int       `json:"lines_of_code"`
	Complexity    int       `json:"complexity"`
	FunctionCount int       `json:"function_count"`
	ClassCount    int       `json:"class_count"`
	Description   string    `json:"description"`
	CreatedAt     time.Time `json:"created_at"`
}

// AnalysisService handles analysis-related database operations, its the basic repo details analytical data
type AnalysisService struct {
	DB *sql.DB
}

// Create creates a new analysis into the db
func (as *AnalysisService) Create(ctx context.Context, analysis *Analysis) (*Analysis, error) {
	if analysis.RepositoryID == 0 {
		return nil, fmt.Errorf("repository_id is required")
	}

	err := as.DB.QueryRowContext(
		ctx,
		`INSERT INTO analyses (repository_id, code_quality_score, security_score,
		                      complexity_score, maintainability_score, performance_score,
		                      total_issues, critical_issues, high_issues, medium_issues, low_issues,
		                      summary, raw_analysis, analyzed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
		 RETURNING id, created_at, updated_at, analyzed_at`,
		analysis.RepositoryID, analysis.CodeQualityScore, analysis.SecurityScore,
		analysis.ComplexityScore, analysis.MaintainabilityScore, analysis.PerformanceScore,
		analysis.TotalIssues, analysis.CriticalIssues, analysis.HighIssues,
		analysis.MediumIssues, analysis.LowIssues, analysis.Summary, analysis.RawAnalysis,
	).Scan(&analysis.ID, &analysis.CreatedAt, &analysis.UpdatedAt, &analysis.AnalyzedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create analysis: %w", err)
	}

	return analysis, nil
}

// GetByID retrieves an analysis by ID
func (as *AnalysisService) GetByID(ctx context.Context, id int) (*Analysis, error) {
	var analysis Analysis

	err := as.DB.QueryRowContext(
		ctx,
		`SELECT id, repository_id, code_quality_score, security_score,
		        complexity_score, maintainability_score, performance_score,
		        total_issues, critical_issues, high_issues, medium_issues, low_issues,
		        summary, raw_analysis, analyzed_at, created_at, updated_at
		 FROM analyses WHERE id = $1`,
		id,
	).Scan(
		&analysis.ID, &analysis.RepositoryID, &analysis.CodeQualityScore,
		&analysis.SecurityScore, &analysis.ComplexityScore,
		&analysis.MaintainabilityScore, &analysis.PerformanceScore,
		&analysis.TotalIssues, &analysis.CriticalIssues, &analysis.HighIssues,
		&analysis.MediumIssues, &analysis.LowIssues, &analysis.Summary,
		&analysis.RawAnalysis, &analysis.AnalyzedAt, &analysis.CreatedAt,
		&analysis.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("analysis not found")
		}
		return nil, fmt.Errorf("failed to fetch analysis: %w", err)
	}

	return &analysis, nil
}

// GetByRepositoryID retrieves the latest analysis for a repository
func (as *AnalysisService) GetByRepositoryID(ctx context.Context, repositoryID int64) (*Analysis, error) {
	var analysis Analysis

	err := as.DB.QueryRowContext(
		ctx,
		`SELECT id, repository_id, code_quality_score, security_score,
		        complexity_score, maintainability_score, performance_score,
		        total_issues, critical_issues, high_issues, medium_issues, low_issues,
		        summary, raw_analysis, analyzed_at, created_at, updated_at
		 FROM analyses WHERE repository_id = $1 ORDER BY analyzed_at DESC LIMIT 1`,
		repositoryID,
	).Scan(
		&analysis.ID, &analysis.RepositoryID, &analysis.CodeQualityScore,
		&analysis.SecurityScore, &analysis.ComplexityScore,
		&analysis.MaintainabilityScore, &analysis.PerformanceScore,
		&analysis.TotalIssues, &analysis.CriticalIssues, &analysis.HighIssues,
		&analysis.MediumIssues, &analysis.LowIssues, &analysis.Summary,
		&analysis.RawAnalysis, &analysis.AnalyzedAt, &analysis.CreatedAt,
		&analysis.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no analysis found for this repository")
		}
		return nil, fmt.Errorf("failed to fetch analysis: %w", err)
	}

	return &analysis, nil
}

// GetHistoryByRepositoryID retrieves analysis history for a repository
func (as *AnalysisService) GetHistoryByRepositoryID(ctx context.Context, repositoryID int64, limit int) ([]Analysis, error) {
	rows, err := as.DB.QueryContext(
		ctx,
		`SELECT id, repository_id, code_quality_score, security_score,
		        complexity_score, maintainability_score, performance_score,
		        total_issues, critical_issues, high_issues, medium_issues, low_issues,
		        summary, raw_analysis, analyzed_at, created_at, updated_at
		 FROM analyses WHERE repository_id = $1
		 ORDER BY analyzed_at DESC LIMIT $2`,
		repositoryID, limit,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to fetch analysis history: %w", err)
	}
	defer rows.Close()

	var analyses []Analysis

	for rows.Next() {
		var analysis Analysis

		err := rows.Scan(
			&analysis.ID, &analysis.RepositoryID, &analysis.CodeQualityScore,
			&analysis.SecurityScore, &analysis.ComplexityScore,
			&analysis.MaintainabilityScore, &analysis.PerformanceScore,
			&analysis.TotalIssues, &analysis.CriticalIssues, &analysis.HighIssues,
			&analysis.MediumIssues, &analysis.LowIssues, &analysis.Summary,
			&analysis.RawAnalysis, &analysis.AnalyzedAt, &analysis.CreatedAt,
			&analysis.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan analysis: %w", err)
		}

		analyses = append(analyses, analysis)
	}

	return analyses, rows.Err()
}

// Delete deletes an analysis
func (as *AnalysisService) Delete(ctx context.Context, id int) error {
	_, err := as.DB.ExecContext(
		ctx,
		`DELETE FROM analyses WHERE id = $1`,
		id,
	)

	return err
}
