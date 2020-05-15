package app

import (
	"context"
	"fmt"
	pub "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/go-chi/chi"
	"github.com/mariusor/qstring"
	"net/http"
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
	Actor      *Filters `qstring:"actor,omitempty"`
}

// FiltersFromRequest loads the filters we use for generating storage queries from the HTTP request
func FiltersFromRequest(r *http.Request) *Filters {
	f := Filters{}
	if err := qstring.Unmarshal(r.URL.Query(), &f); err != nil {
		return nil
	}
	if f.MaxItems == 0 {
		f.MaxItems = MaxContentItems
	}
	return &f
}

var CreateActivitiesFilter = CompStrs{
	CompStr{Str: string(pub.CreateType)},
}

func DefaultFilters(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = CreateActivitiesFilter
		f.Object = &Filters{}
		f.Object.OP = nilIRIs
		f.Object.Type = ActivityTypesFilter(ValidItemTypes...)
		m := ContextListingModel(r.Context())
		m.Title = "Newest items"
		ctx := context.WithValue(context.WithValue(r.Context(), FilterCtxtKey, f), ModelCtxtKey, m)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// FiltersFromRequest loads the filters we use for generating storage queries from the HTTP request
func ContextActivityFilters(ctx context.Context) *Filters {
	if f, ok := ctx.Value(FilterCtxtKey).(*Filters); ok {
		return f
	}
	return nil
}

func SelfFiltersMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = CreateActivitiesFilter
		f.Object = &Filters{}
		f.Object.OP = nilIRIs
		f.Object.Type = ActivityTypesFilter(ValidItemTypes...)
		f.IRI = CompStrs{LikeString(Instance.APIURL)}
		m := ContextListingModel(r.Context())
		m.Title = "Local instance items"
		ctx := context.WithValue(context.WithValue(r.Context(), FilterCtxtKey, f), ModelCtxtKey, m)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

var CreateFollowActivitiesFilter = CompStrs{
	CompStr{Str: string(pub.CreateType)},
	CompStr{Str: string(pub.FollowType)},
}

func FollowedFiltersMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = CreateFollowActivitiesFilter
		m := ContextListingModel(r.Context())
		m.Title = "Followed items"
		ctx := context.WithValue(context.WithValue(r.Context(), FilterCtxtKey, f), ModelCtxtKey, m)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func DifferentThanString(s string) CompStr {
	return CompStr{Operator: "!", Str: s}
}

func FederatedFiltersMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = CreateActivitiesFilter
		f.IRI = CompStrs{DifferentThanString(Instance.APIURL)}
		f.Object = &Filters{}
		f.Object.OP = nilIRIs
		f.Object.Type = ActivityTypesFilter(ValidItemTypes...)
		m := ContextListingModel(r.Context())
		m.Title = "Federated items"
		ctx := context.WithValue(context.WithValue(r.Context(), FilterCtxtKey, f), ModelCtxtKey, m)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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
			f.Object.URL = CompStrs{LikeString(domain)}
			f.Object.Type = CompStrs{EqualsString(string(pub.PageType))}
			m.Title = fmt.Sprintf("Items pointing to %s", domain)
		} else {
			f.Object.MedTypes = CompStrs{
				EqualsString(MimeTypeMarkdown),
				EqualsString(MimeTypeText),
				EqualsString(MimeTypeHTML),
			}
			f.Object.Type = ActivityTypesFilter(ValidItemTypes...)
			m.Title = fmt.Sprintf("Discussion items")
		}
		f.Object.OP = nilIRIs
		ctx := context.WithValue(context.WithValue(r.Context(), FilterCtxtKey, f), ModelCtxtKey, m)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func TagFiltersMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tag := chi.URLParam(r, "tag")
		if len(tag) == 0 {
			ctxtErr(next, w, r, errors.NotFoundf("tag not found"))
			return
		}
		f := FiltersFromRequest(r)
		f.Type = CreateActivitiesFilter
		f.Object = &Filters{}
		f.Object.Type = ActivityTypesFilter(ValidItemTypes...)
		f.Object.Cont = CompStrs{LikeString("#" + tag)}
		m := ContextListingModel(r.Context())
		m.Title = fmt.Sprintf("Items tagged as #%s", tag)
		ctx := context.WithValue(context.WithValue(r.Context(), FilterCtxtKey, f), ModelCtxtKey, m)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ItemFiltersMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = CreateActivitiesFilter
		hash := chi.URLParam(r, "hash")

		m := ContextContentModel(r.Context())
		m.Hash = Hash(hash)

		f.Object = &Filters{}
		f.Object.IRI = CompStrs{LikeString(hash)}
		ctx := context.WithValue(r.Context(), FilterCtxtKey, f)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func AccountFiltersMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = CreateActivitiesFilter

		m := ContextListingModel(r.Context())
		author := ContextAuthor(r.Context())
		if author == nil {
			next.ServeHTTP(w, r)
			return
		}
		f.AttrTo = IRIsFilter(author.pub.GetLink())
		m.Title = fmt.Sprintf("%s items", genitive(author.Handle))
		m.User = author

		ctx := context.WithValue(context.WithValue(r.Context(), FilterCtxtKey, f), ModelCtxtKey, m)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

var ModerationActivitiesFilter = CompStrs{
	CompStr{Str: string(pub.BlockType)},
	CompStr{Str: string(pub.FlagType)},
}

func ModerationFiltersMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = ModerationActivitiesFilter
		m := ContextListingModel(r.Context())
		m.Title = "Moderation log"
		ctx := context.WithValue(context.WithValue(r.Context(), FilterCtxtKey, f), ModelCtxtKey, m)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func AnonymizeListing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

func LoadInvitedMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := chi.URLParam(r, "hash")
		if len(code) == 0 {}
		s := ContextRepository(r.Context())
		if s == nil {
			next.ServeHTTP(w, r)
			return
		}
		f := &Filters{IRI: CompStrs{LikeString(code)}}
		a, err := s.LoadAccount(f)
		if err != nil {
			ctxtErr(next, w, r, err)
			return
		}
		if m := ContextRegisterModel(r.Context()); a.IsValid() && m != nil {
			m.Account = a
		}
		next.ServeHTTP(w, r)
	})
}

var ActorsFilters = CompStrs{
	CompStr{Str: string(pub.PersonType)},
}

func ActorsFiltersMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = CreateActivitiesFilter
		f.Object = &Filters{
			Type: ActorsFilters,
		}
		m := ContextListingModel(r.Context())
		m.Title = "Account listing"
		ctx := context.WithValue(context.WithValue(r.Context(), FilterCtxtKey, f), ModelCtxtKey, m)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
