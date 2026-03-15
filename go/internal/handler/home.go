package handler

import (
	"html/template"
	"log"
	"net/http"
)

type HomeHandler struct {
	tmpl *template.Template
}

func NewHomeHandler(tmpl *template.Template) *HomeHandler {
	return &HomeHandler{tmpl: tmpl}
}

func (h *HomeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if err := h.tmpl.ExecuteTemplate(w, "index.html", struct{ Address string }{}); err != nil {
		log.Printf("[home] template error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
