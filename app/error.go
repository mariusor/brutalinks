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

	var terr error
	log.Printf("%s %s Message: %q", r.Method, r.URL, d.Error)

	t, terr := l.LoadTemplates(templateDir, "error.html")
	if terr != nil {
		log.Print(terr)
	}
	terr = t.Execute(w, d)
	if terr != nil {
		log.Print(terr)
	}
}
