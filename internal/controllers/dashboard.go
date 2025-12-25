package controllers

import (
	"net/http"

	"github.com/gorilla/csrf"
	"github.com/rahul4469/github-analyzer/internal/middleware"
	"github.com/rahul4469/github-analyzer/internal/models"
	"github.com/rahul4469/github-analyzer/internal/views"
)

// DashboardController handles the user dashboard.
type DashboardController struct {
	analysisService   *models.AnalysisService
	repositoryService *models.RepositoryService
	template          *views.Template
}

// NewDashboardController creates a new DashboardController.
func NewDashboardController(
	analysisService *models.AnalysisService,
	repositoryService *models.RepositoryService,
	template *views.Template,
) *DashboardController {
	return &DashboardController{
		analysisService:   analysisService,
		repositoryService: repositoryService,
		template:          template,
	}
}

// DashboardData holds data for the dashboard template.
type DashboardData struct {
	Analyses      []*models.Analysis
	StatusCounts  map[models.AnalysisStatus]int
	TotalAnalyses int
	QuotaUsed     int
	QuotaLimit    int
	QuotaPercent  int
}

// GetDashboard renders the user dashboard.
func (c *DashboardController) GetDashboard(w http.ResponseWriter, r *http.Request) {
	user := middleware.MustCurrentUser(r)

	// Get recent analyses
	analyses, err := c.analysisService.ByUserID(r.Context(), user.ID, 20)
	if err != nil {
		http.Error(w, "Failed to load analyses", http.StatusInternalServerError)
		return
	}

	// Get status counts
	statusCounts, err := c.analysisService.CountByStatus(r.Context(), user.ID)
	if err != nil {
		statusCounts = make(map[models.AnalysisStatus]int)
	}

	// Calculate total
	totalAnalyses := 0
	for _, count := range statusCounts {
		totalAnalyses += count
	}

	data := &views.TemplateData{
		Title:       "Dashboard",
		CSRFToken:   csrf.Token(r),
		CurrentUser: user,
		Data: DashboardData{
			Analyses:      analyses,
			StatusCounts:  statusCounts,
			TotalAnalyses: totalAnalyses,
			QuotaUsed:     user.APIQuotaUsed,
			QuotaLimit:    user.APIQuotaLimit,
			QuotaPercent:  user.QuotaPercentUsed(),
		},
	}

	// Check for success/error messages from query params
	if msg := r.URL.Query().Get("success"); msg != "" {
		data.Success = msg
	}
	if msg := r.URL.Query().Get("error"); msg != "" {
		data.Error = msg
	}

	c.template.ExecuteHTTP(w, r, data)
}
