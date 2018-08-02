package api

import (
	"net/http"
	)

// GET /api/outbox
func HandleServiceOutbox(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{}"))
}
