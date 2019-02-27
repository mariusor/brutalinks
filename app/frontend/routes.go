package frontend

import (
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/mariusor/littr.go/internal/errors"
	"github.com/mariusor/littr.go/app"
	"net/http"
	"os"
	"path/filepath"
)

func (h *handler) Routes() func(chi.Router) {
	return func(r chi.Router) {
		r.Use(h.LoadSession)
		r.Use(middleware.GetHead)
		r.Use(app.NeedsDBBackend(h.HandleError))
		//r.Use(middleware.RedirectSlashes)

		r.Get("/", h.HandleIndex)

		r.Get("/about", h.HandleAbout)

		r.Get("/submit", h.ShowSubmit)
		r.Post("/submit", h.HandleSubmit)

		r.Get("/register", h.ShowRegister)
		r.Post("/register", h.HandleRegister)

		r.Route("/~{handle}", func(r chi.Router) {
			r.Get("/", h.ShowAccount)

			r.Route("/{hash}", func(r chi.Router) {
				r.Get("/", h.ShowItem)
				r.Post("/", h.HandleSubmit)

				r.Get("/yay", h.HandleVoting)
				r.Get("/nay", h.HandleVoting)

				r.Get("/bad", h.ShowReport)
				r.Post("/bad", h.HandleReport)

				r.With(h.ValidatePermissions()).Get("/edit", h.ShowItem)
				r.With(h.ValidatePermissions()).Post("/edit", h.HandleSubmit)
				r.With(h.ValidatePermissions()).Get("/rm", h.HandleDelete)
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
		r.With(h.NeedsSessions).Get("/login", h.ShowLogin)
		r.With(h.NeedsSessions).Post("/login", h.HandleLogin)

		r.Get("/self", h.HandleIndex)
		r.Get("/federated", h.HandleIndex)
		r.Get("/followed", h.HandleIndex)

		r.With(h.NeedsSessions).Get("/auth/{provider}", h.HandleAuth)
		r.With(h.NeedsSessions).Get("/auth/{provider}/callback", h.HandleCallback)

		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			h.HandleError(w, r, errors.NotFoundf("%q", r.RequestURI))
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			h.HandleError(w, r, errors.MethodNotAllowedf("invalid %q request", r.Method))
		})

		workDir, _ := os.Getwd()
		assets := filepath.Join(workDir, "assets")

		// static
		r.With(app.StripCookies).Get("/ns", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json+ld")
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
