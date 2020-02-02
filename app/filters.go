package app

import (
	"context"
	pub "github.com/go-ap/activitypub"
	"github.com/go-chi/chi"
	"github.com/mariusor/qstring"
	"net/http"
	"net/url"
	"time"
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

type LoadVotesFilter struct {
	ItemKey              []Hash                      `qstring:"object,omitempty"`
	Type                 pub.ActivityVocabularyTypes `qstring:"type,omitempty"`
	AttributedTo         []Hash                      `qstring:"attributedTo,omitempty"`
	SubmittedAt          time.Time                   `qstring:"submittedAt,omitempty"`
	SubmittedAtMatchType MatchType                   `qstring:"submittedAtMatchType,omitempty"`
}

type LoadAccountsFilter struct {
	Key      []Hash   `qstring:"iri,omitempty"`
	Handle   []string `qstring:"name,omitempty"`
	Email    []string `qstring:"email,omitempty"`
	Deleted  []bool   `qstring:"deleted,omitempty"`
	IRI      string   `qstring:"id,omitempty"`
	InboxIRI string   `qstring:"inbox,omitempty"`
}

type LoadFollowRequestsFilter struct {
	Key   []Hash `qstring:"iri,omitempty"`
	Actor []Hash `qstring:"actor,omitempty"`
	On    []Hash `qstring:"object,omitempty"`
}

type LoadItemsFilter struct {
	Key                  []Hash    `qstring:"iri,omitempty"`
	MediaType            []string  `qstring:"mediaType,omitempty"`
	AttributedTo         []Hash    `qstring:"attributedTo,omitempty"`
	InReplyTo            []string  `qstring:"inReplyTo,omitempty"`
	Context              []string  `qstring:"context,omitempty"`
	SubmittedAt          time.Time `qstring:"submittedAt,omitempty"`
	SubmittedAtMatchType MatchType `qstring:"submittedAtMatchType,omitempty"`
	Content              string    `qstring:"content,omitempty"`
	ContentMatchType     MatchType `qstring:"contentMatchType,omitempty"`
	Deleted              []bool    `qstring:"-"` // used as an array to allow for it to be missing
	IRI                  string    `qstring:"id,omitempty"`
	URL                  string    `qstring:"url,omitempty"`
	Depth                int       `qstring:"depth,omitempty"`
	Federated            []bool    `qstring:"-"` // used as an array to allow for it to be missing
	Private              []bool    `qstring:"-"` // used as an array to allow for it to be missing
	// FollowedBy is the hash or handle of the user of which we should show the list of items that were commented on or liked
	FollowedBy   string `qstring:"followedBy,omitempty"`
	contentAlias string
	authorAlias  string
}

type Filters struct {
	LoadAccountsFilter
	LoadItemsFilter
	LoadVotesFilter
	LoadFollowRequestsFilter
	Page     int `qstring:"page,omitempty"`
	MaxItems int `qstring:"maxItems,omitempty"`
}

func query(f Filterable) string {
	res := ""

	var u url.Values
	var err error
	if u, err = qstring.Marshal(f); err != nil {
		return ""
	}

	if len(u) > 0 {
		res = "?" + u.Encode()
	}
	return res
}

type fedFilters struct {
	Name       CompStrs `qstring:"name,omitempty"`
	Cont       CompStrs `qstring:"content,omitempty"`
	URL        CompStrs `qstring:"url,omitempty"`
	MedTypes   CompStrs `qstring:"mediaType,omitempty"`
	IRI        CompStrs `qstring:"iri,omitempty"`
	ObjectIRI  CompStrs `qstring:"object,omitempty"`
	Generator  CompStrs `qstring:"generator,omitempty"`
	ActorKey   CompStrs `qstring:"actor,omitempty"`
	TargetKey  CompStrs `qstring:"target,omitempty"`
	Type       CompStrs `qstring:"type,omitempty"`
	AttrTo     CompStrs `qstring:"attributedTo,omitempty"`
	InReplTo   CompStrs `qstring:"inReplyTo,omitempty"`
	OP         CompStrs `qstring:"context,omitempty"`
	FollowedBy CompStrs `qstring:"followedBy,omitempty"` // todo(marius): not really used
	Recipients CompStrs `qstring:"recipients,omitempty"`
	Next       string   `qstring:"after,omitempty"`
	Prev       string   `qstring:"before,omitempty"`
	MaxItems   int      `qstring:"maxItems,omitempty"`
}

// FiltersFromRequest loads the filters we use for generating storage queries from the HTTP request
func FiltersFromRequest(r *http.Request) *fedFilters {
	f := fedFilters{}
	if err := qstring.Unmarshal(r.URL.Query(), &f); err != nil {
		return nil
	}
	if f.MaxItems == 0 {
		f.MaxItems = MaxContentItems
	}
	return &f
}

// FiltersFromRequest loads the filters we use for generating storage queries from the HTTP request
func FiltersFromContext(ctx context.Context) *fedFilters {
	if f, ok := ctx.Value(FilterCtxtKey).(*fedFilters); ok {
		return f
	}
	return nil
}

var CreateActivitiesFilter = CompStrs{
	CompStr{Str: string(pub.CreateType)},
}

func DefaultFilters(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = CreateActivitiesFilter
		ctx := context.WithValue(r.Context(), FilterCtxtKey, f)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func SelfFilters(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = CreateActivitiesFilter
		f.IRI = CompStrs{LikeString(Instance.APIURL)}
		ctx := context.WithValue(r.Context(), FilterCtxtKey, f)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

var CreateFollowActivitiesFilter = CompStrs{
	CompStr{Str: string(pub.CreateType)},
	CompStr{Str: string(pub.FollowType)},
}

func FollowedFilters(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = CreateFollowActivitiesFilter
		ctx := context.WithValue(r.Context(), FilterCtxtKey, f)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func DifferentThanString(s string) CompStr {
	return CompStr{Operator: "!", Str: s}
}

func FederatedFilters(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = CreateActivitiesFilter
		f.IRI = CompStrs{DifferentThanString(Instance.APIURL)}
		ctx := context.WithValue(r.Context(), FilterCtxtKey, f)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LikeString(s string) CompStr {
	return CompStr{Operator: "~", Str: s}
}

func DomainFilters(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.URL = CompStrs{LikeString(chi.URLParam(r, "domain"))}
		ctx := context.WithValue(r.Context(), FilterCtxtKey, f)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func TagFilters(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Cont = CompStrs{LikeString(chi.URLParam(r, "tag"))}
		ctx := context.WithValue(r.Context(), FilterCtxtKey, f)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func AccountFilters(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.AttrTo = CompStrs{LikeString(chi.URLParam(r, "handle"))}
		ctx := context.WithValue(r.Context(), FilterCtxtKey, f)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
