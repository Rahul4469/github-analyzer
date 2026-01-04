package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rahul4469/github-analyzer/internal/models"
)

// PerplexityService handles AI-powered code analysis.
type PerplexityService struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewPerplexityService creates a new Perplexity AI client.
func NewPerplexityService(apiKey, model string) *PerplexityService {
	return &PerplexityService{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // AI responses can take time
		},
	}
}

// AnalysisInput contains all data needed for AI analysis.
type AnalysisInput struct {
	RepoName        string
	RepoOwner       string
	Description     string
	PrimaryLanguage string
	README          string
	CodeStructure   *models.CodeStructure
	CodeFiles       []models.FileContent
}

// AnalysisResult contains the parsed AI analysis.
type AnalysisResult struct {
	RawAnalysis string
	Summary     *models.AnalysisSummary
	Issues      []models.Issue
	TokensUsed  int
}

// PerplexityRequest represents the API request body.
type PerplexityRequest struct {
	Model    string              `json:"model"`
	Messages []PerplexityMessage `json:"messages"`
}

// PerplexityMessage represents a message in the conversation.
type PerplexityMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// PerplexityResponse represents the API response.
type PerplexityResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Created int64  `json:"created"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// Analyze sends the repository data to Perplexity AI for analysis.
func (s *PerplexityService) Analyze(ctx context.Context, input AnalysisInput) (*AnalysisResult, error) {
	prompt := s.buildPrompt(input)

	// Build the request to be sent to ai
	request := PerplexityRequest{
		Model: s.model,
		Messages: []PerplexityMessage{
			{
				Role:    "system",
				Content: s.getSystemPrompt(),
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.perplexity.ai/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Send the request and receive response using http.Do()
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Perplexity API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Perplexity API error (%d): %s", resp.StatusCode, string(body))
	}

	var response PerplexityResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("no response from Perplexity AI")
	}

	rawAnalysis := response.Choices[0].Message.Content

	// Parse the structured response
	issues := s.parseIssues(rawAnalysis)
	summary := s.buildSummary(issues, rawAnalysis)

	return &AnalysisResult{
		RawAnalysis: rawAnalysis,
		Summary:     summary,
		Issues:      issues,
		TokensUsed:  response.Usage.TotalTokens,
	}, nil
}

// getSystemPrompt returns the system prompt for the AI.
func (s *PerplexityService) getSystemPrompt() string {
	return `You are an expert code reviewer and software architect. Your task is to analyze code repositories and identify:

1. **Bugs & Errors**: Logic errors, potential crashes, unhandled edge cases, null pointer issues
2. **Security Vulnerabilities**: SQL injection, XSS, authentication flaws, secrets exposure, input validation issues
3. **Performance Issues**: N+1 queries, memory leaks, inefficient algorithms, unnecessary allocations
4. **Code Quality**: Poor error handling, missing validation, code smells, anti-patterns
5. **Best Practice Violations**: Naming conventions, code organization, documentation gaps

For each issue found, provide:
- Severity: HIGH, MEDIUM, LOW, or INFO
- Category: bug, security, performance, quality, or style
- File and line number if identifiable
- Clear description of the problem
- Specific suggestion for fixing it

Format your response with a structured ISSUES section using this exact format:

## ISSUES

[HIGH/security] Title of the issue
File: path/to/file.go:123
Description: Detailed description of what's wrong
Suggestion: How to fix it

[MEDIUM/bug] Another issue title
File: path/to/file.go:45
Description: What's the problem
Suggestion: How to fix it

Also provide:
- An OVERVIEW section with general assessment
- A SUMMARY section with counts by severity
- A RECOMMENDATIONS section with top priorities

Be thorough but focus on real, actionable issues rather than style nitpicks.`
}

// buildPrompt constructs the analysis prompt with actual code.
func (s *PerplexityService) buildPrompt(input AnalysisInput) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("# Repository Analysis: %s/%s\n\n", input.RepoOwner, input.RepoName))

	// Repository info
	prompt.WriteString("## Repository Information\n")
	prompt.WriteString(fmt.Sprintf("- **Name**: %s\n", input.RepoName))
	prompt.WriteString(fmt.Sprintf("- **Primary Language**: %s\n", input.PrimaryLanguage))
	if input.Description != "" {
		prompt.WriteString(fmt.Sprintf("- **Description**: %s\n", input.Description))
	}
	prompt.WriteString("\n")

	// Code structure overview
	if input.CodeStructure != nil {
		prompt.WriteString("## Project Structure\n")
		prompt.WriteString(fmt.Sprintf("- **Total Files**: %d\n", input.CodeStructure.TotalFiles))
		prompt.WriteString(fmt.Sprintf("- **Total Size**: %d bytes\n", input.CodeStructure.TotalSize))

		if len(input.CodeStructure.LanguageBreakdown) > 0 {
			prompt.WriteString("- **Languages**:\n")
			for lang, count := range input.CodeStructure.LanguageBreakdown {
				prompt.WriteString(fmt.Sprintf("  - %s: %d files\n", lang, count))
			}
		}

		// List key directories
		if len(input.CodeStructure.Directories) > 0 {
			prompt.WriteString("- **Key Directories**: ")
			dirs := filterImportantDirs(input.CodeStructure.Directories)
			if len(dirs) > 10 {
				dirs = dirs[:10]
			}
			prompt.WriteString(strings.Join(dirs, ", "))
			prompt.WriteString("\n")
		}
		prompt.WriteString("\n")
	}

	// README (truncated if too long)
	if input.README != "" {
		prompt.WriteString("## README\n")
		readme := input.README
		if len(readme) > 2000 {
			readme = readme[:2000] + "\n... (truncated)"
		}
		prompt.WriteString("```\n")
		prompt.WriteString(readme)
		prompt.WriteString("\n```\n\n")
	}

	// Actual code files - THE KEY PART
	if len(input.CodeFiles) > 0 {
		prompt.WriteString("## Source Code Files\n\n")
		prompt.WriteString("Analyze the following source code files for bugs, security issues, and improvements:\n\n")

		for _, file := range input.CodeFiles {
			prompt.WriteString(fmt.Sprintf("### %s\n", file.Path))
			prompt.WriteString(fmt.Sprintf("**Language**: %s | **Size**: %d bytes\n", file.Language, file.Size))
			prompt.WriteString("```" + getLanguageTag(file.Language) + "\n")

			// Truncate very long files
			content := file.Content
			if len(content) > 15000 {
				content = content[:15000] + "\n// ... (file truncated for analysis)"
			}
			prompt.WriteString(content)
			prompt.WriteString("\n```\n\n")
		}
	}

	// Analysis request
	prompt.WriteString("---\n\n")
	prompt.WriteString("## Analysis Request\n\n")
	prompt.WriteString("Please analyze this codebase thoroughly and provide:\n\n")
	prompt.WriteString("1. **OVERVIEW**: General assessment of code quality, architecture, and patterns used\n")
	prompt.WriteString("2. **ISSUES**: Specific bugs, security vulnerabilities, and problems found (use the format specified)\n")
	prompt.WriteString("3. **SUMMARY**: Count of issues by severity (HIGH/MEDIUM/LOW/INFO)\n")
	prompt.WriteString("4. **RECOMMENDATIONS**: Top 3-5 priority improvements\n\n")
	prompt.WriteString("Focus on actionable, specific issues with file paths and line numbers where possible.\n")

	return prompt.String()
}

// parseIssues extracts structured issues from the AI response.
func (s *PerplexityService) parseIssues(response string) []models.Issue {
	var issues []models.Issue

	// Pattern to match issues in format: [SEVERITY/category] Title
	// Followed by File:, Description:, Suggestion:
	issuePattern := regexp.MustCompile(`\[(HIGH|MEDIUM|LOW|INFO)/(bug|security|performance|quality|style)\]\s*(.+?)(?:\n|$)`)
	filePattern := regexp.MustCompile(`(?i)File:\s*([^\n:]+)(?::(\d+))?`)
	descPattern := regexp.MustCompile(`(?i)Description:\s*(.+?)(?:\n(?:Suggestion:|File:|\[)|$)`)
	suggPattern := regexp.MustCompile(`(?i)Suggestion:\s*(.+?)(?:\n\n|\n\[|$)`)

	// Find the ISSUES section
	issuesSection := response
	if idx := strings.Index(strings.ToUpper(response), "## ISSUES"); idx != -1 {
		issuesSection = response[idx:]
		// Find the end of the issues section
		if endIdx := strings.Index(issuesSection[10:], "##"); endIdx != -1 {
			issuesSection = issuesSection[:endIdx+10]
		}
	}

	// Split by issue markers
	parts := issuePattern.FindAllStringSubmatchIndex(issuesSection, -1)

	for i, loc := range parts {
		if len(loc) < 8 {
			continue
		}

		severity := strings.ToUpper(issuesSection[loc[2]:loc[3]])
		category := strings.ToLower(issuesSection[loc[4]:loc[5]])
		title := strings.TrimSpace(issuesSection[loc[6]:loc[7]])

		// Find the content between this issue and the next
		endPos := len(issuesSection)
		if i+1 < len(parts) {
			endPos = parts[i+1][0]
		}
		issueContent := issuesSection[loc[1]:endPos]

		issue := models.Issue{
			Severity: severity,
			Category: category,
			Title:    title,
		}

		// Extract file and line
		if fileMatch := filePattern.FindStringSubmatch(issueContent); fileMatch != nil {
			issue.File = strings.TrimSpace(fileMatch[1])
			if len(fileMatch) > 2 && fileMatch[2] != "" {
				if line, err := strconv.Atoi(fileMatch[2]); err == nil {
					issue.Line = line
				}
			}
		}

		// Extract description
		if descMatch := descPattern.FindStringSubmatch(issueContent); descMatch != nil {
			issue.Description = strings.TrimSpace(descMatch[1])
		}

		// Extract suggestion
		if suggMatch := suggPattern.FindStringSubmatch(issueContent); suggMatch != nil {
			issue.Suggestion = strings.TrimSpace(suggMatch[1])
		}

		// Only add if we have meaningful content
		if issue.Title != "" && (issue.Description != "" || issue.Suggestion != "") {
			issues = append(issues, issue)
		}
	}

	// If structured parsing didn't find issues, try a simpler approach
	if len(issues) == 0 {
		issues = s.parseIssuesSimple(response)
	}

	return issues
}

// parseIssuesSimple is a fallback parser for less structured responses.
func (s *PerplexityService) parseIssuesSimple(response string) []models.Issue {
	var issues []models.Issue

	// Look for severity indicators
	severityPatterns := []struct {
		pattern  *regexp.Regexp
		severity string
	}{
		{regexp.MustCompile(`(?i)\*\*?(critical|high severity|high risk|severe)\*\*?[:\s]+(.+?)(?:\n|$)`), "HIGH"},
		{regexp.MustCompile(`(?i)\*\*?(warning|medium severity|medium risk|moderate)\*\*?[:\s]+(.+?)(?:\n|$)`), "MEDIUM"},
		{regexp.MustCompile(`(?i)\*\*?(minor|low severity|low risk|suggestion)\*\*?[:\s]+(.+?)(?:\n|$)`), "LOW"},
	}

	for _, sp := range severityPatterns {
		matches := sp.pattern.FindAllStringSubmatch(response, -1)
		for _, match := range matches {
			if len(match) >= 3 {
				issues = append(issues, models.Issue{
					Severity:    sp.severity,
					Category:    "quality",
					Title:       strings.TrimSpace(match[2]),
					Description: strings.TrimSpace(match[2]),
				})
			}
		}
	}

	// Look for bullet points with issue-like content
	bulletPattern := regexp.MustCompile(`(?m)^[\-\*]\s+(.+(?:bug|issue|error|vulnerability|problem|missing|should|needs to).+)$`)
	bullets := bulletPattern.FindAllStringSubmatch(response, -1)
	for _, match := range bullets {
		if len(match) >= 2 {
			// Determine severity from content
			severity := "LOW"
			content := strings.ToLower(match[1])
			if strings.Contains(content, "security") || strings.Contains(content, "vulnerability") ||
				strings.Contains(content, "injection") || strings.Contains(content, "critical") {
				severity = "HIGH"
			} else if strings.Contains(content, "bug") || strings.Contains(content, "error") ||
				strings.Contains(content, "crash") || strings.Contains(content, "fail") {
				severity = "MEDIUM"
			}

			// Determine category
			category := "quality"
			if strings.Contains(content, "security") || strings.Contains(content, "auth") ||
				strings.Contains(content, "injection") || strings.Contains(content, "xss") {
				category = "security"
			} else if strings.Contains(content, "performance") || strings.Contains(content, "slow") ||
				strings.Contains(content, "memory") || strings.Contains(content, "n+1") {
				category = "performance"
			} else if strings.Contains(content, "bug") || strings.Contains(content, "error") {
				category = "bug"
			}

			issues = append(issues, models.Issue{
				Severity:    severity,
				Category:    category,
				Title:       truncateString(match[1], 100),
				Description: match[1],
			})
		}
	}

	return issues
}

// buildSummary creates an AnalysisSummary from issues and raw analysis.
func (s *PerplexityService) buildSummary(issues []models.Issue, rawAnalysis string) *models.AnalysisSummary {
	summary := &models.AnalysisSummary{
		TotalIssues:      len(issues),
		IssuesBySeverity: make(map[string]int),
		IssuesByCategory: make(map[string]int),
		KeyFindings:      []string{},
	}

	// Count by severity and category
	for _, issue := range issues {
		summary.IssuesBySeverity[issue.Severity]++
		summary.IssuesByCategory[issue.Category]++
	}

	// Calculate overall score (0-100)
	// Start at 100, deduct points for issues
	score := 100
	score -= summary.IssuesBySeverity["HIGH"] * 10
	score -= summary.IssuesBySeverity["MEDIUM"] * 5
	score -= summary.IssuesBySeverity["LOW"] * 3
	score -= summary.IssuesBySeverity["INFO"] * 1
	if score < 0 {
		score = 0
	}
	summary.OverallScore = score

	// Extract key findings (top 5 high/medium issues)
	for _, issue := range issues {
		if len(summary.KeyFindings) >= 5 {
			break
		}
		if issue.Severity == "HIGH" || issue.Severity == "MEDIUM" {
			summary.KeyFindings = append(summary.KeyFindings, issue.Title)
		}
	}

	return summary
}

// Helper functions

func filterImportantDirs(dirs []string) []string {
	important := []string{}
	importantNames := map[string]bool{
		"cmd": true, "internal": true, "pkg": true, "src": true, "lib": true,
		"api": true, "handlers": true, "controllers": true, "services": true,
		"models": true, "routes": true, "middleware": true, "config": true,
		"utils": true, "helpers": true, "core": true, "app": true, "server": true,
	}

	for _, dir := range dirs {
		parts := strings.Split(dir, "/")
		if len(parts) > 0 && importantNames[parts[0]] {
			important = append(important, dir)
		}
	}

	return important
}

func getLanguageTag(language string) string {
	tags := map[string]string{
		"Go":               "go",
		"Python":           "python",
		"JavaScript":       "javascript",
		"TypeScript":       "typescript",
		"React":            "jsx",
		"React TypeScript": "tsx",
		"Rust":             "rust",
		"Java":             "java",
		"C":                "c",
		"C++":              "cpp",
		"Ruby":             "ruby",
		"PHP":              "php",
		"SQL":              "sql",
		"Shell":            "bash",
		"YAML":             "yaml",
		"JSON":             "json",
	}

	if tag, ok := tags[language]; ok {
		return tag
	}
	return ""
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
