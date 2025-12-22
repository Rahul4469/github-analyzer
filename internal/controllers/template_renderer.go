package controllers

import (
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"text/template"
)

// TemplateRenderer handles template Rendering: ParseFS + Execute(tpl)
type TemplateRenderer interface {
	Render(w http.ResponseWriter, templateName string, data interface{}) error
}

// GoTemplateRenderer implements TemplateRenderer using html/template
type GoTemplateRenderer struct {
	FS        fs.FS
	BasePath  string
	Templates map[string]*template.Template
	Cache     bool
}

// Constructor: create a new Go template renderer
func NewTemplateRenderer(fs fs.FS, basePath string, cache bool) *GoTemplateRenderer {
	return &GoTemplateRenderer{
		FS:        fs,
		BasePath:  basePath,
		Templates: make(map[string]*template.Template),
		Cache:     cache,
	}
}

// Render templates with data, usual template Execution premises
func (gtr *GoTemplateRenderer) Render(w http.ResponseWriter, templateName string, data interface{}) error {
	// Check cache first if enabled
	if gtr.Cache {
		if tmpl, exists := gtr.Templates[templateName]; exists {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			return tmpl.Execute(w, data)
		}
	}

	// Parse template
	tmpl, err := gtr.parseTemplate(templateName)
	if err != nil {
		return err
	}

	// Cache if enabled
	if gtr.Cache {
		gtr.Templates[templateName] = tmpl
	}

	// Execute template
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.Execute(w, data)
}

// parseTemplate uses FS to give an escaped tmpl to get rendered/executed
func (gtr *GoTemplateRenderer) parseTemplate(templateName string) (*template.Template, error) {
	// Parse base template first
	basePath := filepath.Join(gtr.BasePath, "base.gohtml")
	tmpl, err := template.ParseFS(gtr.FS, basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base template: %w", err)
	}

	// Parse requested template
	templatePath := filepath.Join(gtr.BasePath, templateName)
	tmpl, err = tmpl.ParseFS(gtr.FS, templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template file: %w", err)
	}

	return tmpl, nil
}

func (gtr *GoTemplateRenderer) RenderError(w http.ResponseWriter, templateName string, errorMsg string) error {
	data := map[string]interface{}{
		"Error": errorMsg,
	}
	return gtr.Render(w, templateName, data)
}
