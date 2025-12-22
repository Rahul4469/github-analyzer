package models

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Repository represents a GitHub repository
type Repository struct {
	ID              int64         `json:"id"`
	UserID          int           `json:"user_id"`
	FullName        string        `json:"full_name"` // owner/repo
	Owner           string        `json:"owner"`
	Name            string        `json:"name"`
	URL             string        `json:"url"`
	Description     string        `json:"description"`
	StarsCount      int           `json:"stars_count"`
	ForksCount      int           `json:"forks_count"`
	WatchersCount   int           `json:"watchers_count"`
	OpenIssuesCount int           `json:"open_issues_count"`
	PrimaryLanguage string        `json:"primary_language"`
	Languages       []Language    `json:"languages,omitempty"`
	Contributors    []Contributor `json:"contributors,omitempty"`
	Commits         []Commit      `json:"commits,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

// Language represents programming language breakdown
type Language struct {
	ID           int       `json:"id"`
	RepositoryID int64     `json:"repository_id"`
	Language     string    `json:"language"`
	Percentage   float64   `json:"percentage"`
	BytesOfCode  int64     `json:"bytes_of_code"`
	CreatedAt    time.Time `json:"created_at"`
}

// Contributor represents a repository contributor
type Contributor struct {
	ID           int       `json:"id"`
	RepositoryID int64     `json:"repository_id"`
	Name         string    `json:"name"`
	Email        string    `json:"email,omitempty"`
	AvatarURL    string    `json:"avatar_url,omitempty"`
	ProfileURL   string    `json:"profile_url,omitempty"`
	Commits      int       `json:"commits"`
	CreatedAt    time.Time `json:"created_at"`
}

// Commit represents a GitHub commit
type Commit struct {
	ID            int       `json:"id"`
	RepositoryID  int64     `json:"repository_id"`
	CommitHash    string    `json:"commit_hash"`
	CommitMessage string    `json:"commit_message"`
	AuthorName    string    `json:"author_name"`
	AuthorEmail   string    `json:"author_email"`
	Additions     int       `json:"additions"`
	Deletions     int       `json:"deletions"`
	ChangedFiles  int       `json:"changed_files"`
	CommitDate    time.Time `json:"commit_date"`
	CreatedAt     time.Time `json:"created_at"`
}

type RepositoryService struct {
	DB *sql.DB
}

func NewRepositoryService(db *sql.DB) *RepositoryService {
	return &RepositoryService{DB: db}
}

// Create creates a new repository
func (rs *RepositoryService) Create(ctx context.Context, repo *Repository) (*Repository, error) {
	if repo.UserID == 0 || repo.FullName == "" {
		return nil, fmt.Errorf("user_id and full_name are required")
	}

	err := rs.DB.QueryRowContext(
		ctx,
		`INSERT INTO repositories (user_id, full_name, owner, name, url, description,
		                           stars_count, forks_count, watchers_count,
		                           open_issues_count, primary_language)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 RETURNING id, created_at, updated_at`,
		repo.UserID, repo.FullName, repo.Owner, repo.Name, repo.URL,
		repo.Description, repo.StarsCount, repo.ForksCount, repo.WatchersCount,
		repo.OpenIssuesCount, repo.PrimaryLanguage,
	).Scan(&repo.ID, &repo.CreatedAt, &repo.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create repository: %w", err)
	}

	return repo, nil
}

// GetByID retrieves a repository by ID
func (rs *RepositoryService) GetByID(ctx context.Context, id int64) (*Repository, error) {
	var repo Repository

	err := rs.DB.QueryRowContext(
		ctx,
		`SELECT id, user_id, full_name, owner, name, url, description,
		        stars_count, forks_count, watchers_count, open_issues_count,
		        primary_language, created_at, updated_at
		 FROM repositories WHERE id = $1`,
		id,
	).Scan(
		&repo.ID, &repo.UserID, &repo.FullName, &repo.Owner, &repo.Name,
		&repo.URL, &repo.Description, &repo.StarsCount, &repo.ForksCount,
		&repo.WatchersCount, &repo.OpenIssuesCount, &repo.PrimaryLanguage,
		&repo.CreatedAt, &repo.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("repository not found")
		}
		return nil, fmt.Errorf("failed to fetch repository: %w", err)
	}

	return &repo, nil
}

// GetByFullName retrieves a repository by full name for a user
func (rs *RepositoryService) GetByFullName(ctx context.Context, userID int, fullName string) (*Repository, error) {
	var repo Repository

	err := rs.DB.QueryRowContext(
		ctx,
		`SELECT id, user_id, full_name, owner, name, url, description,
		        stars_count, forks_count, watchers_count, open_issues_count,
		        primary_language, created_at, updated_at
		 FROM repositories WHERE user_id = $1 AND full_name = $2`,
		userID, fullName,
	).Scan(
		&repo.ID, &repo.UserID, &repo.FullName, &repo.Owner, &repo.Name,
		&repo.URL, &repo.Description, &repo.StarsCount, &repo.ForksCount,
		&repo.WatchersCount, &repo.OpenIssuesCount, &repo.PrimaryLanguage,
		&repo.CreatedAt, &repo.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("repository not found")
		}
		return nil, fmt.Errorf("failed to fetch repository: %w", err)
	}

	return &repo, nil
}

// GetByUserID retrieves all repositories for a user
func (rs *RepositoryService) GetByUserID(ctx context.Context, userID int) ([]Repository, error) {
	rows, err := rs.DB.QueryContext(
		ctx,
		`SELECT id, user_id, full_name, owner, name, url, description,
		        stars_count, forks_count, watchers_count, open_issues_count,
		        primary_language, created_at, updated_at
		 FROM repositories WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to fetch repositories: %w", err)
	}
	defer rows.Close()

	var repos []Repository

	for rows.Next() {
		var repo Repository

		err := rows.Scan(
			&repo.ID, &repo.UserID, &repo.FullName, &repo.Owner, &repo.Name,
			&repo.URL, &repo.Description, &repo.StarsCount, &repo.ForksCount,
			&repo.WatchersCount, &repo.OpenIssuesCount, &repo.PrimaryLanguage,
			&repo.CreatedAt, &repo.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan repository: %w", err)
		}

		repos = append(repos, repo)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return repos, nil
}

// Update updates an existing repository
func (rs *RepositoryService) Update(ctx context.Context, repo *Repository) error {
	_, err := rs.DB.ExecContext(
		ctx,
		`UPDATE repositories SET full_name = $1, owner = $2, name = $3,
		                        url = $4, description = $5, stars_count = $6,
		                        forks_count = $7, watchers_count = $8,
		                        open_issues_count = $9, primary_language = $10,
		                        updated_at = NOW()
		 WHERE id = $11`,
		repo.FullName, repo.Owner, repo.Name, repo.URL, repo.Description,
		repo.StarsCount, repo.ForksCount, repo.WatchersCount,
		repo.OpenIssuesCount, repo.PrimaryLanguage, repo.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update repository: %w", err)
	}

	return nil
}

// Delete deletes a repository
func (rs *RepositoryService) Delete(ctx context.Context, id int64) error {
	_, err := rs.DB.ExecContext(
		ctx,
		`DELETE FROM repositories WHERE id = $1`,
		id,
	)

	return err
}
