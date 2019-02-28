package api

import (
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/mariusor/littr.go/internal/errors"
	"github.com/mariusor/littr.go/app"
	"github.com/writeas/go-nodeinfo"
	"net/http"
)

func (h handler) Routes() func(chi.Router) {
	return func(r chi.Router) {
		//r.Use(middleware.GetHead)
		r.Use(h.VerifyHttpSignature)
		r.Use(app.StripCookies)
		r.Use(app.NeedsDBBackend(h.HandleError))

		r.Route("/self", func(r chi.Router) {
			r.With(LoadFiltersCtxt(h.HandleError)).Get("/", h.HandleService)
			r.Route("/{collection}", func(r chi.Router) {
				r.Use(ServiceCtxt)

				r.With(LoadFiltersCtxt(h.HandleError), h.ItemCollectionCtxt).Get("/", h.HandleCollection)
				r.Route("/{hash}", func(r chi.Router) {
					r.With(LoadFiltersCtxt(h.HandleError), h.ItemCtxt).Get("/", h.HandleCollectionActivity)
					r.With(LoadFiltersCtxt(h.HandleError), h.ItemCtxt).Get("/object", h.HandleCollectionActivityObject)
					r.With(LoadFiltersCtxt(h.HandleError), h.ItemCollectionCtxt).Get("/object/replies", h.HandleCollection)
				})
			})
			r.With(LoadFiltersCtxt(h.HandleError), h.LoadActivity).Post("/outbox", h.ClientRequest)
			r.With(LoadFiltersCtxt(h.HandleError), h.LoadActivity).Post("/inbox", h.ServerRequest)
		})
		r.Route("/actors", func(r chi.Router) {
			r.With(LoadFiltersCtxt(h.HandleError)).Get("/", h.HandleActorsCollection)

			r.Route("/{handle}", func(r chi.Router) {
				r.Use(h.AccountCtxt)

				r.Get("/", h.HandleActor)
				r.Route("/{collection}", func(r chi.Router) {
					r.With(LoadFiltersCtxt(h.HandleError), h.ItemCollectionCtxt).Get("/", h.HandleCollection)
					r.Route("/{hash}", func(r chi.Router) {
						r.Use(middleware.GetHead)
						// this should update the activity
						r.With(LoadFiltersCtxt(h.HandleError), h.ItemCtxt).Get("/", h.HandleCollectionActivity)
						r.With(LoadFiltersCtxt(h.HandleError), h.ItemCtxt).Get("/object", h.HandleCollectionActivityObject)
						// this should update the item
						r.With(LoadFiltersCtxt(h.HandleError), h.ItemCtxt, h.ItemCollectionCtxt).Get("/object/replies", h.HandleCollection)
					})
				})
				r.With(LoadFiltersCtxt(h.HandleError), h.LoadActivity).Post("/outbox", h.ClientRequest)
				r.With(LoadFiltersCtxt(h.HandleError), h.LoadActivity).Post("/inbox", h.ServerRequest)
			})
		})

		cfg := nodeinfo.Config{
			BaseURL: BaseURL,
			InfoURL: "/nodeinfo",

			Metadata: nodeinfo.Metadata{
				NodeName:        app.Instance.NodeInfo().Title,
				NodeDescription: app.Instance.NodeInfo().Summary,
				Private:         false,
				Software: nodeinfo.SoftwareMeta{
					GitHub:   "https://github.com/mariusor/littr.go",
					HomePage: "https://littr.me",
					Follow:   "mariusor@metalhead.club",
				},
			},
			Protocols: []nodeinfo.NodeProtocol{
				nodeinfo.ProtocolActivityPub,
			},
			Services: nodeinfo.Services{
				Inbound:  []nodeinfo.NodeService{},
				Outbound: []nodeinfo.NodeService{},
			},
			Software: nodeinfo.SoftwareInfo{
				Name:    app.Instance.NodeInfo().Title,
				Version: app.Instance.NodeInfo().Version,
			},
		}

		ni := nodeinfo.NewService(cfg, NodeInfoResolver{})
		r.Get(cfg.InfoURL, ni.NodeInfo)

		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			h.HandleError(w, r, errors.NotFoundf("%s", r.RequestURI))
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			h.HandleError(w, r, errors.MethodNotAllowedf("invalid %s request", r.Method))
		})
	}
}
