package app

import (
	"fmt"
	"log"
	"net/http"
)

func (l *Littr) HandleError(w http.ResponseWriter, r *http.Request, err error, status int) {
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	d := errorModel{
		Status:        status,
		Title:         fmt.Sprintf("Error %d", status),
		InvertedTheme: l.InvertedTheme,
		Error:         err,
	}
	w.WriteHeader(status)

	log.Printf("%s %s Message: %q", r.Method, r.URL, d.Error)

	RenderTemplate(w, "error.html", d)
}
