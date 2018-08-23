package app

import (
	"fmt"
	"net/http"

	log "github.com/sirupsen/logrus"
)

// HandleError serves failed requests
func HandleError(w http.ResponseWriter, r *http.Request, status int, errs ...error) {
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	d := errorModel{
		Status:        status,
		Title:         fmt.Sprintf("Error %d", status),
		InvertedTheme: isInverted(r),
		Errors:        errs,
	}
	w.WriteHeader(status)

	for _, err := range errs {
		if err != nil {
			log.WithFields(log.Fields{}).Error("Err: %q", err)
		}
	}

	RenderTemplate(r, w, "error", d)
}
