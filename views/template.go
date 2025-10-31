package views

import (
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"path"
)

type Template struct {
	htmlTpl *template.Template
}

func ParseFs(fs fs.FS, patterns ...string) (Template, error) {
	tpl := template.New(path.Base(patterns[0]))
	//Registers custom functions that can be called
	//from within your HTML templates
	//Funcs must be called before ParseFS/Parse
	tpl.Funcs(
		template.FuncMap{
			"csrfField": func() template.HTML {
				return `<input type="hidden">`
			},
		},
	)

	tpl, err := tpl.ParseFS(fs, patterns...)
	if err != nil {
		return Template{}, fmt.Errorf("parsing template: %w", err)
	}
	return Template{htmlTpl: tpl}, nil
}

// helper func to reuse for templates
// Execute writes the tpl data as a response to the client
func (t Template) Execute(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := t.htmlTpl.Execute(w, data)
	if err != nil {
		log.Printf("parsing template: %v", err)
		http.Error(w, "There was an error Executing the template", http.StatusInternalServerError)
		return
	}
}
