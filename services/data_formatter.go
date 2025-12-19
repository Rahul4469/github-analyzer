package services

import (
	"fmt"
	"sort"
	"strings"
)

// DataFormatter provides utilities for formatting data for analysis
// The data received from github_fetcher and to be sent for analysis
type DataFormatter struct{}

// NewDataFormatter constructor creates a new data formatter
func NewDataFormatter() *DataFormatter {
	return &DataFormatter{}
}

// FormatRepositoryDataForAnalysis formats repo data as a string for AI analysis
func (df *DataFormatter) FormatRepositoryDataForAnalysis(repoData *RepositoryData) string {
	if repoData == nil || repoData.Repository == nil {
		return ""
	}

	var output strings.Builder

	// Repository metadata
	repo := repoData.Repository // of standard github library repository type
	output.WriteString(fmt.Sprintf("Repository: %s\n", repo.GetFullName()))
	output.WriteString(fmt.Sprintf("URL: %s\n", repo.GetHTMLURL()))
	output.WriteString(fmt.Sprintf("Description: %s\n", repo.GetDescription()))
	output.WriteString(fmt.Sprintf("Language: %s\n", repo.GetLanguage()))
	output.WriteString("\n")

	// Statistics
	output.WriteString(fmt.Sprintf("Statistics:\n"))
	output.WriteString(fmt.Sprintf("  Stars: %d\n", repo.GetStargazersCount()))
	output.WriteString(fmt.Sprintf("  Forks: %d\n", repo.GetForksCount()))
	output.WriteString(fmt.Sprintf("  Watchers: %d\n", repo.GetWatchersCount()))
	output.WriteString(fmt.Sprintf("  Open Issues: %d\n", repo.GetOpenIssuesCount()))
	output.WriteString(fmt.Sprintf("  Repository Size: %d KB\n", repo.GetSize()))
	output.WriteString("\n")

	// Language breakdown
	if len(repoData.Languages) > 0 {
		output.WriteString("Programming Languages:\n")

		// Sort languages by bytes count
		type langBytes struct {
			name  string
			bytes int
		}

		var langs []langBytes
		for name, bytes := range repoData.Languages {
			langs = append(langs, langBytes{name, bytes})
		}
		sort.Slice(langs, func(i, j int) bool {
			return langs[i].bytes > langs[j].bytes
		})

		for _, lang := range langs {
			output.WriteString(fmt.Sprintf("  %s: %d bytes\n", lang.name, lang.bytes))
		}
		output.WriteString("\n")
	}

	// Top Contributors
	if len(repoData.Contributors) > 0 {
		output.WriteString(fmt.Sprintf("Top %d Contributors:\n", len(repoData.Contributors)))
		for i, contributor := range repoData.Contributors {
			if i >= 5 { // limit to top 5
				break
			}
			output.WriteString(fmt.Sprintf(" %d. %s - %d commits\n",
				i+1, contributor.GetLogin(), contributor.GetContributions()))
		}
		output.WriteString("\n")
	}

	// Recent commits
	if len(repoData.Commits) > 0 {
		output.WriteString(fmt.Sprintf("Recent Commits (last %d):\n", len(repoData.Commits)))
		for i, commit := range repoData.Commits {
			if i >= 5 { // Limit to 5
				break
			}
			msg := commit.Commit.GetMessage()
			if len(msg) > 60 {
				msg = msg[:60] + "..."
			}
			output.WriteString(fmt.Sprintf("  %d. %s - %s\n",
				i+1, commit.GetSHA()[:7], msg))
		}
		output.WriteString("\n")
	}

	// Code files content
	if len(repoData.CodeFiles) > 0 {
		output.WriteString("Important Code Files:\n")
		for _, file := range repoData.CodeFiles {
			output.WriteString(fmt.Sprintf("\n--- File: %s (%d bytes) ---\n", file.Path, file.Size))
			// Limit content size for API
			content := file.Content
			if len(content) > 1000 {
				content = content[:1000] + "\n... [truncated] ..."
			}
			output.WriteString(content)
			output.WriteString("\n")
		}
	}

	// Topics
	if len(repoData.Topics) > 0 {
		output.WriteString("\nTopics:\n")
		for _, topic := range repoData.Topics {
			output.WriteString(fmt.Sprintf("  - %s\n", topic))
		}
	}

	return output.String()
}

// SummarizeAnalysis creates a summary from raw analysis
// data received back from AI API
func (df *DataFormatter) SummarizeAnalysis(rawAnalysis string) string {
	// Extract first few lines as summary
	lines := strings.Split(rawAnalysis, "\n")

	var summary strings.Builder
	lineCount := 0

	for _, line := range lines {
		if lineCount >= 5 {
			break
		}
		if strings.TrimSpace(line) != "" {
			summary.WriteString(line)
			summary.WriteString("\n")
			lineCount++
		}
	}

	return summary.String()
}
