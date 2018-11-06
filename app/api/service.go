package api

import (
	"fmt"
	as "github.com/mariusor/activitypub.go/activitystreams"
	json "github.com/mariusor/activitypub.go/jsonld"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/frontend"
	"net/http"
)

// HandleService
// GET /api/self
func HandleService(w http.ResponseWriter, r *http.Request) {
	us := as.Service{}

	id := app.Instance.BaseURL + "/api/self"

	rr := r.Context().Value(app.RepositoryCtxtKey)

	var err error
	var inf app.Info
	if repo, ok := rr.(app.CanLoadInfo); ok {
		if inf, err = repo.LoadInfo(); err != nil {
			HandleError(w, r, http.StatusInternalServerError, err)
			return
		}
	}

	us.ID = as.ObjectID(id)
	us.Type = as.ServiceType
	us.Name.Set(as.NilLangRef, inf.Title)
	us.URL = as.IRI(inf.URI)
	us.Inbox = as.IRI(fmt.Sprintf("%s/inbox", id))
	us.Outbox = as.IRI(fmt.Sprintf("%s/outbox", id))
	//us.Summary.Set(as.NilLangRef, "This is a link aggregator similar to hacker news and reddit")
	us.Summary.Set(as.NilLangRef, inf.Summary)
	us.Content.Set(as.NilLangRef, string(frontend.Markdown(inf.Description)))

	us.AttributedTo = as.IRI("https://github.com/mariusor")
	data, _ := json.Marshal(us)
	w.Header().Set("Content-Type", "application/activity+json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
