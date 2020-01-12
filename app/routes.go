package app

import (
	"github.com/go-ap/errors"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"net/http"
	"os"
	"path/filepath"
)

func (h *handler) Routes() func(chi.Router) {
	return func(r chi.Router) {
		r.Use(middleware.GetHead)
		r.Use(ReqLogger(h.logger))
		r.Use(h.LoadSession)
		r.Use(SetSecurityHeaders)
		//r.Use(middleware.RedirectSlashes)

		r.Get("/about", h.HandleAbout)

		r.With(DefaultFilters, h.storage.Load).Get("/", h.HandleListing)
		r.With(h.CSRF).Group(func(r chi.Router) {
			r.Get("/submit", h.ShowSubmit)
			r.Post("/submit", h.HandleSubmit)
			r.With(checkUserCreatingEnabled).Get("/register", h.ShowRegister)
			r.With(checkUserCreatingEnabled).Post("/register", h.HandleRegister)
		})

		r.With(DefaultFilters).Route("/~{handle}", func(r chi.Router) {
			r.Get("/", h.ShowAccount)
			r.Post("/", h.HandleSubmit)
			r.Get("/follow", h.FollowAccount)
			r.Get("/follow/{action}", h.HandleFollowRequest)

			r.Route("/{hash}", func(r chi.Router) {
				r.Use(h.CSRF)
				r.Get("/", h.ShowItem)
				r.Post("/", h.HandleSubmit)

				r.Group(func(r chi.Router) {
					r.Use(h.ValidateLoggedIn(h.v.HandleErrors))
					r.Get("/yay", h.HandleVoting)
					r.Get("/nay", h.HandleVoting)

					r.Get("/bad", h.ShowReport)
					r.Post("/bad", h.HandleReport)

					r.With(h.ValidateItemAuthor).Group(func(r chi.Router) {
						r.Get("/edit", h.ShowItem)
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
		r.With(DomainFilters, middleware.StripSlashes).Get("/d", h.HandleDomains)
		r.With(DomainFilters).Get("/d/{domain}", h.HandleDomains)
		// @todo(marius) :link_generation:
		r.With(TagFilters).Get("/t/{tag}", h.HandleTags)

		r.With(h.NeedsSessions).Get("/logout", h.HandleLogout)
		r.With(h.CSRF, h.NeedsSessions).Group(func(r chi.Router) {
			r.Get("/login", h.ShowLogin)
			r.Post("/login", h.HandleLogin)
		})

		r.With(SelfFilters).Get("/self", h.HandleIndex)
		r.With(FederatedFilters).Get("/federated", h.HandleIndex)
		r.With(h.NeedsSessions, SelfFilters, h.ValidateLoggedIn(h.v.HandleErrors)).Get("/followed", h.HandleInbox)

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
		r.With(StripCookies).Get("/ns", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/ld+json")
			http.ServeFile(w, r, filepath.Join(assets, "ns.json"))
		}))
		r.With(StripCookies).Get("/favicon.ico", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(assets, "favicon.ico"))
		}))
		r.With(StripCookies).Get("/icons.svg", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(assets, "icons.svg"))
		}))
		r.With(StripCookies).Get("/robots.txt", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(assets, "robots.txt"))
		}))
		r.With(StripCookies).Get("/css/{path}", serveFiles(filepath.Join(assets, "css")))
		r.With(StripCookies).Get("/js/{path}", serveFiles(filepath.Join(assets, "js")))
	}
}

func serveFiles(st string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Clean(chi.URLParam(r, "path"))
		fullPath := filepath.Join(st, path)
		http.ServeFile(w, r, fullPath)
	}
}
