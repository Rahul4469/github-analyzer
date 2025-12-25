package models

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// both HTTPS and shorthand formats supported:
// - https://github.com/owner/repo
// - https://github.com/owner/repo.git
// - github.com/owner/repo
// MustCompile for fail fast impl
var GitHubURLPattern = regexp.MustCompile(`^(?:https?://)?github\.com/([a-zA-Z0-9_.-]+)/([a-zA-Z0-9_.-]+?)(?:\.git)?/?$`)

type Repository struct {
	ID              int64     `json:"id"`
	UserID          int64     `json:"user_id"`
	GitHubURL       string    `json:"github_url"`
	Owner           string    `json:"owner"`
	Name            string    `json:"name"`
	Description     *string   `json:"description,omitempty"`
	PrimaryLanguage *string   `json:"primary_language,omitempty"`
	StarsCount      int       `json:"stars_count"`
	ForksCount      int       `json:"forks_count"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type RepositoryService struct {
	pool *pgxpool.Pool
}

// constructor ~
func NewRepositoryService(pool *pgxpool.Pool) *RepositoryService {
	return &RepositoryService{pool: pool}
}

// ParseGitHubURL extracts owner and repo name from a GitHub URL.
func ParseGitHubURL(url string) (owner, repo string, err error) {
	url = strings.TrimSpace(url)

	matches := GitHubURLPattern.FindStringSubmatch(url)
	if matches == nil || len(matches) != 3 {
		return "", "", ErrInvalidRepositoryURL
	}

	return matches[1], matches[2], nil
}

// save repo data to db
func (s *RepositoryService) Create(ctx context.Context, repo *Repository) (*Repository, error) {
	// Validate URL format
	owner, name, err := ParseGitHubURL(repo.GitHubURL)
	if err != nil {
		return nil, err
	}

	// Normalize the URL
	repo.Owner = owner
	repo.Name = name
	repo.GitHubURL = fmt.Sprintf("https://github.com/%s/%s", owner, name)

	query := `
		INSERT INTO repositories (user_id, github_url, owner, name, description, primary_language, stars_count, forks_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id, github_url) DO UPDATE SET
			description = EXCLUDED.description,
			primary_language = EXCLUDED.primary_language,
			stars_count = EXCLUDED.stars_count,
			forks_count = EXCLUDED.forks_count,
			updated_at = NOW()
		RETURNING id, user_id, github_url, owner, name, description, primary_language, stars_count, forks_count, created_at, updated_at
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	result := &Repository{}
	err = s.pool.QueryRow(ctx, query,
		repo.UserID,
		repo.GitHubURL,
		repo.Owner,
		repo.Name,
		repo.Description,
		repo.PrimaryLanguage,
		repo.StarsCount,
		repo.ForksCount,
	).Scan(
		&result.ID,
		&result.UserID,
		&result.GitHubURL,
		&result.Owner,
		&result.Name,
		&result.Description,
		&result.PrimaryLanguage,
		&result.StarsCount,
		&result.ForksCount,
		&result.CreatedAt,
		&result.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create repository: %w", err)
	}

	return result, nil
}

// ByID retrieves a repository by its ID.
func (s *RepositoryService) ByID(ctx context.Context, id int64) (*Repository, error) {
	query := `
		SELECT id, user_id, github_url, owner, name, description, primary_language, stars_count, forks_count, created_at, updated_at
		FROM repositories
		WHERE id = $1
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	repo := &Repository{}
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&repo.ID,
		&repo.UserID,
		&repo.GitHubURL,
		&repo.Owner,
		&repo.Name,
		&repo.Description,
		&repo.PrimaryLanguage,
		&repo.StarsCount,
		&repo.ForksCount,
		&repo.CreatedAt,
		&repo.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRepositoryNotFound
		}
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	return repo, nil
}

// ByUserID retrieves all repositories for a user, ordered by most recent.
func (s *RepositoryService) ByUserID(ctx context.Context, userID int64) ([]*Repository, error) {
	query := `
		SELECT id, user_id, github_url, owner, name, description, primary_language, stars_count, forks_count, created_at, updated_at
		FROM repositories
		WHERE user_id = $1
		ORDER BY updated_at DESC
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w", err)
	}
	defer rows.Close()

	var repos []*Repository
	for rows.Next() {
		repo := &Repository{}
		err := rows.Scan(
			&repo.ID,
			&repo.UserID,
			&repo.GitHubURL,
			&repo.Owner,
			&repo.Name,
			&repo.Description,
			&repo.PrimaryLanguage,
			&repo.StarsCount,
			&repo.ForksCount,
			&repo.CreatedAt,
			&repo.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan repository: %w", err)
		}
		repos = append(repos, repo)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating repositories: %w", err)
	}

	return repos, nil
}

// ByUserAndURL finds a repository by user ID and GitHub URL.
func (s *RepositoryService) ByUserAndURL(ctx context.Context, userID int64, githubURL string) (*Repository, error) {
	// Normalize URL
	owner, name, err := ParseGitHubURL(githubURL)
	if err != nil {
		return nil, err
	}
	normalizedURL := fmt.Sprintf("https://github.com/%s/%s", owner, name)

	query := `
		SELECT id, user_id, github_url, owner, name, description, primary_language, stars_count, forks_count, created_at, updated_at
		FROM repositories
		WHERE user_id = $1 AND github_url = $2
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	repo := &Repository{}
	err = s.pool.QueryRow(ctx, query, userID, normalizedURL).Scan(
		&repo.ID,
		&repo.UserID,
		&repo.GitHubURL,
		&repo.Owner,
		&repo.Name,
		&repo.Description,
		&repo.PrimaryLanguage,
		&repo.StarsCount,
		&repo.ForksCount,
		&repo.CreatedAt,
		&repo.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRepositoryNotFound
		}
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	return repo, nil
}

// on delete, the associated analysis is also deleted (on cascade)
func (s *RepositoryService) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM repositories WHERE id = $1`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	result, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete repository: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrRepositoryNotFound
	}

	return nil
}

// CountByUser returns the number of repositories for a user.
func (s *RepositoryService) CountByUser(ctx context.Context, userID int64) (int, error) {
	query := `SELECT COUNT(*) FROM repositories WHERE user_id = $1`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	var count int
	err := s.pool.QueryRow(ctx, query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count repositories: %w", err)
	}

	return count, nil
}

// HELPER FUNCS ------------------------------------------------------

// FullName returns the owner/repo format.
func (r *Repository) FullName() string {
	return fmt.Sprintf("%s/%s", r.Owner, r.Name)
}

// CanonicalURL returns the full GitHub URL.
func (r *Repository) CanonicalURL() string {
	return fmt.Sprintf("https://github.com/%s/%s", r.Owner, r.Name)
}

// ShortDescription returns a truncated description for display.
func (r *Repository) ShortDescription(maxLen int) string {
	if r.Description == nil || *r.Description == "" {
		return "No description"
	}
	desc := *r.Description
	if len(desc) <= maxLen {
		return desc
	}
	return desc[:maxLen-3] + "..."
}
