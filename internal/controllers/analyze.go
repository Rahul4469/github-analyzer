package controllers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"github.com/rahul4469/github-analyzer/internal/crypto"
	"github.com/rahul4469/github-analyzer/internal/middleware"
	"github.com/rahul4469/github-analyzer/internal/models"
	"github.com/rahul4469/github-analyzer/internal/services"
	"github.com/rahul4469/github-analyzer/internal/views"
)

// AnalyzeController handles repository analysis.
type AnalyzeController struct {
	analysisService   *models.AnalysisService
	repositoryService *models.RepositoryService
	userService       *models.UserService
	githubService     *services.GitHubService
	perplexityService *services.PerplexityService
	encryptor         *crypto.Encryptor
	templates         AnalyzeTemplates
	maxFilesToFetch   int
}

// AnalyzeTemplates holds the templates for analysis pages.
type AnalyzeTemplates struct {
	Form   *views.Template
	Result *views.Template
}

// NewAnalyzeController creates a new AnalyzeController.
func NewAnalyzeController(
	analysisService *models.AnalysisService,
	repositoryService *models.RepositoryService,
	userService *models.UserService,
	githubService *services.GitHubService,
	perplexityService *services.PerplexityService,
	encryptor *crypto.Encryptor,
	templates AnalyzeTemplates,
) *AnalyzeController {
	return &AnalyzeController{
		analysisService:   analysisService,
		repositoryService: repositoryService,
		userService:       userService,
		githubService:     githubService,
		perplexityService: perplexityService,
		encryptor:         encryptor,
		templates:         templates,
		maxFilesToFetch:   15,
	}
}

// AnalyzeFormData holds data for the analyze form template.
type AnalyzeFormData struct {
	RepoURL         string
	GitHubConnected bool
	GitHubUsername  string
}

// GetAnalyze renders the analysis form.
func (c *AnalyzeController) GetAnalyze(w http.ResponseWriter, r *http.Request) {
	user := middleware.MustCurrentUser(r)

	// Check quota
	if user.RemainingQuota() <= 0 {
		http.Redirect(w, r, "/dashboard?error=Quota+exceeded", http.StatusSeeOther)
		return
	}

	// Check if GitHub is connected
	githubConnected := user.HasGitHubConnected()
	var githubUsername string
	if user.GitHubUsername != nil {
		githubUsername = *user.GitHubUsername
	}

	data := &views.TemplateData{
		Title:       "Analyze Repository",
		CSRFToken:   csrf.Token(r),
		CurrentUser: user,
		Data: AnalyzeFormData{
			GitHubConnected: githubConnected,
			GitHubUsername:  githubUsername,
		},
	}

	// If GitHub not connected, show warning
	if !githubConnected {
		data.Warning = "Please connect your GitHub account first to analyze repositories."
	}

	c.templates.Form.ExecuteHTTP(w, r, data)
}

// PostAnalyze handles the analysis form submission.
func (c *AnalyzeController) PostAnalyze(w http.ResponseWriter, r *http.Request) {
	user := middleware.MustCurrentUser(r)

	if err := r.ParseForm(); err != nil {
		c.renderFormError(w, r, user, "", "Invalid form data")
		return
	}

	repoURL := r.FormValue("repo_url")

	// Validate inputs
	if repoURL == "" {
		c.renderFormError(w, r, user, repoURL, "Repository URL is required")
		return
	}

	// Check if GitHub is connected
	if !user.HasGitHubConnected() {
		c.renderFormError(w, r, user, repoURL, "Please connect your GitHub account first")
		return
	}

	// Get and decrypt the GitHub token
	encryptedToken, err := c.userService.GetGitHubToken(r.Context(), user.ID)
	if err != nil || encryptedToken == "" {
		c.renderFormError(w, r, user, repoURL, "GitHub token not found. Please reconnect your GitHub account.")
		return
	}

	githubToken, err := c.encryptor.Decrypt(encryptedToken)
	if err != nil {
		log.Printf("Failed to decrypt GitHub token: %v", err)
		c.renderFormError(w, r, user, repoURL, "Failed to access GitHub token. Please reconnect your GitHub account.")
		return
	}

	// Parse and validate GitHub URL
	owner, repo, err := models.ParseGitHubURL(repoURL)
	if err != nil {
		c.renderFormError(w, r, user, repoURL, "Invalid GitHub repository URL. Use format: https://github.com/owner/repo")
		return
	}

	// Check user quota
	if user.RemainingQuota() <= 0 {
		c.renderFormError(w, r, user, repoURL, "You have exceeded your API quota. Please contact support.")
		return
	}

	// Perform the analysis
	analysisID, err := c.performAnalysis(r, user, owner, repo, repoURL, githubToken)
	if err != nil {
		log.Printf("Analysis failed for %s/%s: %v", owner, repo, err)
		c.renderFormError(w, r, user, repoURL, fmt.Sprintf("Analysis failed: %v", err))
		return
	}

	// Redirect to results page
	http.Redirect(w, r, fmt.Sprintf("/analyze/%d", analysisID), http.StatusSeeOther)
}

// performAnalysis executes the full analysis pipeline.
func (c *AnalyzeController) performAnalysis(r *http.Request, user *models.User, owner, repo, repoURL, githubToken string) (int64, error) {
	ctx := r.Context()

	// Step 1: Fetch repository metadata from GitHub
	log.Printf("Fetching repository metadata for %s/%s", owner, repo)
	repoInfo, err := c.githubService.GetRepository(ctx, owner, repo, githubToken)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch repository: %w", err)
	}

	// Step 2: Create or update repository record
	repoModel := &models.Repository{
		UserID:          user.ID,
		GitHubURL:       repoURL,
		Owner:           owner,
		Name:            repo,
		Description:     &repoInfo.Description,
		PrimaryLanguage: &repoInfo.Language,
		StarsCount:      repoInfo.StargazersCount,
		ForksCount:      repoInfo.ForksCount,
	}

	savedRepo, err := c.repositoryService.Create(ctx, repoModel)
	if err != nil {
		return 0, fmt.Errorf("failed to save repository: %w", err)
	}

	// Step 3: Create analysis record
	analysis, err := c.analysisService.Create(ctx, user.ID, savedRepo.ID)
	if err != nil {
		return 0, fmt.Errorf("failed to create analysis: %w", err)
	}

	// Step 4: Mark as processing
	if err := c.analysisService.MarkProcessing(ctx, analysis.ID); err != nil {
		log.Printf("Failed to mark analysis as processing: %v", err)
	}

	// Step 5: Fetch actual code files (THE ENHANCED FEATURE!)
	log.Printf("Fetching source code files for %s/%s", owner, repo)
	codeFiles, codeStructure, err := c.githubService.GetRepositoryFiles(ctx, owner, repo, githubToken, c.maxFilesToFetch)
	if err != nil {
		_ = c.analysisService.Fail(ctx, analysis.ID, fmt.Sprintf("Failed to fetch code: %v", err))
		return 0, fmt.Errorf("failed to fetch code files: %w", err)
	}
	log.Printf("Fetched %d code files for analysis", len(codeFiles))

	// Step 6: Fetch README
	readme, _ := c.githubService.GetREADME(ctx, owner, repo, githubToken)

	// Step 7: Store GitHub data
	if err := c.analysisService.UpdateGitHubData(ctx, analysis.ID, codeStructure, codeFiles, readme); err != nil {
		log.Printf("Failed to store GitHub data: %v", err)
	}

	// Step 8: Send to Perplexity AI for analysis
	log.Printf("Sending %d files to Perplexity AI for analysis", len(codeFiles))
	aiInput := services.AnalysisInput{
		RepoName:        repo,
		RepoOwner:       owner,
		Description:     repoInfo.Description,
		PrimaryLanguage: repoInfo.Language,
		README:          readme,
		CodeStructure:   codeStructure,
		CodeFiles:       codeFiles, // THE ACTUAL CODE!
	}

	aiResult, err := c.perplexityService.Analyze(ctx, aiInput)
	if err != nil {
		_ = c.analysisService.Fail(ctx, analysis.ID, fmt.Sprintf("AI analysis failed: %v", err))
		return 0, fmt.Errorf("AI analysis failed: %w", err)
	}
	log.Printf("AI analysis completed, found %d issues, used %d tokens", len(aiResult.Issues), aiResult.TokensUsed)

	// Step 9: Store results
	if err := c.analysisService.Complete(ctx, analysis.ID, aiResult.RawAnalysis, aiResult.Summary, aiResult.Issues, aiResult.TokensUsed); err != nil {
		return 0, fmt.Errorf("failed to store results: %w", err)
	}

	// Step 10: Update user quota
	if err := c.userService.UpdateAPIQuota(ctx, user.ID, aiResult.TokensUsed); err != nil {
		log.Printf("Failed to update user quota: %v", err)
	}

	return analysis.ID, nil
}

// renderFormError renders the form with an error message.
func (c *AnalyzeController) renderFormError(w http.ResponseWriter, r *http.Request, user *models.User, repoURL, errMsg string) {
	// Get GitHub connection status
	githubConnected := user.HasGitHubConnected()
	var githubUsername string
	if user.GitHubUsername != nil {
		githubUsername = *user.GitHubUsername
	}

	data := &views.TemplateData{
		Title:       "Analyze Repository",
		CSRFToken:   csrf.Token(r),
		CurrentUser: user,
		Error:       errMsg,
		Data: AnalyzeFormData{
			RepoURL:         repoURL,
			GitHubConnected: githubConnected,
			GitHubUsername:  githubUsername,
		},
	}
	c.templates.Form.ExecuteHTTPWithStatus(w, r, http.StatusUnprocessableEntity, data)
}

// AnalysisResultData holds data for the result template.
type AnalysisResultData struct {
	Analysis *models.Analysis
}

// GetResult renders the analysis results page.
func (c *AnalyzeController) GetResult(w http.ResponseWriter, r *http.Request) {
	user := middleware.MustCurrentUser(r)

	// Get analysis ID from URL
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid analysis ID", http.StatusBadRequest)
		return
	}

	// Fetch analysis
	analysis, err := c.analysisService.ByID(r.Context(), id)
	if err != nil {
		if err == models.ErrAnalysisNotFound {
			http.Error(w, "Analysis not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to load analysis", http.StatusInternalServerError)
		return
	}

	// Verify ownership
	if analysis.UserID != user.ID {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	data := &views.TemplateData{
		Title:       fmt.Sprintf("Analysis: %s", analysis.Repository.FullName()),
		CSRFToken:   csrf.Token(r),
		CurrentUser: user,
		Data: AnalysisResultData{
			Analysis: analysis,
		},
	}

	c.templates.Result.ExecuteHTTP(w, r, data)
}

// DeleteAnalysis handles analysis deletion.
func (c *AnalyzeController) DeleteAnalysis(w http.ResponseWriter, r *http.Request) {
	user := middleware.MustCurrentUser(r)

	// Get analysis ID from URL
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid analysis ID", http.StatusBadRequest)
		return
	}

	// Fetch analysis to verify ownership
	analysis, err := c.analysisService.ByID(r.Context(), id)
	if err != nil {
		http.Redirect(w, r, "/dashboard?error=Analysis+not+found", http.StatusSeeOther)
		return
	}

	// Verify ownership
	if analysis.UserID != user.ID {
		http.Redirect(w, r, "/dashboard?error=Access+denied", http.StatusSeeOther)
		return
	}

	// Delete
	if err := c.analysisService.Delete(r.Context(), id); err != nil {
		http.Redirect(w, r, "/dashboard?error=Failed+to+delete", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/dashboard?success=Analysis+deleted", http.StatusSeeOther)
}
