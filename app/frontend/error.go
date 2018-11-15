package frontend

import (
	"fmt"
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/app/log"
	"net/http"
)

// HandleError serves failed requests
func (h *handler) HandleError(w http.ResponseWriter, r *http.Request, status int, errs ...error) {
	d := errorModel{
		Status:        status,
		Title:         fmt.Sprintf("Error %d", status),
		InvertedTheme: isInverted(r),
		Errors:        errs,
	}
	w.WriteHeader(status)

	for _, err := range errs {
		if err != nil {
			h.logger.WithContext(log.Ctx{
				"trace": errors.ErrorStack(err),
			}).Error(err.Error())
		}
	}

	w.Header().Set("Cache-Control", " no-store, must-revalidate")
	w.Header().Set("Pragma", " no-cache")
	w.Header().Set("Expires", " 0")
	h.RenderTemplate(r, w, "error", d)
}
