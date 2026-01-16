package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rahul4469/github-analyzer/internal/models"
)

type GitHubService struct {
	baseURL    string
	httpClient *http.Client
}

func NewGitHubService(baseURL string) *GitHubService {
	return &GitHubService{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type GitHubRepository struct {
	Name            string `json:"name"`
	FullName        string `json:"full_name"`
	Description     string `json:"description"`
	Language        string `json:"language"`
	StargazersCount int    `json:"stargazers_count"`
	ForksCount      int    `json:"forks_count"`
	DefaultBranch   string `json:"default_branch"`
	HTMLURL         string `json:"html_url"`
	Private         bool   `json:"private"`
}

type GitHubTreeEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"` // "blob" for file, "tree" for directory
	SHA  string `json:"sha"`
	Size int    `json:"size,omitempty"`
	URL  string `json:"url,omitempty"`
}

// GitHubTree represents the full repository tree.
type GitHubTree struct {
	SHA       string            `json:"sha"`
	URL       string            `json:"url"`
	Tree      []GitHubTreeEntry `json:"tree"`
	Truncated bool              `json:"truncated"`
}

// GitHubContent represents file content from GitHub API.
type GitHubContent struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	SHA         string `json:"sha"`
	Size        int    `json:"size"`
	Type        string `json:"type"`
	Content     string `json:"content"` // Base64 encoded
	Encoding    string `json:"encoding"`
	HTMLURL     string `json:"html_url"`
	DownloadURL string `json:"download_url"`
}

type GitHubError struct {
	Message          string `json:"message"`
	DocumentationURL string `json:"documentation_url"`
}

func (s *GitHubService) GetRepository(ctx context.Context, owner, repo, token string) (*GitHubRepository, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", s.baseURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	s.setHeaders(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch repository: %w", err)
	}
	defer resp.Body.Close()

	if err := s.checkResponse(resp); err != nil {
		return nil, err
	}

	var repository GitHubRepository
	if err := json.NewDecoder(resp.Body).Decode(&repository); err != nil {
		return nil, fmt.Errorf("failed to decode repository: %w", err)
	}

	return &repository, nil
}

func (s *GitHubService) GetRepositoryTree(ctx context.Context, owner, repo, token string) (*GitHubTree, error) {
	// First get the default branch
	repoInfo, err := s.GetRepository(ctx, owner, repo, token)
	if err != nil {
		return nil, err
	}

	// Fetch the tree recursively
	url := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1", s.baseURL, owner, repo, repoInfo.DefaultBranch)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	s.setHeaders(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tree: %w", err)
	}
	defer resp.Body.Close()

	if err := s.checkResponse(resp); err != nil {
		return nil, err
	}

	var tree GitHubTree
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		return nil, fmt.Errorf("failed to decode tree: %w", err)
	}

	return &tree, nil
}

// GetFileContent fetches the content of a single file.
func (s *GitHubService) GetFileContent(ctx context.Context, owner, repo, path, token string) (*GitHubContent, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s", s.baseURL, owner, repo, path)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	s.setHeaders(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch file: %w", err)
	}
	defer resp.Body.Close()

	if err := s.checkResponse(resp); err != nil {
		return nil, err
	}

	var content GitHubContent
	if err := json.NewDecoder(resp.Body).Decode(&content); err != nil {
		return nil, fmt.Errorf("failed to decode content: %w", err)
	}

	return &content, nil
}

func (s *GitHubService) GetREADME(ctx context.Context, owner, repo, token string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/readme", s.baseURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	s.setHeaders(req, token)
	req.Header.Set("Accept", "application/vnd.github.raw")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch README: %w", err)
	}
	defer resp.Body.Close()

	// README might not exist, that's okay
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}

	if err := s.checkResponse(resp); err != nil {
		return "", err
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read README: %w", err)
	}

	return string(content), nil
}

// FileImportance determines how important a file is for analysis.
type FileImportance struct {
	Path     string
	Score    int
	Language string
	Category string // "entry", "config", "source", "test", "docs"
}

// GetRepositoryFiles fetches actual source code from important files.
// This is the key method for Option 2 - Enhanced Analysis.
//
// Strategy:
// 1. Get complete file tree
// 2. Score files by importance
// 3. Fetch top N files (respecting size/token limits)
// 4. Return file contents for AI analysis
func (s *GitHubService) GetRepositoryFiles(ctx context.Context, owner, repo, token string, maxFiles int) ([]models.FileContent, *models.CodeStructure, error) {
	if maxFiles <= 0 {
		maxFiles = 15
	}

	// Get the complete tree
	tree, err := s.GetRepositoryTree(ctx, owner, repo, token)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get repository tree: %w", err)
	}

	// Build code structure
	codeStructure := s.buildCodeStructure(tree)

	// Score and prioritize files
	scoredFiles := s.scoreFiles(tree.Tree)

	// Sort by score (highest first)
	sort.Slice(scoredFiles, func(i, j int) bool {
		return scoredFiles[i].Score > scoredFiles[j].Score
	})

	// Fetch top files (respect size limits)
	var files []models.FileContent
	totalSize := 0
	maxTotalSize := 500000 // ~500KB total to stay within token limits

	for _, sf := range scoredFiles {
		if len(files) >= maxFiles {
			break
		}
		if totalSize >= maxTotalSize {
			break
		}

		// Find the tree entry to get size
		var fileSize int
		for _, entry := range tree.Tree {
			if entry.Path == sf.Path {
				fileSize = entry.Size
				break
			}
		}

		// Skip files that are too large individually
		if fileSize > 100000 { // 100KB per file max
			continue
		}

		// Fetch the file content
		content, err := s.GetFileContent(ctx, owner, repo, sf.Path, token)
		if err != nil {
			// Skip files we can't fetch, continue with others
			continue
		}

		// Decode base64 content
		decoded, err := s.decodeContent(content)
		if err != nil {
			continue
		}

		// Skip binary files
		if isBinaryContent(decoded) {
			continue
		}

		files = append(files, models.FileContent{
			Path:     sf.Path,
			Content:  decoded,
			Language: sf.Language,
			Size:     len(decoded),
		})

		totalSize += len(decoded)
	}

	return files, codeStructure, nil
}

// buildCodeStructure creates a CodeStructure from the tree.
func (s *GitHubService) buildCodeStructure(tree *GitHubTree) *models.CodeStructure {
	structure := &models.CodeStructure{
		Directories:       []string{},
		Files:             []string{},
		LanguageBreakdown: make(map[string]int),
	}

	for _, entry := range tree.Tree {
		if entry.Type == "tree" {
			structure.Directories = append(structure.Directories, entry.Path)
		} else if entry.Type == "blob" {
			structure.Files = append(structure.Files, entry.Path)
			structure.TotalFiles++
			structure.TotalSize += entry.Size

			// Count by language
			lang := detectLanguage(entry.Path)
			if lang != "" {
				structure.LanguageBreakdown[lang]++
			}
		}
	}

	return structure
}

func (s *GitHubService) scoreFiles(entries []GitHubTreeEntry) []FileImportance {
	var scored []FileImportance

	for _, entry := range entries {
		// Skip directories
		if entry.Type != "blob" {
			continue
		}

		// Skip non-code files
		if !isCodeFile(entry.Path) {
			continue
		}

		score, category := calculateFileScore(entry.Path)
		if score > 0 {
			scored = append(scored, FileImportance{
				Path:     entry.Path,
				Score:    score,
				Language: detectLanguage(entry.Path),
				Category: category,
			})
		}
	}

	return scored
}

func calculateFileScore(path string) (int, string) {
	name := filepath.Base(path)
	dir := filepath.Dir(path)
	ext := strings.ToLower(filepath.Ext(path))
	nameLower := strings.ToLower(name)

	score := 0
	category := "source"

	// Entry point files (highest priority)
	entryPoints := []string{"main.go", "main.py", "main.rs", "main.ts", "main.js",
		"app.go", "app.py", "app.ts", "app.js", "index.ts", "index.js",
		"server.go", "server.ts", "server.js", "cmd.go"}
	for _, ep := range entryPoints {
		if nameLower == ep {
			return 100, "entry"
		}
	}

	// Config files (high priority - reveal project structure)
	configFiles := []string{"go.mod", "go.sum", "package.json", "cargo.toml",
		"requirements.txt", "pyproject.toml", "dockerfile", "docker-compose.yml",
		"makefile", ".env.example", "config.yaml", "config.json", "tsconfig.json"}
	for _, cf := range configFiles {
		if nameLower == cf {
			return 90, "config"
		}
	}

	// Important directories
	importantDirs := map[string]int{
		"cmd":         85,
		"internal":    80,
		"pkg":         75,
		"src":         75,
		"lib":         70,
		"api":         80,
		"handlers":    85,
		"controllers": 85,
		"services":    80,
		"models":      80,
		"routes":      75,
		"middleware":  75,
		"utils":       60,
		"helpers":     60,
		"core":        80,
	}

	for importantDir, dirScore := range importantDirs {
		if strings.Contains(strings.ToLower(dir), importantDir) {
			score = dirScore
			break
		}
	}

	// Test files (lower priority but still useful)
	if strings.Contains(nameLower, "_test.") || strings.Contains(nameLower, ".test.") ||
		strings.Contains(nameLower, ".spec.") || strings.HasPrefix(nameLower, "test_") {
		return max(score, 40), "test"
	}

	// Boost by file extension (code files)
	extBoost := map[string]int{
		".go":   20,
		".py":   20,
		".rs":   20,
		".ts":   18,
		".js":   15,
		".tsx":  18,
		".jsx":  15,
		".java": 18,
		".c":    15,
		".cpp":  15,
		".h":    10,
		".rb":   15,
		".php":  15,
		".sql":  12,
	}

	if boost, ok := extBoost[ext]; ok {
		score += boost
	}

	// Penalize deeply nested files
	depth := strings.Count(path, "/")
	if depth > 4 {
		score -= (depth - 4) * 5
	}

	// Penalize vendor/node_modules
	if strings.Contains(path, "vendor/") || strings.Contains(path, "node_modules/") {
		return 0, ""
	}

	// Minimum score for code files
	if score < 10 && isCodeFile(path) {
		score = 10
	}

	return score, category
}

// isCodeFile checks if a file is a source code file.
func isCodeFile(path string) bool {
	codeExtensions := map[string]bool{
		".go": true, ".py": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
		".rs": true, ".java": true, ".c": true, ".cpp": true, ".h": true, ".hpp": true,
		".rb": true, ".php": true, ".swift": true, ".kt": true, ".scala": true,
		".cs": true, ".vb": true, ".fs": true, ".clj": true, ".ex": true, ".exs": true,
		".hs": true, ".ml": true, ".sql": true, ".sh": true, ".bash": true,
		".yaml": true, ".yml": true, ".json": true, ".toml": true, ".xml": true,
	}

	ext := strings.ToLower(filepath.Ext(path))
	return codeExtensions[ext]
}

// detectLanguage returns the programming language based on file extension.
func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	languages := map[string]string{
		".go":    "Go",
		".py":    "Python",
		".js":    "JavaScript",
		".ts":    "TypeScript",
		".jsx":   "React",
		".tsx":   "React TypeScript",
		".rs":    "Rust",
		".java":  "Java",
		".c":     "C",
		".cpp":   "C++",
		".h":     "C/C++ Header",
		".rb":    "Ruby",
		".php":   "PHP",
		".swift": "Swift",
		".kt":    "Kotlin",
		".sql":   "SQL",
		".sh":    "Shell",
		".yaml":  "YAML",
		".json":  "JSON",
	}

	if lang, ok := languages[ext]; ok {
		return lang
	}
	return ""
}

// decodeContent decodes base64 content from GitHub API.
func (s *GitHubService) decodeContent(content *GitHubContent) (string, error) {
	if content.Encoding != "base64" {
		return content.Content, nil
	}

	// GitHub returns base64 with newlines, need to remove them
	cleaned := strings.ReplaceAll(content.Content, "\n", "")
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	return string(decoded), nil
}

// isBinaryContent checks if content appears to be binary.
func isBinaryContent(content string) bool {
	// Check for null bytes (common in binary files)
	if strings.Contains(content, "\x00") {
		return true
	}

	// Check ratio of printable characters
	nonPrintable := 0
	for _, r := range content {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			nonPrintable++
		}
	}

	// If more than 10% non-printable, likely binary
	if len(content) > 0 && float64(nonPrintable)/float64(len(content)) > 0.1 {
		return true
	}

	return false
}

func (s *GitHubService) setHeaders(req *http.Request, token string) {
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "GitHub-Analyzer/1.0")

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// checkResponse checks for API errors in the response.
func (s *GitHubService) checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)

	var ghErr GitHubError
	if err := json.Unmarshal(body, &ghErr); err == nil && ghErr.Message != "" {
		return fmt.Errorf("GitHub API error (%d): %s", resp.StatusCode, ghErr.Message)
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("GitHub authentication failed: invalid or expired token")
	case http.StatusForbidden:
		return fmt.Errorf("GitHub API rate limit exceeded or access forbidden")
	case http.StatusNotFound:
		return fmt.Errorf("repository not found or not accessible")
	default:
		return fmt.Errorf("GitHub API error: %d - %s", resp.StatusCode, string(body))
	}
}

func (s *GitHubService) GetRateLimit(ctx context.Context, token string) (remaining, limit int, resetTime time.Time, err error) {
	url := fmt.Sprintf("%s/rate_limit", s.baseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, 0, time.Time{}, err
	}

	s.setHeaders(req, token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, 0, time.Time{}, err
	}
	defer resp.Body.Close()

	var result struct {
		Resources struct {
			Core struct {
				Limit     int   `json:"limit"`
				Remaining int   `json:"remaining"`
				Reset     int64 `json:"reset"`
			} `json:"core"`
		} `json:"resources"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, 0, time.Time{}, err
	}

	return result.Resources.Core.Remaining,
		result.Resources.Core.Limit,
		time.Unix(result.Resources.Core.Reset, 0),
		nil
}

// max returns the larger of two integers.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
