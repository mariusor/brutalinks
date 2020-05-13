package app

import (
	"fmt"
	"github.com/go-ap/errors"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/tdewolff/minify"
	"github.com/tdewolff/minify/css"
	"github.com/tdewolff/minify/js"
	"github.com/tdewolff/minify/svg"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

func (h *handler) Routes() func(chi.Router) {
	return func(r chi.Router) {
		r.Use(middleware.GetHead)
		r.Use(ReqLogger(h.logger))
		r.Use(h.LoadSession)
		r.Use(SetSecurityHeaders)
		r.Use(middleware.Timeout(60 * time.Millisecond))

		r.With(h.CSRF).Group(func(r chi.Router) {
			r.With(ModelMw(&contentModel{Title: "Add new submission", Content: &Item{Edit: true}})).Get("/submit", h.HandleShow)
			r.Post("/submit", h.HandleSubmit)
			r.With(checkUserCreatingEnabled).Route("/register", func(r chi.Router) {
				r.Group(func(r chi.Router) {
					r.With(ModelMw(&registerModel{Title: "Register new account"})).Get("/", h.HandleShow)
					r.With(ModelMw(&registerModel{Title: "Register account from invite"}), LoadInvitedMw).Get("/{hash}", h.HandleShow)
				})
				r.Post("/", h.HandleRegister)
			})
			r.With(h.NeedsSessions).Group(func(r chi.Router) {
				r.With(ModelMw(&loginModel{Title: "Local authentication"})).Get("/login", h.HandleShow)
				r.Post("/login", h.HandleLogin)
			})
		})

		r.With(h.LoadAuthorMw).Route("/~{handle}", func(r chi.Router) {
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

					//r.Get("/bad", h.ShowReport)
					r.With(ModelMw(&contentModel{Title: "Report item"})).Get("/bad", h.HandleShow)
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

		r.With(h.NeedsSessions).Get("/logout", h.HandleLogout)
		r.With(h.NeedsSessions, h.ValidateLoggedIn(h.v.HandleErrors)).Post("/invite", h.HandleSendInvite)

		r.With(ModelMw(&listingModel{})).Group(func(r chi.Router) {
			// @todo(marius) :link_generation:
			r.With(DefaultFilters, LoadServiceInboxMw).Get("/", h.HandleShow)
			r.With(DomainFiltersMw, LoadServiceInboxMw, middleware.StripSlashes).Get("/d", h.HandleShow)
			r.With(DomainFiltersMw, LoadServiceInboxMw).Get("/d/{domain}", h.HandleShow)
			r.With(TagFiltersMw, LoadServiceInboxMw).Get("/t/{tag}", h.HandleShow)
			r.With(SelfFiltersMw, LoadServiceInboxMw).Get("/self", h.HandleShow)
			r.With(FederatedFiltersMw, LoadServiceInboxMw).Get("/federated", h.HandleShow)
			r.With(h.NeedsSessions, FollowedFiltersMw, h.ValidateLoggedIn(h.v.HandleErrors), LoadInboxMw).
				Get("/followed", h.HandleShow)
			r.With(ModelMw(&listingModel{tpl: "moderation"}), ModerationFiltersMw, LoadInboxMw, AnonymizeListing).
				Get("/moderation", h.HandleShow)
			r.With(ModelMw(&listingModel{tpl: "accounts"}), ActorsFiltersMw, LoadServiceInboxMw).
				Get("/~", h.HandleShow)
		})

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

		r.Get("/about", h.HandleAbout)
		workDir, _ := os.Getwd()
		assets := filepath.Join(workDir, "assets")

		// static
		r.With(StripCookies).Group(func(r chi.Router) {
			r.Get("/ns", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/ld+json")
				w.Header().Set("Cache-Control", fmt.Sprintf("public,max-age=%d", int(year.Seconds())))
				http.ServeFile(w, r, filepath.Join(assets, "ns.json"))
			})
			r.Get("/favicon.ico", serveFiles(filepath.Join(assets, "/favicon.ico")))
			r.Get("/icons.svg", serveFiles(filepath.Join(assets, "/icons.svg")))
			r.Get("/robots.txt", serveFiles(filepath.Join(assets, "/robots.txt")))
			r.Get("/css/{path}", serveFiles(filepath.Join(assets, "css")))
			r.Get("/js/{path}", serveFiles(filepath.Join(assets, "js")))
		})
	}
}

const year = 8766 * time.Hour

func serveFiles(st string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Clean(chi.URLParam(r, "path"))
		fullPath := filepath.Join(st, path)

		m := minify.New()
		m.AddFunc("image/svg+xml", svg.Minify)
		m.AddFunc("text/css", css.Minify)
		m.AddFuncRegexp(regexp.MustCompile("^(application|text)/(x-)?(java|ecma)script$"), js.Minify)

		mw := m.ResponseWriter(w, r)
		defer mw.Close()

		w = mw
		w.Header().Set("Cache-Control", fmt.Sprintf("public,max-age=%d", int(year.Seconds())))
		http.ServeFile(w, r, fullPath)
	}
}
