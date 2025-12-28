package views

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"
)

var TemplateFS fs.FS

// Template wraps a parsed template with helper methods for rendering.
type Template struct {
	tmpl *template.Template
}

// TemplateData is the standard data structure passed to all templates.
// It contains common fields that every page might need.
type TemplateData struct {
	// Current authenticated user (nil if not logged in)
	CurrentUser interface{}

	// CSRF token for forms
	CSRFToken string

	// Flash messages
	Error   string
	Success string
	Warning string
	Info    string

	// Page-specific data
	Data interface{}

	// Additional metadata
	Title       string
	Description string

	// Request info (useful for active nav highlighting)
	CurrentPath string

	// Environment
	IsDevelopment bool
}

// DefaultFuncMap returns the default template functions available in all templates.
func DefaultFuncMap() template.FuncMap {
	return template.FuncMap{
		// String manipulation
		"upper":    strings.ToUpper,
		"lower":    strings.ToLower,
		"title":    toTitle,
		"trim":     strings.TrimSpace,
		"truncate": truncate,

		// Date/time formatting
		"formatDate":     formatDate,
		"formatDateTime": formatDateTime,
		"formatRelative": formatRelative,
		"timeAgo":        timeAgo,

		// Number formatting
		"formatNumber": formatNumber,
		"percentage":   percentage,

		// Conditionals and comparisons
		"eq": func(a, b interface{}) bool { return a == b },
		"ne": func(a, b interface{}) bool { return a != b },
		"lt": func(a, b int) bool { return a < b },
		"gt": func(a, b int) bool { return a > b },
		"le": func(a, b int) bool { return a <= b },
		"ge": func(a, b int) bool { return a >= b },

		// Slice/map helpers
		"first":    first,
		"last":     last,
		"contains": strings.Contains,
		"join":     strings.Join,

		// HTML helpers
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		"safeURL":  func(s string) template.URL { return template.URL(s) },
		"safeCSS":  func(s string) template.CSS { return template.CSS(s) },
		"safeJS":   func(s string) template.JS { return template.JS(s) },

		// Status/severity styling
		"statusClass":   statusClass,
		"severityClass": severityClass,
		"severityIcon":  severityIcon,

		// Markdown rendering (basic)
		"markdown": markdownToHTML,

		// Default value
		"default": defaultValue,

		// Iteration helpers
		"seq": seq,
	}
}

// ParseFS parses templates from the embedded filesystem.
// It automatically includes the base layout and any partials.
//
// Usage:
//
//	tmpl, err := views.ParseFS("pages/home.gohtml")
//	// This will parse:
//	// - templates/layouts/base.gohtml
//	// - templates/partials/*.gohtml
//	// - templates/pages/home.gohtml
func ParseFS(patterns ...string) (*Template, error) {
	// Start with function map
	tmpl := template.New("").Funcs(DefaultFuncMap())

	// Parse base layout first
	basePath := "templates/layouts/base.gohtml"
	baseContent, err := fs.ReadFile(TemplateFS, basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read base template: %w", err)
	}

	tmpl, err = tmpl.Parse(string(baseContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse base template: %w", err)
	}

	// Parse all partials - they define their own names with {{define "name"}}
	partialPattern := "templates/partials/*.gohtml"
	partialMatches, err := fs.Glob(TemplateFS, partialPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob partials: %w", err)
	}

	for _, match := range partialMatches {
		content, err := fs.ReadFile(TemplateFS, match)
		if err != nil {
			return nil, fmt.Errorf("failed to read partial %s: %w", match, err)
		}

		// Parse the content - it contains its own {{define}} blocks
		tmpl, err = tmpl.Parse(string(content))
		if err != nil {
			return nil, fmt.Errorf("failed to parse partial %s: %w", match, err)
		}
	}

	// Parse the requested page templates - they define their own "content" block
	for _, pattern := range patterns {
		fullPattern := "templates/" + pattern
		content, err := fs.ReadFile(TemplateFS, fullPattern)
		if err != nil {
			return nil, fmt.Errorf("failed to read template %s: %w", pattern, err)
		}

		// Parse the content - pages define {{define "content"}} and use {{template "base" .}}
		tmpl, err = tmpl.Parse(string(content))
		if err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", pattern, err)
		}
	}

	return &Template{tmpl: tmpl}, nil
}

// MustParseFS is like ParseFS but panics on error.
// Use this during initialization when templates must be valid.
func MustParseFS(patterns ...string) *Template {
	tmpl, err := ParseFS(patterns...)
	if err != nil {
		panic(fmt.Sprintf("failed to parse templates: %v", err))
	}
	return tmpl
}

// Execute renders the template to the given writer with the provided data.
func (t *Template) Execute(w io.Writer, data *TemplateData) error {
	return t.tmpl.ExecuteTemplate(w, "base", data)
}

// ExecuteHTTP renders the template as an HTTP response.
// It handles errors gracefully and sets appropriate headers.
func (t *Template) ExecuteHTTP(w http.ResponseWriter, r *http.Request, data *TemplateData) {
	// Set current path for nav highlighting
	if data != nil {
		data.CurrentPath = r.URL.Path
	}

	// Render to buffer first to catch errors
	buf := &bytes.Buffer{}
	err := t.Execute(buf, data)
	if err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Set headers and write response
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	buf.WriteTo(w)
}

// ExecuteHTTPWithStatus renders the template with a custom HTTP status code.
func (t *Template) ExecuteHTTPWithStatus(w http.ResponseWriter, r *http.Request, status int, data *TemplateData) {
	if data != nil {
		data.CurrentPath = r.URL.Path
	}

	buf := &bytes.Buffer{}
	err := t.Execute(buf, data)
	if err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

// Template function implementations

func truncate(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length-3] + "..."
}

// toTitle converts a string to title case.
// Example: "hello world" -> "Hello World"
func toTitle(s string) string {
	if s == "" {
		return s
	}
	words := strings.Fields(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(string(word[0])) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, " ")
}

func formatDate(t time.Time) string {
	return t.Format("Jan 2, 2006")
}

func formatDateTime(t time.Time) string {
	return t.Format("Jan 2, 2006 3:04 PM")
}

func formatRelative(t time.Time) string {
	return timeAgo(t)
}

func timeAgo(t time.Time) string {
	duration := time.Since(t)

	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		mins := int(duration.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case duration < 24*time.Hour:
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case duration < 7*24*time.Hour:
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%d days ago", days)
	case duration < 30*24*time.Hour:
		weeks := int(duration.Hours() / 24 / 7)
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	default:
		return formatDate(t)
	}
}

func formatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1000000)
}

func percentage(value, total int) int {
	if total == 0 {
		return 0
	}
	return (value * 100) / total
}

func first(slice interface{}) interface{} {
	// This is a simplified implementation
	// In production, use reflection for proper slice handling
	return slice
}

func last(slice interface{}) interface{} {
	return slice
}

func statusClass(status string) string {
	switch status {
	case "pending":
		return "bg-yellow-100 text-yellow-800"
	case "processing":
		return "bg-blue-100 text-blue-800"
	case "completed":
		return "bg-green-100 text-green-800"
	case "failed":
		return "bg-red-100 text-red-800"
	default:
		return "bg-gray-100 text-gray-800"
	}
}

func severityClass(severity string) string {
	switch strings.ToUpper(severity) {
	case "HIGH":
		return "bg-red-100 text-red-800 border-red-200"
	case "MEDIUM":
		return "bg-orange-100 text-orange-800 border-orange-200"
	case "LOW":
		return "bg-yellow-100 text-yellow-800 border-yellow-200"
	case "INFO":
		return "bg-blue-100 text-blue-800 border-blue-200"
	default:
		return "bg-gray-100 text-gray-800 border-gray-200"
	}
}

func severityIcon(severity string) string {
	switch strings.ToUpper(severity) {
	case "HIGH":
		return "🔴"
	case "MEDIUM":
		return "🟠"
	case "LOW":
		return "🟡"
	case "INFO":
		return "🔵"
	default:
		return "⚪"
	}
}

func markdownToHTML(s string) template.HTML {
	// Basic markdown conversion - in production, use a proper markdown library
	// This is a placeholder that just preserves newlines
	s = strings.ReplaceAll(s, "\n", "<br>")
	return template.HTML(s)
}

func defaultValue(value, defaultVal interface{}) interface{} {
	if value == nil || value == "" || value == 0 {
		return defaultVal
	}
	return value
}

func seq(start, end int) []int {
	result := make([]int, end-start+1)
	for i := range result {
		result[i] = start + i
	}
	return result
}
