package controllers

import (
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
	return tmpl.Execute(w, data)
}

// parseTemplate uses FS to give an escaped tmpl to get rendered/executed
func (gtr *GoTemplateRenderer) parseTemplate(templateName string) (*template.Template, error) {
	// Parse base template first
	basePath := filepath.Join(gtr.BasePath, "base.gohtml")
	tmpl, err := template.ParseFS(gtr.FS, basePath)
	if err != nil {
		return nil, err
	}

	// Parse requested template
	templatePath := filepath.Join(gtr.BasePath, templateName)
	tmpl, err = tmpl.ParseFS(gtr.FS, templatePath)
	if err != nil {
		return nil, err
	}

	return tmpl, nil
}
