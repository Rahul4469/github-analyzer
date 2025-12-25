package controllers

import (
	"net/http"

	"github.com/rahul4469/github-analyzer/context"
	"github.com/rahul4469/github-analyzer/internal/views"
)

// StaticController handles static pages like home, about, etc.
type StaticController struct {
	templates StaticTemplates
}

// StaticTemplates holds templates for static pages.
type StaticTemplates struct {
	Home *views.Template
}

// NewStaticController creates a new StaticController.
func NewStaticController(templates StaticTemplates) *StaticController {
	return &StaticController{
		templates: templates,
	}
}

// HomeData holds data for the home page template.
type HomeData struct {
	Features []Feature
}

// Feature represents a feature displayed on the home page.
type Feature struct {
	Icon        string
	Title       string
	Description string
}

// GetHome renders the home page.
func (c *StaticController) GetHome(w http.ResponseWriter, r *http.Request) {
	// Get current user from context (may be nil)
	user := context.ContextGetUser(r.Context())

	// Check for logout message
	var success string
	if r.URL.Query().Get("msg") == "logged_out" {
		success = "You have been logged out successfully."
	}

	features := []Feature{
		{
			Icon:        "Code Analysis",
			Title:       "Deep Code Analysis",
			Description: "AI analyzes your actual source code, not just metadata. Find real bugs, security vulnerabilities, and performance issues.",
		},
		{
			Icon:        "Security Analysis",
			Title:       "Security Scanning",
			Description: "Identify SQL injection, XSS, authentication flaws, hardcoded secrets, and other security vulnerabilities in your codebase.",
		},
		{
			Icon:        "Performance Insights",
			Title:       "Performance Insights",
			Description: "Detect N+1 queries, memory leaks, inefficient algorithms, and other performance bottlenecks.",
		},
		{
			Icon:        "Reports",
			Title:       "Actionable Reports",
			Description: "Get prioritized issues with severity levels, file locations, and specific fix suggestions.",
		},
		{
			Icon:        "Security",
			Title:       "Secure by Design",
			Description: "Your code is analyzed securely. GitHub tokens are never stored, and analysis data belongs to you.",
		},
		{
			Icon:        "Fast Results",
			Title:       "Quick Results",
			Description: "Get comprehensive analysis in seconds. No lengthy setup or configuration required.",
		},
	}

	data := &views.TemplateData{
		Title:       "GitHub Analyzer - AI-Powered Code Analysis",
		CurrentUser: user,
		Success:     success,
		Data: HomeData{
			Features: features,
		},
	}

	c.templates.Home.ExecuteHTTP(w, r, data)
}

// HealthCheck returns a simple health status for monitoring.
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}
