package app

import (
	"github.com/go-ap/errors"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/mariusor/littr.go/internal/assets"
	"github.com/mariusor/littr.go/internal/config"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func (h *handler) Routes(c *config.Configuration) func(chi.Router) {
	return func(r chi.Router) {
		r.Use(middleware.GetHead)
		r.Use(ReqLogger(h.logger))
		r.Use(SetSecurityHeaders)

		r.Group(func(r chi.Router) {
			r.Use(h.LoadSession)
			r.Use(middleware.Timeout(60 * time.Millisecond))
			r.Use(h.OutOfOrderMw)

			r.With(h.CSRF).Group(func(r chi.Router) {
				r.With(AddModelMw).Get("/submit", h.HandleShow)
				r.Post("/submit", h.HandleSubmit)
				r.With(c.CheckUserCreatingEnabled).Route("/register", func(r chi.Router) {
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
				r.With(AccountListingModelMw, AccountFiltersMw, LoadOutboxMw).Get("/", h.HandleShow)

				r.Get("/follow", h.FollowAccount)
				r.Get("/follow/{action}", h.HandleFollowRequest)

				r.With(h.CSRF, MessageUserContentModelMw, AccountFiltersMw, LoadOutboxMw).Group(func(r chi.Router) {
					r.Get("/message", h.HandleShow)
					r.Post("/message", h.HandleSubmit)

					r.With(BlockAccountModelMw).Get("/block", h.HandleShow)
					r.Post("/block", h.BlockAccount)
					r.With(ReportAccountModelMw).Get("/bad", h.HandleShow)
					r.Post("/bad", h.ReportAccount)
				})

				r.Route("/{hash}", func(r chi.Router) {
					r.Use(h.CSRF, ContentModelMw, ItemFiltersMw, LoadObjectFromInboxMw, ThreadedListingMw)
					r.Get("/", h.HandleShow)
					r.Post("/", h.HandleSubmit)

					r.Group(func(r chi.Router) {
						r.Use(h.ValidateLoggedIn(h.v.HandleErrors))
						r.Get("/yay", h.HandleVoting)
						r.Get("/nay", h.HandleVoting)

						//r.Get("/bad", h.ShowReport)
						r.With(ReportContentModelMw).Get("/bad", h.HandleShow)
						r.Post("/bad", h.ReportItem)
						r.With(BlockContentModelMw).Get("/block", h.HandleShow)
						r.Post("/block", h.BlockItem)

						r.With(h.ValidateItemAuthor).Group(func(r chi.Router) {
							r.With(EditContentModelMw).Get("/edit", h.HandleShow)
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

			r.With(ListingModelMw).Group(func(r chi.Router) {
				// @todo(marius) :link_generation:
				r.With(DefaultFilters, LoadServiceInboxMw).Get("/", h.HandleShow)
				r.With(DomainFiltersMw, LoadServiceInboxMw, middleware.StripSlashes).Get("/d", h.HandleShow)
				r.With(DomainFiltersMw, LoadServiceInboxMw).Get("/d/{domain}", h.HandleShow)
				r.With(TagFiltersMw, LoadServiceInboxMw).Get("/t/{tag}", h.HandleShow)
				r.With(SelfFiltersMw, LoadServiceInboxMw).Get("/self", h.HandleShow)
				r.With(FederatedFiltersMw, LoadServiceInboxMw).Get("/federated", h.HandleShow)
				r.With(h.NeedsSessions, FollowedFiltersMw, h.ValidateLoggedIn(h.v.HandleErrors), LoadInboxMw).
					Get("/followed", h.HandleShow)
				r.With(ModelMw(&listingModel{tpl: "moderation"}), ModerationFiltersMw, LoadServiceInboxMw).
					Get("/moderation", h.HandleShow)
				r.With(ModelMw(&listingModel{tpl: "listing"}), ActorsFiltersMw, LoadServiceInboxMw, ThreadedListingMw).
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
		})

		r.Get("/about", h.HandleAbout)
		workDir, _ := os.Getwd()
		assetsDir := filepath.Join(workDir, "assets")

		styles := assets.AssetFiles{
			"moderate.css": []string{"main.css", "listing.css", "article.css", "moderate.css"},
			"content.css": []string{"main.css", "article.css", "content.css"},
			"listing.css": []string{"main.css", "listing.css", "article.css"},
			"moderation.css": []string{"main.css", "listing.css", "article.css", "moderation.css"},
			"user.css": []string{"main.css", "listing.css", "article.css", "user.css"},
			"user-message.css": []string{"main.css", "listing.css", "article.css", "user-message.css"},
			"new.css": []string{"main.css", "listing.css", "article.css"},
			"404.css" : []string{"main.css", "error.css"},
			"about.css": []string{"main.css", "about.css"},
			"error.css": []string{"main.css", "error.css"},
			"login.css": []string{"main.css", "login.css"},
			"register.css": []string{"main.css", "login.css"},
			"main.js": []string{"base.js", "main.js"},
		}
		r.Group(func(r chi.Router) {
			r.Get("/ns", assets.ServeStatic(filepath.Join(assetsDir, "/ns.json")))
			r.Get("/favicon.ico", assets.ServeStatic(filepath.Join(assetsDir, "/favicon.ico")))
			r.Get("/icons.svg", assets.ServeStatic(filepath.Join(assetsDir, "/icons.svg")))
			r.Get("/robots.txt", assets.ServeStatic(filepath.Join(assetsDir, "/robots.txt")))
			r.Get("/css/{path}", assets.ServeAsset(styles))
			r.Get("/js/{path}", assets.ServeAsset(styles))
		})
	}
}
