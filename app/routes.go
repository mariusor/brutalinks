package app

import (
	"fmt"
	"github.com/go-ap/errors"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func (h *handler) Routes() func(chi.Router) {
	return func(r chi.Router) {

		r.Use(middleware.GetHead)
		r.Use(ReqLogger(h.logger))
		r.Use(h.LoadSession)
		r.Use(SetSecurityHeaders)
		//r.Use(middleware.RedirectSlashes)
		r.Use(middleware.Timeout(60 * time.Millisecond))

		r.Get("/about", h.HandleAbout)

		r.With(ModelMw(&listingModel{}), DefaultFilters, LoadServiceInboxMw).Get("/", h.HandleShow)
		r.With(h.CSRF).Group(func(r chi.Router) {
			r.With(ModelMw(&contentModel{Title: "Add new submission", Content: &Item{Edit: true}})).Get("/submit", h.HandleShow)
			r.Post("/submit", h.HandleSubmit)
			r.With(ModelMw(&registerModel{Title: "Register new account"}), checkUserCreatingEnabled).Get("/register", h.HandleShow)
			r.With(checkUserCreatingEnabled).Post("/register", h.HandleRegister)
		})

		r.With(LoadAuthorMw).Route("/~{handle}", func(r chi.Router) {
			r.With(ModelMw(&listingModel{tpl: "user"}), AccountFiltersMw, LoadOutboxMw).Get("/", h.HandleShow)
			r.Post("/", h.HandleSubmit)
			r.Get("/follow", h.FollowAccount)
			r.Get("/follow/{action}", h.HandleFollowRequest)

			r.Route("/{hash}", func(r chi.Router) {
				r.Use(h.CSRF, ModelMw(&contentModel{}), ItemFiltersMw, LoadOutboxObjectMw)
				r.Get("/", h.HandleShow)
				r.Post("/", h.HandleSubmit)

				r.Group(func(r chi.Router) {
					r.Use(h.ValidateLoggedIn(h.v.HandleErrors))
					r.Get("/yay", h.HandleVoting)
					r.Get("/nay", h.HandleVoting)

					r.Get("/bad", h.ShowReport)
					r.Post("/bad", h.HandleReport)

					r.With(h.ValidateItemAuthor).Group(func(r chi.Router) {
						r.With(ModelMw(&contentModel{Title: "Edit item"})).Get("/edit", h.HandleShow)
						r.Post("/edit", h.HandleSubmit)
						r.Get("/rm", h.HandleDelete)
					})
				})
			})
		})

		//r.Get("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/", frontend.HandleDate)
		//r.Get("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}", h.ShowItem)
		//r.Get("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}/{direction}", h.HandleVoting)
		//r.Post("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}", h.HandleSubmit)

		// @todo(marius) :link_generation:
		r.Get("/i/{hash}", h.HandleItemRedirect)

		// @todo(marius) :link_generation:
		r.With(ModelMw(&listingModel{}), DomainFiltersMw, LoadServiceInboxMw, middleware.StripSlashes).Get("/d", h.HandleShow)
		r.With(ModelMw(&listingModel{}), DomainFiltersMw, LoadServiceInboxMw).Get("/d/{domain}", h.HandleShow)
		// @todo(marius) :link_generation:
		r.With(ModelMw(&listingModel{}), TagFiltersMw, LoadServiceInboxMw).Get("/t/{tag}", h.HandleShow)

		r.With(h.NeedsSessions).Get("/logout", h.HandleLogout)
		r.With(h.CSRF, h.NeedsSessions).Group(func(r chi.Router) {
			r.With(ModelMw(&loginModel{Title: "Local authentication"})).Get("/login", h.HandleShow)
			r.Post("/login", h.HandleLogin)
		})

		r.With(ModelMw(&listingModel{}), SelfFiltersMw, LoadServiceInboxMw).Get("/self", h.HandleShow)
		r.With(ModelMw(&listingModel{}), FederatedFiltersMw, LoadServiceInboxMw).Get("/federated", h.HandleShow)
		r.With(ModelMw(&listingModel{}), h.NeedsSessions, FollowedFiltersMw, h.ValidateLoggedIn(h.v.HandleErrors), LoadInboxMw).Get("/followed", h.HandleShow)

		r.Route("/auth", func(r chi.Router) {
			r.Use(h.NeedsSessions)
			r.Get("/{provider}/callback", h.HandleCallback)
		})

		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			h.v.HandleErrors(w, r, errors.NotFoundf("%q", r.RequestURI))
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			h.v.HandleErrors(w, r, errors.MethodNotAllowedf("invalid %q request", r.Method))
		})

		workDir, _ := os.Getwd()
		assets := filepath.Join(workDir, "assets")

		// static
		r.With(StripCookies).Get("/ns", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/ld+json")
			w.Header().Set("Cache-Control", "public,max-age=31557600")
			http.ServeFile(w, r, filepath.Join(assets, "ns.json"))
		})
		r.With(StripCookies).Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "public,max-age=31557600")
			http.ServeFile(w, r, filepath.Join(assets, "favicon.ico"))
		})
		r.With(StripCookies).Get("/icons.svg", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "public,max-age=31557600")
			http.ServeFile(w, r, filepath.Join(assets, "icons.svg"))
		})
		r.With(StripCookies).Get("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(assets, "robots.txt"))
		})
		r.With(StripCookies).Get("/css/{path}", serveFiles(filepath.Join(assets, "css")))
		r.With(StripCookies).Get("/js/{path}", serveFiles(filepath.Join(assets, "js")))
	}
}

const year = 8766 * time.Hour

func serveFiles(st string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Clean(chi.URLParam(r, "path"))
		fullPath := filepath.Join(st, path)
		w.Header().Set("Cache-Control", fmt.Sprintf("public,max-age=%d", int(year.Seconds())))
		http.ServeFile(w, r, fullPath)
	}
}
