package oauth

import (
	"database/sql"
	"fmt"
	"github.com/mariusor/littr.go/internal/errors"
	"github.com/mariusor/littr.go/internal/log"
	"github.com/openshift/osin"
	"net/http"
	"time"
)

type logger struct {
	l log.Logger
}

func (l logger) Printf(format string, v ...interface{}) {
	l.l.Debugf(format, v...)
}

func NewOAuth(Host, User, Pw, Name string, l log.Logger) (*osin.Server, error) {
	url := fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=disable", Host, User, Pw, Name)
	db, err := sql.Open("postgres", url)
	if err != nil {
		return nil, err
	}

	config := osin.ServerConfig{
		AuthorizationExpiration:   86400,
		AccessExpiration:          2678400,
		TokenType:                 "Bearer",
		AllowedAuthorizeTypes:     osin.AllowedAuthorizeType{osin.CODE},
		AllowedAccessTypes:        osin.AllowedAccessType{osin.AUTHORIZATION_CODE},
		ErrorStatusCode:           http.StatusForbidden,
		AllowClientSecretInParams: false,
		AllowGetAccessRequest:     false,
		RetainTokenAfterRefresh:   true,
		RedirectUriSeparator:      "\n",
		//RequirePKCEForPublicClients: true,
	}
	store := New(db, l)
	s := osin.NewServer(&config, store)
	s.Logger = logger{l: l}
	return s, nil
}

// Storage implements interface "github.com/RangelReale/osin".Storage and interface "github.com/ory/osin-storage".Storage
type Storage struct {
	db *sql.DB
	l  log.Logger
}

// New returns a new postgres storage instance.
func New(db *sql.DB, l log.Logger) *Storage {
	return &Storage{db, l}
}

// Clone the storage if needed. For example, using mgo, you can clone the session with session.Clone
// to avoid concurrent access problems.
// This is to avoid cloning the connection at each method access.
// Can return itself if not a problem.
func (s *Storage) Clone() osin.Storage {
	return s
}

// Close the resources the Storage potentially holds (using Clone for example)
func (s *Storage) Close() {
}

// GetClient loads the client by id
func (s *Storage) GetClient(id string) (osin.Client, error) {
	row := s.db.QueryRow("SELECT id, secret, redirect_uri, extra FROM client WHERE id=$1", id)
	var c osin.DefaultClient
	var extra []byte

	if err := row.Scan(&c.Id, &c.Secret, &c.RedirectUri, &extra); err == sql.ErrNoRows {
		return nil, errors.NotFoundf("")
	} else if err != nil {
		return nil, errors.Annotate(err, "")
	}
	c.UserData = extra
	return &c, nil
}

// UpdateClient updates the client (identified by it's id) and replaces the values with the values of client.
func (s *Storage) UpdateClient(c osin.Client) error {
	data, err := assertToString(c.GetUserData())
	if err != nil {
		s.l.WithContext(log.Ctx{"id": c.GetId()}).Error(err.Error())
		return err
	}

	if _, err := s.db.Exec("UPDATE client SET (secret, redirect_uri, extra) = ($2, $3, $4) WHERE id=$1", c.GetId(), c.GetSecret(), c.GetRedirectUri(), data); err != nil {
		s.l.WithContext(log.Ctx{"id": c.GetId()}).Error(err.Error())
		return errors.Annotate(err, "")
	}
	return nil
}

// CreateClient stores the client in the database and returns an error, if something went wrong.
func (s *Storage) CreateClient(c osin.Client) error {
	data, err := assertToString(c.GetUserData())
	if err != nil {
		s.l.WithContext(log.Ctx{"id": c.GetId()}).Error(err.Error())
		return err
	}

	if _, err := s.db.Exec("INSERT INTO client (id, secret, redirect_uri, extra) VALUES ($1, $2, $3, $4)", c.GetId(), c.GetSecret(), c.GetRedirectUri(), data); err != nil {
		s.l.WithContext(log.Ctx{"id": c.GetId(), "redirect_uri": c.GetRedirectUri()}).Debugf("Added new client")
		return errors.Annotate(err, "")
	}
	return nil
}

// RemoveClient removes a client (identified by id) from the database. Returns an error if something went wrong.
func (s *Storage) RemoveClient(id string) (err error) {
	if _, err = s.db.Exec("DELETE FROM client WHERE id=$1", id); err != nil {
		s.l.WithContext(log.Ctx{"id": id}).Error(err.Error())
		return errors.Annotate(err, "")
	}
	s.l.WithContext(log.Ctx{"id": id}).Debugf("removed client")
	return nil
}

// SaveAuthorize saves authorize data.
func (s *Storage) SaveAuthorize(data *osin.AuthorizeData) (err error) {
	extra, err := assertToString(data.UserData)
	if err != nil {
		s.l.WithContext(log.Ctx{"id": data.Client.GetId(), "code": data.Code}).Error(err.Error())
		return err
	}

	if _, err = s.db.Exec(
		"INSERT INTO authorize (client, code, expires_in, scope, redirect_uri, state, created_at, extra) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
		data.Client.GetId(),
		data.Code,
		data.ExpiresIn,
		data.Scope,
		data.RedirectUri,
		data.State,
		data.CreatedAt,
		extra,
	); err != nil {
		s.l.WithContext(log.Ctx{"id": data.Client.GetId(), "code": data.Code}).Error(err.Error())
		return errors.Annotate(err, "")
	}
	return nil
}

// LoadAuthorize looks up AuthorizeData by a code.
// Client information MUST be loaded together.
// Optionally can return error if expired.
func (s *Storage) LoadAuthorize(code string) (*osin.AuthorizeData, error) {
	var data osin.AuthorizeData
	var extra string
	var cid string
	if err := s.db.QueryRow("SELECT client, code, expires_in, scope, redirect_uri, state, created_at, extra FROM authorize WHERE code=$1 LIMIT 1", code).Scan(&cid, &data.Code, &data.ExpiresIn, &data.Scope, &data.RedirectUri, &data.State, &data.CreatedAt, &extra); err == sql.ErrNoRows {
		s.l.WithContext(log.Ctx{"code": code}).Error(err.Error())
		return nil, errors.NotFoundf("")
	} else if err != nil {
		s.l.WithContext(log.Ctx{"code": code}).Error(err.Error())
		return nil, errors.Annotate(err, "")
	}
	data.UserData = extra

	c, err := s.GetClient(cid)
	if err != nil {
		return nil, err
	}

	if data.ExpireAt().Before(time.Now()) {
		s.l.WithContext(log.Ctx{"code": code}).Error(err.Error())
		return nil, errors.Errorf("Token expired at %s.", data.ExpireAt().String())
	}

	data.Client = c
	return &data, nil
}

// RemoveAuthorize revokes or deletes the authorization code.
func (s *Storage) RemoveAuthorize(code string) (err error) {
	if _, err = s.db.Exec("DELETE FROM authorize WHERE code=$1", code); err != nil {
		s.l.WithContext(log.Ctx{"code": code}).Error(err.Error())
		return errors.Annotate(err, "")
	}
	s.l.WithContext(log.Ctx{"code": code}).Debugf("removed authorization token")
	return nil
}

// SaveAccess writes AccessData.
// If RefreshToken is not blank, it must save in a way that can be loaded using LoadRefresh.
func (s *Storage) SaveAccess(data *osin.AccessData) (err error) {
	prev := ""
	authorizeData := &osin.AuthorizeData{}

	if data.AccessData != nil {
		prev = data.AccessData.AccessToken
	}

	if data.AuthorizeData != nil {
		authorizeData = data.AuthorizeData
	}

	extra, err := assertToString(data.UserData)
	if err != nil {
		s.l.WithContext(log.Ctx{"id": data.Client.GetId()}).Error(err.Error())
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		s.l.WithContext(log.Ctx{"id": data.Client.GetId()}).Error(err.Error())
		return errors.Annotate(err, "")
	}

	if data.RefreshToken != "" {
		if err := s.saveRefresh(tx, data.RefreshToken, data.AccessToken); err != nil {
			s.l.WithContext(log.Ctx{"id": data.Client.GetId()}).Error(err.Error())
			return err
		}
	}

	if data.Client == nil {
		return errors.New("data.Client must not be nil")
	}

	_, err = tx.Exec("INSERT INTO access (client, authorize, previous, access_token, refresh_token, expires_in, scope, redirect_uri, created_at, extra) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)", data.Client.GetId(), authorizeData.Code, prev, data.AccessToken, data.RefreshToken, data.ExpiresIn, data.Scope, data.RedirectUri, data.CreatedAt, extra)
	if err != nil {
		if rbe := tx.Rollback(); rbe != nil {
			s.l.WithContext(log.Ctx{"id": data.Client.GetId()}).Error(rbe.Error())
			return errors.Annotate(rbe, "")
		}
		s.l.WithContext(log.Ctx{"id": data.Client.GetId()}).Error(err.Error())
		return errors.Annotate(err, "")
	}

	if err = tx.Commit(); err != nil {
		s.l.WithContext(log.Ctx{"id": data.Client.GetId()}).Error(err.Error())
		return errors.Annotate(err, "")
	}

	return nil
}

// LoadAccess retrieves access data by token. Client information MUST be loaded together.
// AuthorizeData and AccessData DON'T NEED to be loaded if not easily available.
// Optionally can return error if expired.
func (s *Storage) LoadAccess(code string) (*osin.AccessData, error) {
	var extra, cid, prevAccessToken, authorizeCode string
	var result osin.AccessData

	if err := s.db.QueryRow(
		"SELECT client, authorize, previous, access_token, refresh_token, expires_in, scope, redirect_uri, created_at, extra FROM access WHERE access_token=$1 LIMIT 1",
		code,
	).Scan(
		&cid,
		&authorizeCode,
		&prevAccessToken,
		&result.AccessToken,
		&result.RefreshToken,
		&result.ExpiresIn,
		&result.Scope,
		&result.RedirectUri,
		&result.CreatedAt,
		&extra,
	); err == sql.ErrNoRows {
		s.l.WithContext(log.Ctx{"code": code}).Error(err.Error())
		return nil, errors.NotFoundf("")
	} else if err != nil {
		return nil, errors.Annotate(err, "")
	}

	result.UserData = extra
	client, err := s.GetClient(cid)
	if err != nil {
		s.l.WithContext(log.Ctx{"code": code}).Error(err.Error())
		return nil, err
	}

	result.Client = client
	result.AuthorizeData, _ = s.LoadAuthorize(authorizeCode)
	prevAccess, _ := s.LoadAccess(prevAccessToken)
	result.AccessData = prevAccess
	return &result, nil
}

// RemoveAccess revokes or deletes an AccessData.
func (s *Storage) RemoveAccess(code string) (err error) {
	_, err = s.db.Exec("DELETE FROM access WHERE access_token=$1", code)
	if err != nil {
		s.l.WithContext(log.Ctx{"code": code}).Error(err.Error())
		return errors.Annotate(err, "")
	}
	s.l.WithContext(log.Ctx{"code": code}).Debugf("removed access token")
	return nil
}

// LoadRefresh retrieves refresh AccessData. Client information MUST be loaded together.
// AuthorizeData and AccessData DON'T NEED to be loaded if not easily available.
// Optionally can return error if expired.
func (s *Storage) LoadRefresh(code string) (*osin.AccessData, error) {
	row := s.db.QueryRow("SELECT access FROM refresh WHERE token=$1 LIMIT 1", code)
	var access string
	if err := row.Scan(&access); err == sql.ErrNoRows {
		s.l.WithContext(log.Ctx{"code": code}).Error(err.Error())
		return nil, errors.NotFoundf("")
	} else if err != nil {
		s.l.WithContext(log.Ctx{"code": code}).Error(err.Error())
		return nil, errors.Annotate(err, "")
	}
	return s.LoadAccess(access)
}

// RemoveRefresh revokes or deletes refresh AccessData.
func (s *Storage) RemoveRefresh(code string) error {
	_, err := s.db.Exec("DELETE FROM refresh WHERE token=$1", code)
	if err != nil {
		s.l.WithContext(log.Ctx{"code": code}).Error(err.Error())
		return errors.Annotate(err, "")
	}
	s.l.WithContext(log.Ctx{"code": code}).Debugf("removed refresh token")
	return nil
}

func (s *Storage) saveRefresh(tx *sql.Tx, refresh, access string) (err error) {
	_, err = tx.Exec("INSERT INTO refresh (token, access) VALUES ($1, $2)", refresh, access)
	if err != nil {
		if rbe := tx.Rollback(); rbe != nil {
			s.l.WithContext(log.Ctx{"code": access}).Error(rbe.Error())
			return errors.Annotate(rbe, "")
		}
		return errors.Annotate(err, "")
	}
	return nil
}

func assertToString(in interface{}) (string, error) {
	var ok bool
	var data string
	if in == nil {
		return "", nil
	} else if data, ok = in.(string); ok {
		return data, nil
	} else if byt, ok := in.([]byte); ok {
		return string(byt), nil
	} else if str, ok := in.(fmt.Stringer); ok {
		return str.String(), nil
	}
	return "", errors.Errorf(`Could not assert "%v" to string`, in)
}
