package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v75/github"
	"golang.org/x/oauth2"
)

// GitHubFetcher handles all Github API interactions
type GitHubFetcher struct {
	Token  string
	Client *github.Client
}

// gf Constructor creates a new GitHub API fetcher with user's token
func NewGitHubFetcher(token string) *GitHubFetcher {
	ctx := context.Background()

	// 1.Create OAuth2 token source
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)

	// 2.Create Oauth2 HTTP client
	tc := oauth2.NewClient(ctx, ts)

	// 3.Create GitHub API client
	client := github.NewClient(tc)

	return &GitHubFetcher{
		Token:  token,
		Client: client,
	}
}

// RepositoryData holds all fetched repo informations
// all Fields are of inbuilt github library - type
type RepositoryData struct {
	Repository   *github.Repository
	Languages    map[string]int
	Contributors []*github.Contributor
	Commits      []*github.RepositoryCommit
	CodeFiles    []FileContent
	Topics       []string
}

// FileContent represents a code file with content
type FileContent struct {
	Path    string
	Content string
	Size    int
}

// Fetches complete repository data by owner & repo name using the github api
func (gf *GitHubFetcher) FetchRepository(ctx context.Context, ownerRepo string) (*RepositoryData, error) {
	// Parse owner/repo format
	parts := strings.Split(ownerRepo, "/") // Split() returns a slice of string
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repository format: expected owner/repo got %s", ownerRepo)
	}

	owner, repo := parts[0], parts[1]

	// Fetch repository details
	repoData, _, err := gf.Client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch repository: %w", err)
	}

	// Fetch languages (format - string : %)
	languages, _, err := gf.Client.Repositories.ListLanguages(ctx, owner, repo)
	if err != nil {
		// Languages are optional, don't need to show fail at this step
		languages = make(map[string]int)
	}

	// Fetch top contributors
	contributors, _, err := gf.Client.Repositories.ListContributors(ctx, owner, repo,
		&github.ListContributorsOptions{
			ListOptions: github.ListOptions{PerPage: 10}, // uses opts for pagination
		})
	if err != nil {
		// contributors are optional too
		contributors = nil
	}

	// Fetch recent commits
	commits, _, err := gf.Client.Repositories.ListCommits(ctx, owner, repo, &github.CommitsListOptions{
		ListOptions: github.ListOptions{PerPage: 20},
	})
	if err != nil {
		// commits are optional too
		commits = nil
	}

	// Fetch important code files
	codeFiles, _ := gf.fetchCodeFiles(ctx, owner, repo)

	// Fetch repository topics
	topics, _, _ := gf.Client.Repositories.ListAllTopics(ctx, owner, repo)

	return &RepositoryData{
		Repository:   repoData,
		Languages:    languages,
		Contributors: contributors,
		Commits:      commits,
		CodeFiles:    codeFiles,
		Topics:       topics,
	}, nil
}

// fetchCodeFiles fetches imp code files from repository
func (gf *GitHubFetcher) fetchCodeFiles(ctx context.Context, owner, repo string) ([]FileContent, error) {
	// Files to try to fetch
	importantFiles := []string{
		"README.md",
		"go.mod",
		"go.sum",
		"main.go",
		"package.json",
		"package-lock.json",
		"requirements.txt",
		"setup.py",
		"Dockerfile",
		".gitignore",
		"LICENSE",
		"CONTRIBUTING.md",
	}

	var files []FileContent

	for _, filename := range importantFiles {
		fileContent, _, _, err := gf.Client.Repositories.GetContents(ctx, owner, repo, filename, nil)
		if err != nil {
			// File doesn't exist, skip it
			continue
		}

		if fileContent != nil && fileContent.Content != nil {
			files = append(files, FileContent{
				Path:    filename,
				Content: *fileContent.Content,
				Size:    *fileContent.Size,
			})
		}
	}

	return files, nil
}

// IsValidToken validates GitHub token "Format"
func (gf *GitHubFetcher) IsValidToken(token string) bool {
	// GitHub tokens have specific prefixes
	// ghp_ = Personal Access Token (classic)
	// github_pat_ = Personal Access Token (fine-grained)
	return (strings.HasPrefix(token, "ghp_") ||
		strings.HasPrefix(token, "github_pat_") &&
			len(token) > 30)
}

// GetRepositoryStats returns summary statistics
func (gf *GitHubFetcher) GetRepositoryStats(repoData *RepositoryData) map[string]interface{} {
	if repoData == nil || repoData.Repository == nil {
		return nil
	}

	repo := repoData.Repository

	return map[string]interface{}{
		"name":             repo.GetName(),
		"full_name":        repo.GetFullName(),
		"description":      repo.GetDescription(),
		"url":              repo.GetHTMLURL(),
		"stars":            repo.GetStargazersCount(),
		"forks":            repo.GetForksCount(),
		"watchers":         repo.GetWatchersCount(),
		"open_issues":      repo.GetOpenIssuesCount(),
		"language":         repo.GetLanguage(),
		"size":             repo.GetSize(),
		"created_at":       repo.GetCreatedAt(),
		"updated_at":       repo.GetUpdatedAt(),
		"pushed_at":        repo.GetPushedAt(),
		"contributors":     len(repoData.Contributors),
		"recent_commits":   len(repoData.Commits),
		"languages_count":  len(repoData.Languages),
		"code_files_count": len(repoData.CodeFiles),
	}
}
