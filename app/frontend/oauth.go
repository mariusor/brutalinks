package frontend

import (
	"github.com/openshift/osin"
	"net/http"
)

func (h *handler) Authorize(w http.ResponseWriter, r *http.Request) {
	resp := h.s.NewResponse()
	defer resp.Close()

	if ar := h.s.HandleAuthorizeRequest(resp, r); ar != nil {
		if h.account.IsLogged() {
			ar.Authorized = true
		}
		h.s.FinishAuthorizeRequest(resp, r, ar)
	}
	url := "/"
	if resp.IsError {
		// this seems to be only in case of non authorized
		h.addFlashMessage(Error, r, resp.StatusText)
		url = "/login"
	}
	if resp.Type == osin.REDIRECT {
		// Output redirect with parameters
		u, err := resp.GetRedirectUrl()
		if err != nil {
			h.HandleErrors(w, r, err)
		}
		url = u
	}
	h.Redirect(w, r, url, http.StatusFound)
}

func (h *handler) Token(w http.ResponseWriter, r *http.Request) {
	resp := h.s.NewResponse()
	defer resp.Close()

	if ar := h.s.HandleAccessRequest(resp, r); ar != nil {
		ar.Authorized = true
		h.s.FinishAccessRequest(resp, r, ar)
	}
	osin.OutputJSON(resp, w, r)
}

