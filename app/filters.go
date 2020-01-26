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
	Key                  []Hash     `qstring:"iri,omitempty"`
	MediaType            []MimeType `qstring:"mediaType,omitempty"`
	AttributedTo         []Hash     `qstring:"attributedTo,omitempty"`
	InReplyTo            []string   `qstring:"inReplyTo,omitempty"`
	Context              []string   `qstring:"context,omitempty"`
	SubmittedAt          time.Time  `qstring:"submittedAt,omitempty"`
	SubmittedAtMatchType MatchType  `qstring:"submittedAtMatchType,omitempty"`
	Content              string     `qstring:"content,omitempty"`
	ContentMatchType     MatchType  `qstring:"contentMatchType,omitempty"`
	Deleted              []bool     `qstring:"-"` // used as an array to allow for it to be missing
	IRI                  string     `qstring:"id,omitempty"`
	URL                  string     `qstring:"url,omitempty"`
	Depth                int        `qstring:"depth,omitempty"`
	Federated            []bool     `qstring:"-"` // used as an array to allow for it to be missing
	Private              []bool     `qstring:"-"` // used as an array to allow for it to be missing
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

func copyVotesFilters(a *LoadVotesFilter, b LoadVotesFilter) {
	a.ItemKey = b.ItemKey
	a.Type = b.Type
	a.AttributedTo = b.AttributedTo
	a.SubmittedAt = b.SubmittedAt
	a.SubmittedAtMatchType = b.SubmittedAtMatchType
}

func copyItemsFilters(a *LoadItemsFilter, b LoadItemsFilter) {
	a.Key = b.Key
	a.MediaType = b.MediaType
	a.AttributedTo = b.AttributedTo
	a.InReplyTo = b.InReplyTo
	a.Context = b.Context
	a.SubmittedAt = b.SubmittedAt
	a.SubmittedAtMatchType = b.SubmittedAtMatchType
	a.Content = b.Content
	a.ContentMatchType = b.ContentMatchType
	a.Deleted = b.Deleted
	a.IRI = b.IRI
	a.Deleted = b.Deleted
	a.FollowedBy = b.FollowedBy
	a.contentAlias = b.contentAlias
	a.authorAlias = b.authorAlias
}

func copyFilters(a *Filters, b Filters) {
	copyAccountFilters(&a.LoadAccountsFilter, b.LoadAccountsFilter)
	copyItemsFilters(&a.LoadItemsFilter, b.LoadItemsFilter)
	copyVotesFilters(&a.LoadVotesFilter, b.LoadVotesFilter)
	a.MaxItems = b.MaxItems
	a.Page = b.Page
}

func copyAccountFilters(a *LoadAccountsFilter, b LoadAccountsFilter) {
	a.Key = b.Key
	a.Handle = b.Handle
	a.Email = b.Email
	a.Deleted = b.Deleted
	a.InboxIRI = b.InboxIRI
	a.IRI = b.IRI
}

type fedFilters struct {
	Name       []string                    `qstring:"name,omitempty"`
	Cont       []string                    `qstring:"content,omitempty"`
	URL        pub.IRIs                    `qstring:"url,omitempty"`
	MedTypes   []pub.MimeType              `qstring:"mediaType,omitempty"`
	IRI        pub.IRIs                    `qstring:"iri,omitempty"`
	ObjectIRI  pub.IRIs                    `qstring:"object,omitempty"`
	Generator  pub.IRIs                    `qstring:"generator,omitempty"`
	ActorKey   []string                    `qstring:"actor,omitempty"`
	TargetKey  []string                    `qstring:"target,omitempty"`
	Type       pub.ActivityVocabularyTypes `qstring:"type,omitempty"`
	AttrTo     []string                    `qstring:"attributedTo,omitempty"`
	InReplTo   []string                    `qstring:"inReplyTo,omitempty"`
	OP         []string                    `qstring:"context,omitempty"`
	FollowedBy []string                    `qstring:"followedBy,omitempty"` // todo(marius): not really used
	Recipients pub.IRIs                    `qstring:"recipients,omitempty"`
	Next       string                      `qstring:"after,omitempty"`
	Prev       string                      `qstring:"before,omitempty"`
	MaxItems   int                         `qstring:"maxItems,omitempty"`
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

func DefaultFilters(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = pub.ActivityVocabularyTypes{
			pub.CreateType,
		}
		ctx := context.WithValue(r.Context(), FilterCtxtKey, f)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func SelfFilters(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = pub.ActivityVocabularyTypes{
			pub.CreateType,
		}
		f.Generator = pub.IRIs{pub.IRI(Instance.BaseURL)}
		ctx := context.WithValue(r.Context(), FilterCtxtKey, f)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func FollowedFilters(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = pub.ActivityVocabularyTypes{
			pub.CreateType,
			pub.FollowType,
		}
		ctx := context.WithValue(r.Context(), FilterCtxtKey, f)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func FederatedFilters(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Type = pub.ActivityVocabularyTypes{
			pub.CreateType,
		}
		f.Generator = pub.IRIs{"-"}
		ctx := context.WithValue(r.Context(), FilterCtxtKey, f)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func DomainFilters(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.URL = pub.IRIs{pub.IRI(chi.URLParam(r, "domain"))}
		ctx := context.WithValue(r.Context(), FilterCtxtKey, f)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func TagFilters(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.Cont = []string{chi.URLParam(r, "tag")}
		ctx := context.WithValue(r.Context(), FilterCtxtKey, f)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
func AccountFilters(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := FiltersFromRequest(r)
		f.AttrTo = []string{chi.URLParam(r, "handle")}
		ctx := context.WithValue(r.Context(), FilterCtxtKey, f)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
