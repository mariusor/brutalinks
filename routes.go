package brutalinks

import (
	"net/http"

	ass "git.sr.ht/~mariusor/assets"
	"git.sr.ht/~mariusor/brutalinks/internal/assets"
	"git.sr.ht/~mariusor/brutalinks/internal/config"
	"git.sr.ht/~mariusor/lw"
	"github.com/go-ap/errors"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

var basicStyles = []string{"css/reset.css", "css/main.css", "css/header.css", "css/footer.css", "css/s.css"}
var assetFiles = ass.Map{
	"/css/moderate.css":     append(basicStyles, "css/listing.css", "css/content.css", "css/article.css", "css/moderate.css"),
	"/css/content.css":      append(basicStyles, "css/article.css", "css/listing.css", "css/threaded.css", "css/content.css"),
	"/css/accounts.css":     append(basicStyles, "css/listing.css", "css/threaded.css", "css/accounts.css"),
	"/css/listing.css":      append(basicStyles, "css/listing.css", "css/article.css", "css/threaded.css", "css/moderate.css"),
	"/css/moderation.css":   append(basicStyles, "css/listing.css", "css/article.css", "css/threaded.css", "css/moderation.css"),
	"/css/user.css":         append(basicStyles, "css/listing.css", "css/article.css", "css/user.css"),
	"/css/user-message.css": append(basicStyles, "css/listing.css", "css/article.css", "css/user-message.css"),
	"/css/new.css":          append(basicStyles, "css/listing.css", "css/article.css"),
	"/css/404.css":          append(basicStyles, "css/article.css", "css/error.css"),
	"/css/about.css":        append(basicStyles, "css/article.css", "css/about.css"),
	"/css/error.css":        append(basicStyles, "css/error.css"),
	"/css/login.css":        append(basicStyles, "css/login.css"),
	"/css/register.css":     append(basicStyles, "css/login.css"),
	"/css/inline.css":       {"css/inline.css"},
	"/css/simple.css":       {"css/simple.css"},
	"/js/main.js":           {"js/base.js", "js/main.js"},
	"/robots.txt":           {"robots.txt"},
	"/favicon.ico":          {"favicon.ico"},
	"/icons.svg":            {"icons.svg"},
}

var (
	instanceSearchFns        = instanceSearches(inbox, outbox)
	applicationInboxSearchFn = applicationSearches(inbox)
	applicationSearchFns     = applicationSearches(inbox, outbox)
	loggedAccountSearchFns   = loggedAccountSearches(inbox, outbox)
	namedAccountSearchFns    = namedAccountSearches(inbox, outbox)
)

func (h *handler) ItemRoutes(extra ...func(http.Handler) http.Handler) func(chi.Router) {
	return func(r chi.Router) {
		r.Use(extra...)
		r.Use(ContentModelMw, h.ItemChecks, LoadSingleObjectMw, SingleItemModelMw)
		r.With(Deps(Votes, Replies, Authors), LoadSingleItemMw, SortByScore).
			Get("/", h.HandleShow)
		r.With(h.ValidateLoggedIn(h.v.RedirectToErrors), LoadSingleItemMw).Post("/", h.HandleSubmit)

		r.Group(func(r chi.Router) {
			r.Use(h.ValidateLoggedIn(h.v.RedirectToErrors))
			r.Get("/yay", h.HandleVoting)
			r.Get("/nay", h.HandleVoting)

			//r.Get("/bad", h.ShowReport)
			r.With(Deps(Votes, Authors), LoadSingleItemMw, ReportContentModelMw).Get("/bad", h.HandleShow)
			r.Post("/bad", h.ReportItem)
			r.With(Deps(Votes, Authors), LoadSingleItemMw, BlockContentModelMw).Get("/block", h.HandleShow)
			r.Post("/block", h.BlockItem)

			r.Group(func(r chi.Router) {
				r.With(h.ValidateItemAuthor("edit"), LoadSingleItemMw, EditContentModelMw).Get("/edit", h.HandleShow)
				r.With(h.ValidateItemAuthor("edit"), LoadSingleItemMw).Post("/edit", h.HandleSubmit)
				r.With(h.ValidateItemAuthor("delete")).Get("/rm", h.HandleDelete)
			})
		})
	}
}

func (h *handler) Routes(c *config.Configuration) func(chi.Router) {
	h.v.assets = assetFiles

	if assetFs, err := ass.New(assets.AssetFS); err == nil {
		_ = assetFs.Overlay(assetFiles)
		h.v.fs = assetFs
	} else {
		h.errFn()("%s: %s", assets.AssetFS, err)
	}

	csrf := h.CSRF()
	return func(r chi.Router) {
		r.Use(lw.Middlewares(h.logger)...)
		r.Use(middleware.GetHead)

		r.Group(func(r chi.Router) {
			r.Use(OutOfOrderMw(h.v))
			r.Use(h.v.SetSecurityHeaders)
			r.Use(h.v.LoadSession)

			submissionsEnabledFn := func(r *http.Request) (bool, string) {
				return c.AnonymousCommentingEnabled || loggedAccount(r).IsLogged(), "Anonymous submissions are disabled"
			}
			usersEnabledFn := func(_ *http.Request) (bool, string) {
				return c.UserCreatingEnabled, "Account creation is disabled"
			}
			usersInvitesFn := func(_ *http.Request) (bool, string) {
				return c.UserInvitesEnabled, "Account invites are disabled"
			}
			usersEnabledOrInvitesFn := func(_ *http.Request) (bool, string) {
				return c.UserInvitesEnabled || c.UserCreatingEnabled, "Unable to create account"
			}
			r.With(csrf).Group(func(r chi.Router) {
				r.With(AddModelMw, h.v.RedirectWithFailMessage(submissionsEnabledFn)).Get("/submit", h.HandleShow)
				r.With(h.v.RedirectWithFailMessage(submissionsEnabledFn)).Post("/submit", h.HandleSubmit)
				r.Route("/register", func(r chi.Router) {
					r.Group(func(r chi.Router) {
						r.With(h.v.RedirectWithFailMessage(usersEnabledFn), ModelMw(&registerModel{Title: "Register new account"})).
							Get("/", h.HandleShow)
						r.With(h.v.RedirectWithFailMessage(usersInvitesFn), ModelMw(&registerModel{Title: "Register account from invite"}), LoadInvitedMw).
							Get("/{hash}", h.HandleShow)
					})
					r.With(h.v.RedirectWithFailMessage(usersEnabledOrInvitesFn)).Post("/", h.HandleRegister)
				})
				r.With(h.NeedsSessions).Group(func(r chi.Router) {
					r.With(ModelMw(&loginModel{Title: "Authentication", Provider: fedboxProvider})).Get("/login", h.HandleShow)
					r.Post("/login", h.HandleLogin)
				})
			})
			r.With(h.ValidateLoggedIn(h.v.RedirectToErrors), Deps(Authors, Follows), FollowFilterMw,
				SearchInCollectionsMw(requestHandleSearches, inbox), applicationSearches(inbox), OperatorSearches, LoadMw).
				Get("/follow/{hash}/{action}", h.HandleFollowResponseRequest)

			r.With(h.LoadAuthorMw, SearchInCollectionsMw(requestHandleSearches, outbox), LoadMw).
				Get("/~{handle}.pub", h.ShowPublicKey)

			r.With(h.LoadAuthorMw).Route("/~{handle}", func(r chi.Router) {
				r.With(AccountListingModelMw, AllFilters, Deps(Authors, Votes), SearchInCollectionsMw(requestHandleSearches, outbox), LoadMw).
					Get("/", h.HandleShow)

				r.With(csrf).Route("/changepw/{hash}", func(r chi.Router) {
					r.With(ModelMw(&registerModel{Title: "Change password"}), LoadInvitedMw).Get("/", h.HandleShow)
					r.Post("/", h.HandleChangePassword)
				})
				r.Group(func(r chi.Router) {
					r.Use(h.ValidateLoggedIn(h.v.RedirectToErrors))
					r.Get("/follow", h.FollowAccount)
					r.With(h.NeedsSessions, h.ValidateLoggedIn(h.v.RedirectToErrors)).Post("/invite", h.HandleCreateInvitation)

					r.With(csrf, MessageUserContentModelMw).Group(func(r chi.Router) {
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

				r.Route("/{hash}", h.ItemRoutes(csrf))
			})
			r.Route("/{year:[0-9]{4}}/{month:[0-9]{2}}/{day:[0-9]{2}}/{hash}", h.ItemRoutes(csrf))

			// @todo(marius) :link_generation:
			r.With(ContentModelMw, h.ItemChecks, LoadSingleObjectMw).
				Get("/i/{hash}", h.HandleItemRedirect)

			r.With(h.NeedsSessions).Get("/logout", h.HandleLogout)

			r.With(ListingModelMw, Deps(Authors, Votes)).Group(func(r chi.Router) {
				// todo(marius) :link_generation:
				r.With(DefaultChecks, LoadV2Mw, SortByScore).Get("/", h.HandleShow)

				r.With(DomainChecksMw, LoadV2Mw, middleware.StripSlashes, SortByDate).Get("/d", h.HandleShow)

				r.With(DomainChecksMw, LoadV2Mw, SortByDate).Get("/d/{domain}", h.HandleShow)

				r.With(TagChecks, LoadV2Mw, Deps(Moderations), h.ModerationListing, SortByDate).
					Get("/t/{tag}", h.HandleShow)

				r.With(SelfChecks(h.storage.fedbox.Service().ID), LoadV2Mw, SortByScore).Get("/self", h.HandleShow)

				r.With(FederatedFiltersMw(h.storage.fedbox.Service().ID), applicationInboxSearchFn, LoadMw, SortByScore).
					Get("/federated", h.HandleShow)

				r.With(h.NeedsSessions, h.ValidateLoggedIn(h.v.RedirectToErrors), Deps(Follows), FollowedFiltersMw,
					loggedAccountSearches(inbox), LoadMw, SortByDate).
					Get("/followed", h.HandleShow)

				r.Route("/moderation", func(r chi.Router) {
					r.With(ModelMw(&listingModel{tpl: "moderation", sortFn: ByDate}), Deps(Moderations, Follows),
						ModerationListingFiltersMw, LoadV2Mw, h.ModerationListing).Get("/", h.HandleShow)
					r.With(h.ValidateModerator(), ModerationFiltersMw, LoadV2Mw).Group(func(r chi.Router) {
						r.Get("/{hash}/rm", h.HandleModerationDelete)
						r.Get("/{hash}/discuss", h.HandleShow)
					})
				})

				r.With(ModelMw(&listingModel{ShowChildren: true, sortFn: ByDate}), ActorsFiltersMw, LoadV2Mw).
					Get("/~", h.HandleShow)
			})

			r.Post("/follow", h.HandleFollowInstanceRequest)
			r.Get("/about", h.HandleAbout)
			r.Route("/auth", func(r chi.Router) {
				r.Use(h.NeedsSessions)
				r.Get("/{provider}/callback", h.HandleCallback)
			})
		})

		r.Group(func(r chi.Router) {
			r.Get("/{path}", h.v.assetHandler)
			r.Get("/css/{path}", h.v.assetHandler)
			r.Get("/js/{path}", h.v.assetHandler)
		})
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			h.v.HandleErrors(w, r, errors.NotFoundf("%q", r.RequestURI))
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			h.v.HandleErrors(w, r, errors.MethodNotAllowedf("invalid %q request", r.Method))
		})

		if c.Env.IsDev() {
			r.With(middleware.BasicAuth("debug", map[string]string{"debug": "#debug$"})).Mount("/debug", middleware.Profiler())
		}
	}
}
