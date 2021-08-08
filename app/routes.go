package app

import (
	"net/http"

	"github.com/go-ap/errors"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mariusor/go-littr/internal/assets"
	"github.com/mariusor/go-littr/internal/config"
)

var assetFiles = assets.AssetFiles{
	"moderate.css":     {"css/main.css", "css/listing.css", "css/article.css", "css/moderate.css", "css/user.css"},
	"content.css":      {"css/main.css", "css/article.css", "css/content.css"},
	"listing.css":      {"css/main.css", "css/listing.css", "css/article.css", "css/moderate.css"},
	"moderation.css":   {"css/main.css", "css/listing.css", "css/article.css", "css/moderation.css"},
	"user.css":         {"css/main.css", "css/listing.css", "css/article.css", "css/user.css"},
	"user-message.css": {"css/main.css", "css/listing.css", "css/article.css", "css/user-message.css"},
	"new.css":          {"css/main.css", "css/listing.css", "css/article.css"},
	"404.css":          {"css/main.css", "css/error.css"},
	"about.css":        {"css/main.css", "css/about.css"},
	"error.css":        {"css/main.css", "css/error.css"},
	"login.css":        {"css/main.css", "css/login.css"},
	"register.css":     {"css/main.css", "css/login.css"},
	"inline.css":       {"css/inline.css"},
	"main.js":          {"js/base.js", "js/main.js"},
	"robots.txt":       {"robots.txt"},
	"ns":               {"ns.json"},
	"favicon.ico":      {"favicon.ico"},
	"icons.svg":        {"icons.svg"},
}

func (h *handler) ItemRoutes() func(chi.Router) {
	return func(r chi.Router) {
		r.Use(h.CSRF, ContentModelMw, h.ItemFiltersMw, LoadObjectFromInboxMw, ThreadedListingMw, SortByScore)
		r.Get("/", h.HandleShow)
		r.With(h.ValidateLoggedIn(h.v.RedirectToErrors)).Post("/", h.HandleSubmit)

		r.Group(func(r chi.Router) {
			r.Use(h.ValidateLoggedIn(h.v.RedirectToErrors))
			r.Get("/yay", h.HandleVoting)
			r.Get("/nay", h.HandleVoting)

			//r.Get("/bad", h.ShowReport)
			r.With(ReportContentModelMw).Get("/bad", h.HandleShow)
			r.Post("/bad", h.ReportItem)
			r.With(BlockContentModelMw).Get("/block", h.HandleShow)
			r.Post("/block", h.BlockItem)

			r.Group(func(r chi.Router) {
				r.With(h.ValidateItemAuthor("edit"), EditContentModelMw).Get("/edit", h.HandleShow)
				r.With(h.ValidateItemAuthor("edit")).Post("/edit", h.HandleSubmit)
				r.With(h.ValidateItemAuthor("delete")).Get("/rm", h.HandleDelete)
			})
		})
	}
}

func (h *handler) Routes(c *config.Configuration) func(chi.Router) {
	h.v.assets = assetFiles

	return func(r chi.Router) {
		r.Use(ReqLogger(h.logger))
		r.Use(OutOfOrderMw(h.v))
		r.Use(middleware.GetHead)

		r.Group(func(r chi.Router) {
			//r.Use(middleware.Timeout(60 * time.Millisecond))
			r.Use(h.SetSecurityHeaders)
			r.Use(h.LoadSession)

			usersEnabledFn := func() (bool, string) {
				return c.UserCreatingEnabled, "Account creation is disabled"
			}
			usersInvitesFn := func() (bool, string) {
				return c.UserInvitesEnabled, "Account invites are disabled"
			}
			usersEnabledOrInvitesFn := func() (bool, string) {
				return c.UserInvitesEnabled || c.UserCreatingEnabled, "Unable to create account"
			}
			r.With(h.CSRF).Group(func(r chi.Router) {
				r.With(AddModelMw).Get("/submit", h.HandleShow)
				r.Post("/submit", h.HandleSubmit)
				r.Route("/register", func(r chi.Router) {
					r.Group(func(r chi.Router) {
						r.With(h.v.RedirectWithFailMessage(usersEnabledFn), ModelMw(&registerModel{Title: "Register new account"})).Get("/", h.HandleShow)
						r.With(h.v.RedirectWithFailMessage(usersInvitesFn), ModelMw(&registerModel{Title: "Register account from invite"}), LoadInvitedMw).Get("/{hash}", h.HandleShow)
					})
					r.With(h.v.RedirectWithFailMessage(usersEnabledOrInvitesFn)).Post("/", h.HandleRegister)
				})
				r.With(h.NeedsSessions).Group(func(r chi.Router) {
					r.With(ModelMw(&loginModel{Title: "Local authentication"})).Get("/login", h.HandleShow)
					r.Post("/login", h.HandleLogin)
				})
			})

			r.With(h.LoadAuthorMw).Route("/~{handle}", func(r chi.Router) {
				r.With(AccountListingModelMw, AccountFiltersMw, LoadOutboxMw).Get("/", h.HandleShow)

				r.Group(func(r chi.Router) {
					r.Use(h.ValidateLoggedIn(h.v.RedirectToErrors))
					r.Get("/follow", h.FollowAccount)
					r.Get("/follow/{action}", h.HandleFollowRequest)
					r.With(h.NeedsSessions, h.ValidateLoggedIn(h.v.RedirectToErrors)).Post("/invite", h.HandleCreateInvitation)

					r.With(h.CSRF, MessageUserContentModelMw).Group(func(r chi.Router) {
						r.Route("/message", func(r chi.Router) {
							r.Get("/", h.HandleShow)
							r.Post("/", h.HandleSubmit)
						})

						r.With(BlockAccountModelMw).Get("/block", h.HandleShow)
						r.Post("/block", h.BlockAccount)
						r.With(ReportAccountModelMw).Get("/bad", h.HandleShow)
						r.Post("/bad", h.ReportAccount)
					})
				})

				r.Route("/{hash}", h.ItemRoutes())
			})
			r.Route("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}", h.ItemRoutes())

			// @todo(marius) :link_generation:
			r.With(ContentModelMw, h.ItemFiltersMw, LoadObjectFromInboxMw).Get("/i/{hash}", h.HandleItemRedirect)

			r.With(h.NeedsSessions).Get("/logout", h.HandleLogout)

			r.With(ListingModelMw).Group(func(r chi.Router) {
				// todo(marius) :link_generation:
				r.With(DefaultFilters, LoadServiceInboxMw, SortByScore).Get("/", h.HandleShow)
				r.With(DomainFiltersMw, LoadServiceInboxMw, middleware.StripSlashes, SortByDate).Get("/d", h.HandleShow)
				r.With(DomainFiltersMw, LoadServiceInboxMw, SortByDate).Get("/d/{domain}", h.HandleShow)
				r.With(TagFiltersMw, LoadServiceInboxMw, ModerationListing, SortByDate).Get("/t/{tag}", h.HandleShow)
				r.With(SelfFiltersMw(h.storage.fedbox.Service().ID), LoadServiceInboxMw, SortByScore).Get("/self", h.HandleShow)
				r.With(FederatedFiltersMw(h.storage.fedbox.Service().ID), LoadServiceInboxMw, SortByScore).Get("/federated", h.HandleShow)
				r.With(h.NeedsSessions, FollowedFiltersMw, h.ValidateLoggedIn(h.v.RedirectToErrors), LoadInboxMw, SortByDate).
					Get("/followed", h.HandleShow)
				r.With(ModelMw(&listingModel{tpl: "moderation", sortFn: ByDate}), ModerationFiltersMw, LoadServiceWithSelfAuthInboxMw, ModerationListing).
					Get("/moderation", h.HandleShow)
				r.With(ModelMw(&listingModel{tpl: "listing", sortFn: ByDate}), ActorsFiltersMw, LoadServiceInboxMw, ThreadedListingMw).
					Get("/~", h.HandleShow)
			})

			r.Get("/about", h.HandleAbout)
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

		r.Group(func(r chi.Router) {
			r.Get("/{path}", assets.ServeAsset(h.v.assets))
			r.Get("/css/{path}", assets.ServeAsset(h.v.assets))
			r.Get("/js/{path}", assets.ServeAsset(h.v.assets))
		})
	}
}
