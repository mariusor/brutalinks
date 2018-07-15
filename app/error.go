package app

import (
	"fmt"
	"log"
	"net/http"
)

func HandleError(w http.ResponseWriter, r *http.Request, status int, errs ...error) {
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	d := errorModel{
		Status:        status,
		Title:         fmt.Sprintf("Error %d", status),
		InvertedTheme: IsInverted(r),
		Errors:        errs,
	}
	w.WriteHeader(status)

	for _, err := range errs {
		log.Printf("Err: %q", err)
	}

	RenderTemplate(r, w, "error.html", d)
}
