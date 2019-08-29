package frontend

import (
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/mariusor/littr.go/app"
	"github.com/go-ap/errors"
	"net/http"
	"os"
	"path/filepath"
)

func (h *handler) Routes() func(chi.Router) {
	return func(r chi.Router) {
		r.Use(middleware.GetHead)
		r.Use(h.LoadSession)
		r.Use(app.NeedsDBBackend(h.HandleErrors))
		r.Use(app.ReqLogger(h.logger))
		//r.Use(middleware.RedirectSlashes)

		r.Get("/about", h.HandleAbout)

		r.Get("/", h.HandleIndex)
		r.With(h.CSRF).Group(func(r chi.Router) {
			r.Get("/submit", h.ShowSubmit)
			r.Post("/submit", h.HandleSubmit)
			r.Get("/register", h.ShowRegister)
			r.Post("/register", h.HandleRegister)
		})

		r.Route("/~{handle}", func(r chi.Router) {
			r.Get("/", h.ShowAccount)

			r.Route("/{hash}", func(r chi.Router) {
				r.Use(h.CSRF)
				r.Get("/", h.ShowItem)
				r.Post("/", h.HandleSubmit)

				r.Group(func(r chi.Router) {
					r.Use(h.ValidateLoggedIn(h.HandleErrors))
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
		r.Get("/d", h.HandleDomains)
		r.Get("/d/", h.HandleDomains)
		r.Get("/d/{domain}", h.HandleDomains)
		// @todo(marius) :link_generation:
		r.Get("/t/{tag}", h.HandleTags)

		r.With(h.NeedsSessions).Get("/logout", h.HandleLogout)
		r.With(h.CSRF, h.NeedsSessions).Group(func(r chi.Router) {
			r.Get("/login", h.ShowLogin)
			r.Post("/login", h.HandleLogin)
		})

		r.Get("/self", h.HandleIndex)
		r.Get("/federated", h.HandleIndex)
		r.With(h.NeedsSessions, h.ValidateLoggedIn(h.HandleErrors)).Get("/followed", h.HandleIndex)

		r.Route("/auth", func(r chi.Router) {
			r.Use(h.NeedsSessions)
			r.Get("/{provider}", h.HandleAuth)
			r.Get("/{provider}/callback", h.HandleCallback)
		})

		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			h.HandleErrors(w, r, errors.NotFoundf("%q", r.RequestURI))
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			h.HandleErrors(w, r, errors.MethodNotAllowedf("invalid %q request", r.Method))
		})

		workDir, _ := os.Getwd()
		assets := filepath.Join(workDir, "assets")

		// static
		r.With(app.StripCookies).Get("/ns", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/ld+json")
			http.ServeFile(w, r, filepath.Join(assets, "ns.json"))
		}))
		r.With(app.StripCookies).Get("/favicon.ico", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(assets, "favicon.ico"))
		}))
		r.With(app.StripCookies).Get("/icons.svg", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(assets, "icons.svg"))
		}))
		r.With(app.StripCookies).Get("/robots.txt", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(assets, "robots.txt"))
		}))
		r.With(app.StripCookies).Get("/css/{path}", serveFiles(filepath.Join(assets, "css")))
		r.With(app.StripCookies).Get("/js/{path}", serveFiles(filepath.Join(assets, "js")))
	}
}

func serveFiles(st string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Clean(chi.URLParam(r, "path"))
		fullPath := filepath.Join(st, path)
		http.ServeFile(w, r, fullPath)
	}
}
