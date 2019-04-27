package frontend

import (
	"encoding/json"
	"github.com/openshift/osin"
	"net/http"
)

func redirectOrOutput (rs *osin.Response, w http.ResponseWriter, r *http.Request, h *handler) {
	// Add headers
	for i, k := range rs.Headers {
		for _, v := range k {
			w.Header().Add(i, v)
		}
	}

	if rs.Type == osin.REDIRECT {
		// Output redirect with parameters
		u, err := rs.GetRedirectUrl()
		if err != nil {
			h.HandleErrors(w, r, err)
			return
		}
		h.Redirect(w, r, u, http.StatusFound)
	} else {
		// set content type if the response doesn't already have one associated with it
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(rs.StatusCode)

		encoder := json.NewEncoder(w)
		if err := encoder.Encode(rs.Output); err != nil {
			h.HandleErrors(w, r, err)
			return
		}
		if err := h.saveSession(w, r); err != nil {
			h.HandleErrors(w, r, err)
			return
		}
	}
}

func (h *handler) Authorize(w http.ResponseWriter, r *http.Request) {
	s := h.os

	resp := s.NewResponse()
	defer resp.Close()

	if ar := s.HandleAuthorizeRequest(resp, r); ar != nil {
		if h.account.IsLogged() {
			ar.Authorized = true
			b, _ := json.Marshal(h.account)
			ar.UserData = b
		}
		s.FinishAuthorizeRequest(resp, r, ar)
	}
	redirectOrOutput(resp, w, r, h)
}

func (h *handler) Token(w http.ResponseWriter, r *http.Request) {
	s := h.os
	resp := s.NewResponse()
	defer resp.Close()

	if ar := s.HandleAccessRequest(resp, r); ar != nil {
		if who, ok := ar.UserData.(json.RawMessage); ok {
			if err := json.Unmarshal([]byte(who), &h.account); err == nil {
				ar.Authorized = h.account.IsLogged()
			} else {
				h.logger.Errorf("%s", err)
			}
		}
		s.FinishAccessRequest(resp, r, ar)
	}
	redirectOrOutput(resp, w, r, h)
}

