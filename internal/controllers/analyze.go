package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	localcontext "github.com/rahul4469/github-analyzer/context"
	"github.com/rahul4469/github-analyzer/internal/models"
	"github.com/rahul4469/github-analyzer/internal/services"
)

type AnalyzeController struct {
	UserService      *models.UserService
	RepoService      *models.RepositoryService
	AnalysisService  *models.AnalysisService
	GitHubFetcher    *services.GitHubFetcher
	AIAnalyzer       *services.AIAnalyzer
	DataFormatter    *services.DataFormatter
	TemplateRenderer TemplateRenderer
}

// Constructor: create a new analyze controller
func NewAnalyzeController(
	userService *models.UserService,
	repoService *models.RepositoryService,
	analysisService *models.AnalysisService,
	githubFetcher *services.GitHubFetcher,
	aiAnalyzer *services.AIAnalyzer,
	dataFormatter *services.DataFormatter,
	templateRenderer TemplateRenderer,
) *AnalyzeController {
	return &AnalyzeController{
		UserService:      userService,
		RepoService:      repoService,
		AnalysisService:  analysisService,
		GitHubFetcher:    githubFetcher,
		AIAnalyzer:       aiAnalyzer,
		DataFormatter:    dataFormatter,
		TemplateRenderer: templateRenderer,
	}
}

// AnalyzeRequest stores form submission data from user
type AnalyzeRequest struct {
	RepositoryURL string `form:"repository"`
	GitHubToken   string `form:"github_token"`
}

// AnalyzeResponse stores analysis result
type AnalyzeResponse struct {
	RepositoryID int64
	AnalysisID   int
	Success      bool
	Message      string
	Error        string
}

// Render User input form page (GET/ analyze)
func (ac *AnalyzeController) GetAnalyzeForm(w http.ResponseWriter, r *http.Request) {
	// get user from session/context
	user := getUserFromContext(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Data to pass to template
	data := map[string]interface{}{
		"Title": "Analyze Repository",
		"User":  user,
	}

	// Render template
	if err := ac.TemplateRenderer.Render(w, "analyze.gohtml", data); err != nil {
		http.Error(w, "Failed to render form", http.StatusInternalServerError)
		return
	}
}

// Process analyze form submission (POSt /analyze)
func (ac *AnalyzeController) PostAnalyze(w http.ResponseWriter, r *http.Request) {
	// get user from session/context
	user := getUserFromContext(r) // TODO
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Parse Form
	if err := r.ParseForm(); err != nil {
		ac.respondWithError(w, r, "Invalud Form submissions", http.StatusBadRequest)
		return
	}

	// Extract formValue
	req := AnalyzeRequest{
		RepositoryURL: strings.TrimSpace(r.FormValue("repository")),
		GitHubToken:   strings.TrimSpace(r.FormValue("github_token")),
	}

	// Validate Input : guthub token + repo url from user are they avaliable request body or not
	if validationErr := ac.validateRequest(&req); validationErr != nil {
		ac.respondWithError(w, r, validationErr.Error(), http.StatusBadRequest)
		return
	}

	// CREATE BASE CONTEXT WITH TIMEOUT
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Validate Github Token format
	tempFetcher := services.NewGitHubFetcher(req.GitHubToken)
	if !tempFetcher.IsValidToken(req.GitHubToken) {
		ac.respondWithError(w, r, "Invalid GitHub token format", http.StatusBadRequest)
		return
	}

	// Fetch repo data from GitHub
	repoData, err := ac.fetchRepositoryData(ctx, req)
	if err != nil {
		ac.respondWithError(w, r, fmt.Sprintf("Failed to fetch repository: %v", err), http.StatusBadRequest)
		return
	}

	// First Chekc user's API quota then send the data to AI for analysis
	hasQuota, err := ac.UserService.CheckQuotaAvailable(ctx, user.ID)
	if err != nil || !hasQuota {
		ac.respondWithError(w, r, "API quota exceeded. Please try again later.", http.StatusTooManyRequests)
		return
	}

	// Analyze code with pplx (but first Format data using DataFormatter)
	formattedData := ac.DataFormatter.FormatRepositoryDataForAnalysis(repoData)
	rawAnalysis, err := ac.analyzeCode(ctx, formattedData)
	if err != nil {
		ac.respondWithError(w, r, fmt.Sprintf("Analysis failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Extract scrores from analysis
	scores := ac.AIAnalyzer.ExtractScores(rawAnalysis)
	issueCount := ac.AIAnalyzer.CountIssues(rawAnalysis)
	summary := ac.DataFormatter.SummarizeAnalysis(rawAnalysis)

	// Save repository to db
	repository := &models.Repository{
		UserID:          user.ID,
		FullName:        repoData.Repository.GetFullName(),
		Owner:           repoData.Repository.GetOwner().GetLogin(),
		Name:            repoData.Repository.GetName(),
		URL:             repoData.Repository.GetHTMLURL(),
		Description:     repoData.Repository.GetDescription(),
		StarsCount:      repoData.Repository.GetStargazersCount(),
		ForksCount:      repoData.Repository.GetForksCount(),
		WatchersCount:   repoData.Repository.GetWatchersCount(),
		OpenIssuesCount: repoData.Repository.GetOpenIssuesCount(),
		PrimaryLanguage: repoData.Repository.GetLanguage(),
	}
	savedRepo, err := ac.RepoService.Create(ctx, repository)
	if err != nil && !strings.Contains(err.Error(), "duplicate") {
		ac.respondWithError(w, r, "Failed to save repository", http.StatusInternalServerError)
		return
	}

	// Use Existing repo if duplicate
	if savedRepo == nil {
		savedRepo, _ = ac.RepoService.GetByFullName(ctx, user.ID, repository.FullName)
	}

	// Save analysis to database
	analysis := &models.Analysis{
		RepositoryID:         savedRepo.ID,
		CodeQualityScore:     getScore(scores, "Quality Score"),
		SecurityScore:        getScore(scores, "Security Score"),
		ComplexityScore:      getScore(scores, "Complexity Score"),
		MaintainabilityScore: getScore(scores, "Maintainability Score"),
		PerformanceScore:     getScore(scores, "Performance Score"),
		TotalIssues:          issueCount,
		CriticalIssues:       countIssueSeverity(rawAnalysis, "critical"),
		HighIssues:           countIssueSeverity(rawAnalysis, "high"),
		MediumIssues:         countIssueSeverity(rawAnalysis, "medium"),
		LowIssues:            countIssueSeverity(rawAnalysis, "low"),
		Summary:              summary,
		RawAnalysis:          rawAnalysis,
	}

	savedAnalysis, err := ac.AnalysisService.Create(ctx, analysis)
	if err != nil {
		ac.respondWithError(w, r, "Failed to save analysis", http.StatusInternalServerError)
		return
	}

	// Update user quota
	_ = ac.UserService.UpdateQuota(ctx, user.ID, user.APIQuotaUsed+1)

	// Redirect to dhashboard
	redirectURL := fmt.Sprintf("/dashboard/%d", savedAnalysis.ID)
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// fetchRepository handler fetches all repo data from GitHub (using the guthub service)
func (ac *AnalyzeController) fetchRepositoryData(ctx context.Context, req AnalyzeRequest) (*services.RepositoryData, error) {
	// Create Github fetcher with user's token
	fetcher := services.NewGitHubFetcher(req.GitHubToken)

	// Fetch repository
	repoData, err := fetcher.FetchRepository(ctx, req.RepositoryURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch repository: %w", err)
	}

	if repoData == nil || repoData.Repository == nil {
		return nil, errors.New("repository not found")
	}

	return repoData, nil
}

// analyzeCode sends code data to pplx for analysis
func (ac *AnalyzeController) analyzeCode(ctx context.Context, codeData string) (string, error) {
	analysis, err := ac.AIAnalyzer.AnalyzeCode(ctx, codeData)
	if err != nil {
		return "", fmt.Errorf("AI analysis failed: %w", err)
	}

	if analysis == "" {
		return "", errors.New("empty analysis reposnse")
	}

	return analysis, nil
}

// HELPER FUNCTIONS ------------------------------------------

// Validate the user input field data (repo url & github token).
// validates for their presence in the request body
func (ac *AnalyzeController) validateRequest(req *AnalyzeRequest) error {
	if req.RepositoryURL == "" {
		return errors.New("repository URL is required")
	}

	// validate repo url format (owner/repo)
	parts := strings.Split(req.RepositoryURL, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return errors.New("invalid repository format: use owner/repo (e.g., golang/go)")
	}

	if req.GitHubToken == "" {
		return errors.New("GitHub token is required")
	}
	if len(req.GitHubToken) < 30 {
		return errors.New("GitHub token appears to be invalid")
	}
	return nil
}

// Error handler to pass error response data to the Renderer.
// Set default data & error message and render
func (ac *AnalyzeController) respondWithError(w http.ResponseWriter, r *http.Request, message string, statusCode int) {
	user := getUserFromContext(r)

	data := map[string]interface{}{
		"Title": "Analyze Repository",
		"User":  user,
		"Error": message,
	}
	w.WriteHeader(statusCode)
	_ = ac.TemplateRenderer.Render(w, "analyze.gohtml", data)
}

// search the "key" string from the analysed "scores" Map and return its value as score
func getScore(scores map[string]int, key string) int {
	if score, exists := scores[key]; exists {
		return score
	}
	return 50 // Default
}

func countIssueSeverity(analysis string, severity string) int {
	count := strings.Count(analysis, severity)
	if count > 100 {
		count = 100 // Cap at 100
	}
	return count
}

func getUserFromContext(r *http.Request) *models.User {
	// TODO: Implement session/context user extraction
	// This would get the user from the session or JWT token
	// For now, returning nil (implement in V6 with authentication)
	// user, ok := r.Context().Value("user").(*models.User)
	// if !ok {
	// 	return nil
	// }
	// return user

	return localcontext.UserFromContext(r.Context())
}
