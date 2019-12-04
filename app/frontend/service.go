package frontend

import (
	"github.com/go-ap/errors"
	"github.com/mariusor/littr.go/app"
	"strings"
)

// see doc/c2s.md
type Service struct {
	c *fedbox
}

type Model interface {
	// LoggedAccount returns the current logged account or AnonymousAccount
	LoggedAccount() app.Account
	// Items returns the comment listing
	Items() comments
	// Filters returns the current filtersf
	Filters() app.Filters
}

type PageModel struct {
	loggedAccount app.Account
	top           comment
	items         comments
	filters       app.Filters
}

type ErrorModel struct {
	er []error
}

func (e ErrorModel) Error() string {
	return strings.Join(func(e []error)[]string {
		s := make ([]string, len(e))
		for i, ee := range e {
			s[i] = ee.Error()
		}
		return s
	}(e.er), "<br/>")
}

func (s Service) LoadMainPage(f app.Filters) (Model, error) {
	return nil, errors.NotImplementedf("LoadMainPage not implemented")
}

func (s Service) LoadFollowedPage(f app.Filters) (Model, error) {
	return nil, errors.NotImplementedf("LoadFollowedPage not implemented")
}

func (s Service) LoadFederatedPage(f app.Filters) (Model, error) {
	return nil, errors.NotImplementedf("LoadFederatedPage not implemented")
}

func (s Service) LoadDiscussionsPage(f app.Filters) (Model, error) {
	return nil, errors.NotImplementedf("LoadDiscussionsPage not implemented")
}

func (s Service) LoadDomainPage(f app.Filters) (Model, error) {
	return nil, errors.NotImplementedf("LoadDomainPage not implemented")
}

func (s Service) LoadAccountPage(f app.Filters) (Model, error) {
	return nil, errors.NotImplementedf("LoadAccountPage not implemented")
}
