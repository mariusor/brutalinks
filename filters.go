package brutalinks

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/go-ap/filters"
	"github.com/go-ap/filters/index"
	"github.com/go-chi/chi/v5"
	"github.com/mariusor/qstring"
	"gitlab.com/golang-commonmark/puny"
)

func AuthorChecks(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := ContextAuthors(r.Context())
		checks := defaultChecks(r)
		for _, a := range auth {
			checks = append(checks, filters.SameAttributedTo(a.AP().GetLink()))
		}
		ctx := context.WithValue(r.Context(), FilterCtxtKey, filters.All(checks...))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func DefaultChecks(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checks := topLevelChecks(r)
		m := ContextListingModel(r.Context())
		m.Title = "Newest items"
		ctx := context.WithValue(r.Context(), FilterCtxtKey, filters.All(checks...))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ContextActivityChecks loads the filters we use for generating storage queries from the HTTP request
func ContextActivityChecks(ctx context.Context) filters.Check {
	if f, ok := ctx.Value(FilterCtxtKey).(filters.Check); ok {
		return f
	}
	return nil
}

func SelfChecks(id vocab.IRI) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m := ContextListingModel(r.Context())
			m.Title = "Local instance items"

			checks := append(topLevelChecks(r), filters.IRILike(id.String()))
			ctx := context.WithValue(r.Context(), FilterCtxtKey, filters.All(checks...))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func FollowChecks(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checks := filters.All(
			filters.HasType(vocab.FollowType),
			filters.IRILike(chi.URLParam(r, "hash")),
		)

		ctx := context.WithValue(r.Context(), FilterCtxtKey, checks)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func FollowedChecks(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m := ContextListingModel(r.Context()); m != nil {
			m.Title = "Followed items"
			m.ShowText = true
		}

		// TODO(marius): this needs more work, as just the recipients list is not enough
		//  for establishing if an object should be in the Followed tab.
		//  We probably need to fetch everything in the actor's outbox collection and work from there.
		loggedUser := loggedAccount(r)
		validTypes := append(ValidContentTypes, vocab.FollowType)
		check := filters.All(
			filters.HasType(validTypes...),
			filters.Recipients(loggedUser.AP().GetLink()),
			filters.Not(filters.SameAttributedTo(loggedUser.AP().GetLink())),
		)
		ctx := context.WithValue(r.Context(), FilterCtxtKey, check)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

const (
	refBase   = 36
	keyAfter  = "after"
	keyBefore = "before"
)

type cursorRef uint64

func (c cursorRef) Match(it vocab.Item) bool {
	ref := index.HashFn(it.GetLink())
	return uint64(c) == ref
}

func (c cursorRef) String() string {
	return strconv.FormatUint(uint64(c), refBase)
}

var _ filters.Check = cursorRef(0)

func RefValue(r uint64) cursorRef {
	return cursorRef(r)
}

func IRIRef(i vocab.IRI) cursorRef {
	return cursorRef(index.HashFn(i))
}

func FromURL(u url.URL) filters.Checks {
	q := u.Query()
	appendChecks := func(checks *filters.Checks, check func(...filters.Check) filters.Check, vv []string) {
		for _, v := range vv {
			if r, err := strconv.ParseUint(v, refBase, 64); err == nil {
				*checks = append(*checks, check(RefValue(r)))
			}
		}
	}
	checks := make(filters.Checks, 0, len(q))
	for k, vv := range q {
		if k == keyAfter {
			appendChecks(&checks, filters.After, vv)
			q.Del(k)
		}
		if k == keyBefore {
			appendChecks(&checks, filters.Before, vv)
			q.Del(k)
		}
	}
	return append(filters.FromValues(q), checks...)
}

func requestChecks(r *http.Request) filters.Checks {
	return append(FromURL(*r.URL), filters.WithMaxCount(MaxContentItems))
}

func defaultChecks(r *http.Request) filters.Checks {
	return append(requestChecks(r), filters.Recipients(vocab.PublicNS))
}

func contentChecks(r *http.Request) filters.Checks {
	return append(defaultChecks(r), filters.HasType(ValidContentTypes...))
}

func topLevelChecks(r *http.Request) filters.Checks {
	return append(contentChecks(r), filters.Not(filters.NameEmpty), filters.NilInReplyTo)
}

func FederatedChecks(id vocab.IRI) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m := ContextListingModel(r.Context())
			m.Title = "Federated items"

			checks := append(topLevelChecks(r), filters.Not(filters.IRILike(id.String())))
			ctx := context.WithValue(r.Context(), FilterCtxtKey, filters.All(checks...))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func DomainChecksMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		domain := chi.URLParam(r, "domain")
		m := ContextListingModel(r.Context())
		checks := defaultChecks(r)
		checks = append(checks, filters.NilInReplyTo, filters.Not(filters.NameEmpty))
		if len(domain) > 0 {
			m.Title = htmlf("Items pointing to %s", domain)
			domainName := fmt.Sprintf("https://%s", puny.ToASCII(domain))
			checks = append(checks, filters.HasType(vocab.PageType), filters.URLLike(domainName))
		} else {
			m.Title = htmlf("Discussion items")
			// TODO(marius): add filters.MediaTypeXXX to support
			//   filtering by MimeTypeMarkdown, MimeTypeText, MimeTypeHTML
			checks = append(checks, filters.HasType(vocab.ArticleType, vocab.NoteType))
		}
		ctx := context.WithValue(r.Context(), FilterCtxtKey, filters.All(checks...))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func TagChecks(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tag := chi.URLParam(r, "tag")
		if len(tag) == 0 {
			ctxtErr(next, w, r, errors.NotFoundf("tag not found"))
			return
		}

		validTypes := append(append(ValidContentTypes, ValidModerationActivityTypes...), ValidActorTypes...)
		checks := append(requestChecks(r),
			filters.HasType(validTypes...),
			filters.Tag(filters.NameIs("#"+tag)),
			filters.WithMaxCount(MaxContentItems),
		)

		m := ContextListingModel(r.Context())
		m.ShowText = true
		m.Title = htmlf("Items tagged as #%s", tag)
		ctx := context.WithValue(r.Context(), FilterCtxtKey, filters.All(checks...))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ItemChecks(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hash := chi.URLParam(r, "hash")

		checks := append(defaultChecks(r),
			filters.Recipients(vocab.PublicNS),
			filters.HasType(ValidContentTypes...),
			filters.IRILike(hash),
		)
		ctx := context.WithValue(r.Context(), FilterCtxtKey, filters.All(checks...))

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type moderationFilter struct {
	Mod  []string `qstring:"m"`
	Type []string `qstring:"t"`
}

func ModerationChecks(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checks := append(requestChecks(r),
			filters.IRILike(chi.URLParam(r, "hash")),
			filters.HasType(ValidModerationActivityTypes...),
		)

		ctx := context.WithValue(r.Context(), FilterCtxtKey, filters.All(checks...))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ModerationListingChecks(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mf := new(moderationFilter)
		_ = qstring.Unmarshal(r.URL.Query(), mf)

		showSubmissions := stringInSlice(mf.Type)("s")
		showComments := stringInSlice(mf.Type)("c")
		showUsers := stringInSlice(mf.Type)("a")

		checks := append(requestChecks(r), filters.HasType(ValidModerationActivityTypes...))
		if len(mf.Type) > 0 && !(showSubmissions == showComments && showSubmissions == showUsers) {
			if showSubmissions {
				checks = append(checks, filters.Object(
					filters.HasType(ValidContentTypes...),
					filters.NilInReplyTo,
				))
			}
			if showComments {
				checks = append(checks, filters.Object(
					filters.HasType(ValidContentTypes...),
					filters.Not(filters.NilInReplyTo),
				))
			}
			if showUsers {
				checks = append(checks, filters.Object(
					filters.HasType(ValidActorTypes...),
				))
			}
		}
		if m := ContextListingModel(r.Context()); m != nil {
			m.Title = "Moderation log"
		}
		ctx := context.WithValue(r.Context(), FilterCtxtKey, filters.All(checks...))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h handler) ModerationListing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := ContextCursor(r.Context())
		if c == nil {
			next.ServeHTTP(w, r)
			return
		}

		s := ContextRepository(r.Context())
		if s == nil {
			next.ServeHTTP(w, r)
			return
		}
		followups, _ := s.loadModerationFollowups(r.Context(), c.items)
		if withFollowups := aggregateModeration(c.items, followups); len(withFollowups) > 0 {
			c.items = withFollowups
		}

		defer next.ServeHTTP(w, r)

		if Instance.Conf.AutoAcceptFollows {
			fol, err := vocab.ToActor(s.app.AP())
			if err != nil || fol.PublicKey.ID == "" {
				return
			}

			for _, ren := range c.items {
				maybeFollow, ok := ren.(*FollowRequest)
				if !ok || maybeFollow == nil {
					continue
				}
				follow := maybeFollow.AP()
				if follow == nil {
					continue
				}
				if !accountsEqual(*s.app, *maybeFollow.Object) {
					continue
				}
				followerIRI := maybeFollow.SubmittedBy.Metadata.URL
				if follow.GetType() != vocab.FollowType || AccountIsFollowed(s.app, maybeFollow.SubmittedBy) {
					continue
				}

				if err = s.SendFollowResponse(r.Context(), *maybeFollow, true, nil); err != nil {
					h.v.addFlashMessage(Error, w, r, fmt.Sprintf("Unable to accept the follow request from %s", followerIRI))
					return
				}
				s.app.Followers = append(s.app.Followers, *maybeFollow.SubmittedBy)
				h.v.addFlashMessage(Success, w, r, fmt.Sprintf("Successfully accepted the follow request from %s", followerIRI))
			}
		}
	})
}

func LoadInvitedMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hash := chi.URLParam(r, "hash")
		if len(hash) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		s := ContextRepository(r.Context())
		if s == nil {
			next.ServeHTTP(w, r)
			return
		}
		a, err := s.LoadAccount(r.Context(), actors.IRI(s.fedbox.Service()).AddPath(hash))
		if err != nil {
			ctxtErr(next, w, r, err)
			return
		}
		if m := ContextRegisterModel(r.Context()); a.IsValid() && m != nil {
			m.Account = *a
		}
		next.ServeHTTP(w, r)
	})
}

func ActorsChecks(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := ContextListingModel(r.Context())
		m.Title = "Account listing"
		checks := filters.HasType(ValidActorTypes...)
		ctx := context.WithValue(r.Context(), FilterCtxtKey, checks)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
