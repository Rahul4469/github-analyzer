package models

import "time"

// ============================================
// RESPONSE TYPES
// ============================================

// AnalysisResponse wraps analysis results for API response
type AnalysisResponse struct {
	Success   bool      `json:"success"`
	Message   string    `json:"message"`
	Analysis  *Analysis `json:"analysis,omitempty"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// APIError represents an API error response
type APIError struct {
	Code      string    `json:"code"`
	Message   string    `json:"message"`
	Details   string    `json:"details,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
