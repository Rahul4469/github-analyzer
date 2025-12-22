package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AIAnalyzer handles Perplexity AI API interactions
type AIAnalyzer struct {
	APIKey string
	Client *http.Client
}

// NewAIAnalyzer constructore creates a new AI analyzer client with server's Perplexity key
func NewAIAnalyzer(apiKey string) *AIAnalyzer {
	return &AIAnalyzer{
		APIKey: apiKey,
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Request to PPLX
type AnalysisRequest struct {
	Model     string              `json:"model"`
	Messages  []PerplexityMessage `json:"messages"`
	MaxTokens int                 `json:"max_tokens"`
}

// Message to PPLX
type PerplexityMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Response from PPLX
type AnalysisResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// AnalyzeCode sends code repository data to Perplexity for analysis
func (aa *AIAnalyzer) AnalyzeCode(ctx context.Context, codeData string) (string, error) {
	// Validating API key from .env file
	if aa.APIKey == "" {
		return "", fmt.Errorf("perplexity API key not configured")
	}

	// Create analysis prompt
	prompt := aa.createAnalysisPrompt(codeData)

	// Request Body
	reqBody := AnalysisRequest{
		Model: "pplx-7b-online",
		Messages: []PerplexityMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxTokens: 2000,
	}

	// Marshal the request body to json
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request : %w", err)
	}

	// Create HTTP request -> Set Headers -> Send Request
	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		"https://api.perplexity.ai/chat/completions",
		bytes.NewBuffer(jsonBody),
	)
	// var buf bytes.Buffer
	// err := json.NewEncoder(&buf).Encode(reqBody)
	// req, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	// Set Headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", aa.APIKey))
	req.Header.Set("Content-Type", "application/json")

	//Send Request
	resp, err := aa.Client.Do(req) //the [Response] will contain a non-nil Body which the user is expected to close
	if err != nil {
		return "", fmt.Errorf("failed to call Perplexity API: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("perplexity API error (status %d): %s", resp.StatusCode, string(body))
	}

	// DECODE response
	var analysisResp AnalysisResponse
	if err := json.NewDecoder(resp.Body).Decode(&analysisResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract content
	if len(analysisResp.Choices) == 0 {
		return "", fmt.Errorf("no response choices from Perplexity API")
	}

	return analysisResp.Choices[0].Message.Content, nil
}

// createAnalysisPrompt creates a detailed prompt for code analysis
func (aa *AIAnalyzer) createAnalysisPrompt(codeData string) string {
	prompt := `You are an expert code analyzer. Analyze the following 
		repository and provide a structured analysis:
		REPOSITORY DATA:` + codeData + `
		
		Please provide analysis in the following format (use the exact scores format shown):

     	## Code Quality Assessment
     	Quality Score: [0-100]
     	- Key Findings:
     	  * [Finding 1]
     	  * [Finding 2]
     	  * [Finding 3]
	
     	## Security Analysis
     	Security Score: [0-100]
     	- Vulnerabilities Found:
     	  * [If any]
     	- Security Best Practices:
     	  * [If any recommendations]

     	## Performance Analysis
     	Performance Score: [0-100]
     	- Performance Issues:
     	  * [If any]
     	- Optimization Opportunities:
     	  * [If any]
	
     	## Code Complexity
     	Complexity Score: [0-100]
     	- Complex Areas:
     	  * [Areas with high complexity]
     	- Recommendations:
     	  * [How to simplify]
	
     	## Maintainability Assessment
     	Maintainability Score: [0-100]
     	- Maintainability Issues:
     	  * [Issues that affect maintainability]
     	- Recommendations:
     	  * [How to improve]
	
     	## Critical Issues (if any)
     	Count: [Number]
     	- [List each critical issue]
	
     	## Overall Recommendations
     	[List top 3-5 recommendations]
	
     	Make the scores practical and realistic. Be specific with findings. 
		Focus on actionable recommendations.
		`

	return prompt
}

// IsValidAPIKey validates Perplexity API key format thats saved in the .env
func (aa *AIAnalyzer) IsValidAPIKey(key string) bool {
	// Usually starts with pplx and is slong
	return len(key) > 20 && strings.Contains(key, "pplx")
}

// ExtractScores extracts numeric scores from analysis response
// Every analysis response block also has a score [0-100]
func (aa *AIAnalyzer) ExtractScores(analysis string) map[string]int {
	scores := make(map[string]int)

	// Default scores if extraction fails
	scoreNames := []string{
		"Quality Score",
		"Security Score",
		"Performance Score",
		"Complexity Score",
		"Maintainability Score",
	}

	for _, name := range scoreNames {
		// Try to find pattern like "Quality Score: 85" in the entore response
		if idx := strings.Index(analysis, name+":"); idx != -1 {
			// Find the no. after the colon
			substr := analysis[idx+len(name)+1:]
			var score int
			_, _ = fmt.Sscanf(strings.TrimSpace(substr), "%d", &score)
			if score > 0 && score <= 100 {
				scores[name] = score // saves one key value pair in scores
			}
		}
	}

	return scores
}

// CountIssues counts mentioned issues in analysis
func (aa *AIAnalyzer) CountIssues(analysis string) int {
	count := 0
	count += strings.Count(analysis, "issues")
	count += strings.Count(analysis, "Issue")
	count += strings.Count(analysis, "problem")
	count += strings.Count(analysis, "Problem")
	count += strings.Count(analysis, "vulnerability")
	count += strings.Count(analysis, "Vulnerability")
	count += strings.Count(analysis, "bug")
	count += strings.Count(analysis, "Bug")

	return count
}
