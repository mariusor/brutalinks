package app

import (
	"context"
	"fmt"
	"net/http"

	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/go-chi/chi/v5"
	"github.com/mariusor/qstring"
	"gitlab.com/golang-commonmark/puny"
)

var (
	nilFilter  = EqualsString("-")
	nilFilters = CompStrs{nilFilter}

	notNilFilter  = DifferentThanString("-")
	notNilFilters = CompStrs{notNilFilter}

	derefIRIFilters = &Filters{IRI: notNilFilters}
)

type CompStr = qstring.ComparativeString
type CompStrs []CompStr

func (cs CompStrs) Contains(f CompStr) bool {
	for _, c := range cs {
		if c.Str == f.Str {
			return true
		}
	}
	return false
}

type Filters struct {
	Name       CompStrs `qstring:"name,omitempty"`
	Cont       CompStrs `qstring:"content,omitempty"`
	MedTypes   CompStrs `qstring:"mediaType,omitempty"`
	URL        CompStrs `qstring:"url,omitempty"`
	IRI        CompStrs `qstring:"iri,omitempty"`
	Generator  CompStrs `qstring:"generator,omitempty"`
	Type       CompStrs `qstring:"type,omitempty"`
	AttrTo     CompStrs `qstring:"attributedTo,omitempty"`
	InReplTo   CompStrs `qstring:"inReplyTo,omitempty"`
	OP         CompStrs `qstring:"context,omitempty"`
	Recipients CompStrs `qstring:"recipients,omitempty"`
	Next       string   `qstring:"after,omitempty"`
	Prev       string   `qstring:"before,omitempty"`
	MaxItems   int      `qstring:"maxItems,omitempty"`
	Object     *Filters `qstring:"object,omitempty"`
	Tag        *Filters `qstring:"tag,omitempty"`
	Actor      *Filters `qstring:"actor,omitempty"`
}

// FiltersFromRequest loads the filters we use for generating storage queries from the HTTP request
func FiltersFromRequest(r *http.Request) *Filters {
	f := new(Filters)
	if err := qstring.Unmarshal(r.URL.Query(), f); err != nil {
		return nil
	}
	if f.MaxItems <= 0 {
		f.MaxItems = MaxContentItems
	}
	return f
}

var (
	CreateActivitiesFilter       = ActivityTypesFilter(vocab.CreateType)
	AppreciationActivitiesFilter = ActivityTypesFilter(vocab.LikeType, vocab.DislikeType)
)

func AllFilters(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := defaultFilters(r)
		f.Object = derefIRIFilters
		ctx := context.WithValue(r.Context(), FilterCtxtKey, []*Filters{f})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func DefaultFilters(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := topLevelFilters(r)
		m := ContextListingModel(r.Context())
		m.Title = "Newest items"
		ctx := context.WithValue(r.Context(), FilterCtxtKey, []*Filters{f})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ContextLoads loads the searches we use for generating storage queries from the HTTP request
func ContextLoads(ctx context.Context) RemoteLoads {
	if f, ok := ctx.Value(LoadsCtxtKey).(RemoteLoads); ok {
		return f
	}
	return nil
}

// ContextActivityFilters loads the filters we use for generating storage queries from the HTTP request
func ContextActivityFilters(ctx context.Context) []*Filters {
	if f, ok := ctx.Value(FilterCtxtKey).([]*Filters); ok {
		return f
	}
	return nil
}

func SelfFiltersMw(id vocab.IRI) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			f := topLevelFilters(r)
			f.Actor.IRI = CompStrs{LikeString(id.String())}
			m := ContextListingModel(r.Context())
			m.Title = "Local instance items"
			ctx := context.WithValue(r.Context(), FilterCtxtKey, []*Filters{f})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

var CreateFollowActivitiesFilter = CompStrs{
	CompStr{Str: string(vocab.CreateType)},
	CompStr{Str: string(vocab.FollowType)},
}

func FollowedFiltersMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := new(Filters)
		f.Object = derefIRIFilters
		f.Actor = derefIRIFilters
		f.Type = CreateFollowActivitiesFilter
		m := ContextListingModel(r.Context())
		m.Title = "Followed items"
		m.ShowText = true
		ctx := context.WithValue(r.Context(), FilterCtxtKey, []*Filters{f})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func DifferentThanString(s string) CompStr {
	return CompStr{Operator: "!", Str: s}
}

func defaultFilters(r *http.Request) *Filters {
	f := FiltersFromRequest(r)
	f.Type = CreateActivitiesFilter
	f.Object = new(Filters)
	f.Object.Type = ActivityTypesFilter(ValidContentTypes...)
	f.Actor = derefIRIFilters
	return f
}

func topLevelFilters(r *http.Request) *Filters {
	f := defaultFilters(r)
	f.Object.Name = notNilFilters
	f.Object.InReplTo = nilFilters
	return f
}

func FederatedFiltersMw(id vocab.IRI) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			f := topLevelFilters(r)
			f.IRI = CompStrs{DifferentThanString(id.String())}
			m := ContextListingModel(r.Context())
			m.Title = "Federated items"
			ctx := context.WithValue(r.Context(), FilterCtxtKey, []*Filters{f})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func LikeString(s string) CompStr {
	return CompStr{Operator: "~", Str: s}
}

func DomainFiltersMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		domain := chi.URLParam(r, "domain")
		f := FiltersFromRequest(r)
		f.Type = CreateActivitiesFilter
		f.Object = &Filters{}
		m := ContextListingModel(r.Context())
		if len(domain) > 0 {
			domainFilter := fmt.Sprintf("https://%s", puny.ToASCII(domain))
			f.Object.URL = CompStrs{LikeString(domainFilter), LikeString(domainFilter)}
			f.Object.Type = CompStrs{EqualsString(string(vocab.PageType))}
			m.Title = fmt.Sprintf("Items pointing to %s", domain)
		} else {
			f.Object.MedTypes = CompStrs{
				EqualsString(MimeTypeMarkdown),
				EqualsString(MimeTypeText),
				EqualsString(MimeTypeHTML),
			}
			f.Object.Type = ActivityTypesFilter(ValidContentTypes...)
			m.Title = fmt.Sprintf("Discussion items")
		}
		f.Object.OP = nilFilters
		f.Actor = derefIRIFilters
		ctx := context.WithValue(r.Context(), FilterCtxtKey, []*Filters{f})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func tagsFilter(tag string) *Filters {
	f := new(Filters)
	f.Name = CompStrs{EqualsString("#" + tag)}
	return f
}

func TagFiltersMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tag := chi.URLParam(r, "tag")
		if len(tag) == 0 {
			ctxtErr(next, w, r, errors.NotFoundf("tag not found"))
			return
		}
		fc := new(Filters)
		fc.MaxItems = MaxContentItems
		fc.Type = CreateActivitiesFilter
		fc.Object = new(Filters)
		fc.Object.Tag = tagsFilter(tag)

		fa := new(Filters)
		fa.MaxItems = MaxContentItems
		fa.Type = ModerationActivitiesFilter
		fa.Tag = tagsFilter(tag)
		fa.Object = derefIRIFilters

		allFilters := []*Filters{fc, fa}

		m := ContextListingModel(r.Context())
		m.ShowText = true
		m.Title = fmt.Sprintf("Items tagged as #%s", tag)
		ctx := context.WithValue(r.Context(), FilterCtxtKey, allFilters)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h handler) ItemFiltersMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = CreateActivitiesFilter
		hash := chi.URLParam(r, "hash")
		f.MaxItems = 1

		m := ContextContentModel(r.Context())
		m.Hash = HashFromString(hash)
		if !m.Hash.IsValid() {
			h.v.HandleErrors(w, r, errors.NotFoundf("%q item", hash))
			return
		}

		f.Object = &Filters{IRI: CompStrs{LikeString(hash)}}
		f.Actor = derefIRIFilters
		ctx := context.WithValue(r.Context(), FilterCtxtKey, []*Filters{f})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func MessageFiltersMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authors := ContextAuthors(r.Context())
		if len(authors) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		f := FiltersFromRequest(r)
		if len(f.IRI) > 0 {
			for _, author := range authors {
				f.AttrTo = append(f.AttrTo, EqualsString(author.AP().GetID().String()))
			}
			f.Type = append(CreateActivitiesFilter, AppreciationActivitiesFilter...)
			f.Actor = derefIRIFilters
		}
		ctx := context.WithValue(r.Context(), FilterCtxtKey, []*Filters{f})
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}

type moderationFilter struct {
	Mod  []string `qstring:"m"`
	Type []string `qstring:"t"`
}

var (
	modSubmissionsObjectFilter = &Filters{
		Type:     ActivityTypesFilter(ValidContentTypes...),
		InReplTo: nilFilters,
	}
	modCommentsObjectFilter = &Filters{
		Type:     ActivityTypesFilter(ValidContentTypes...),
		InReplTo: notNilFilters,
	}
	modAccountsObjectFilter = &Filters{
		Type: ActivityTypesFilter(ValidActorTypes...),
	}
)

func ModerationFiltersMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = ModerationActivitiesFilter
		f.IRI = CompStrs{LikeString(chi.URLParam(r, "hash"))}
		f.MaxItems = 1
		f.Object = derefIRIFilters
		f.Actor = derefIRIFilters

		ctx := context.WithValue(r.Context(), FilterCtxtKey, []*Filters{f})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ModerationListingFiltersMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = ModerationActivitiesFilter
		f.Object = derefIRIFilters
		f.Actor = derefIRIFilters
		f.MaxItems = MaxContentItems

		mf := new(moderationFilter)
		qstring.Unmarshal(r.URL.Query(), mf)
		allFilters := make([]*Filters, 0)
		showSubmissions := stringInSlice(mf.Type)("s")
		showComments := stringInSlice(mf.Type)("c")
		showUsers := stringInSlice(mf.Type)("a")
		if len(mf.Type) > 0 && !(showSubmissions == showComments && showSubmissions == showUsers) {
			if showSubmissions {
				fs := *f
				fs.Object = modSubmissionsObjectFilter
				allFilters = append(allFilters, &fs)
			}
			if showComments {
				fc := *f
				fc.Object = modCommentsObjectFilter
				allFilters = append(allFilters, &fc)
			}
			if showUsers {
				fu := *f
				fu.Object = modAccountsObjectFilter
				allFilters = append(allFilters, &fu)
			}
		} else {
			allFilters = append(allFilters, f)
		}
		m := ContextListingModel(r.Context())
		m.Title = "Moderation log"
		ctx := context.WithValue(r.Context(), FilterCtxtKey, allFilters)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ModerationListing(next http.Handler) http.Handler {
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

		next.ServeHTTP(w, r)
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

func ActorsFiltersMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = CreateActivitiesFilter
		f.Object = &Filters{Type: ActivityTypesFilter(ValidActorTypes...)}
		f.Actor = derefIRIFilters
		m := ContextListingModel(r.Context())
		m.Title = "Account listing"
		ctx := context.WithValue(r.Context(), FilterCtxtKey, []*Filters{f})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
