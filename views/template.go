package views

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"path"

	"github.com/gorilla/csrf"
	"github.com/rahul4469/github-analyzer/context"
	"github.com/rahul4469/github-analyzer/models"
)

func ParseFS(fs fs.FS, patterns ...string) (Template, error) {
	tpl := template.New(path.Base(patterns[0]))
	//Registers custom functions that can be called
	//from within your HTML templates
	//Funcs must be called before ParseFS/Parse
	tpl.Funcs(
		template.FuncMap{
			"csrfField": func() (template.HTML, error) {
				return "", fmt.Errorf("CSRDField not implemented")
			},
			"currentUser": func() (template.HTML, error) {
				return "", fmt.Errorf("current user not implemented")
			},
		},
	)

	tpl, err := tpl.ParseFS(fs, patterns...)
	if err != nil {
		return Template{}, fmt.Errorf("parsing template: %w", err)
	}
	return Template{htmlTpl: tpl}, nil
}

type Template struct {
	htmlTpl *template.Template
}

// helper func to reuse for templates
// Execute writes the tpl data as a response to the client
func (t Template) Execute(w http.ResponseWriter, r *http.Request, data interface{}) {
	tpl, err := t.htmlTpl.Clone()
	if err != nil {
		log.Printf("Cloning Template: %v", err)
		http.Error(w, "There was an error rendering the page", http.StatusInternalServerError)
		return
	}

	tpl.Funcs(
		template.FuncMap{
			"csrfField": func() template.HTML {
				return csrf.TemplateField(r)
			},
			"currentUser": func() *models.User {
				return context.User(r.Context())
			},
		},
	)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var buf bytes.Buffer
	err = tpl.Execute(&buf, data) // ?
	if err != nil {
		log.Printf("parsing template: %v", err)
		http.Error(w, "There was an error Executing the template", http.StatusInternalServerError)
		return
	}
	io.Copy(w, &buf)
}
