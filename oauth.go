package brutalinks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"git.sr.ht/~mariusor/box"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/client/credentials"
	"github.com/go-ap/errors"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

const WellKnownOAuthAuthorizationServer = "/.well-known/oauth-authorization-server"

func buildOAuthAuthorizationWellKnownURL(actor vocab.IRI) (string, error) {
	// NOTE(marius): try building the RFC8414 well-known URL
	// based on the actor IRI: https://datatracker.ietf.org/doc/html//URL#section-2
	u, err := actor.URL()
	if err != nil {
		return "", errors.Annotatef(err, "unable to build OAuth authorization URL")
	}
	u.Path = filepath.Join(WellKnownOAuthAuthorizationServer, u.Path)
	return u.String(), nil
}

func createDynamicOAuth2Client(cl *http.Client, actor vocab.IRI, conf appConfig) (*box.ClientRegistrationResponse, error) {
	// NOTE(marius): this is the RFC8414 .well-known/oauth-authorization-server method for
	// creating the OAuth2 client for the actor.
	oauthURL, err := buildOAuthAuthorizationWellKnownURL(actor)
	if err != nil {
		return nil, errors.Annotatef(err, "unable to build OAuth authorization URL")
	}
	req, err := http.NewRequest(http.MethodGet, oauthURL, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "unable to build OAuth authorization request")
	}
	metaResp, err := cl.Do(req)
	if err != nil {
		return nil, errors.Annotatef(err, "unable to fetch OAuth authorization response")
	}
	defer metaResp.Body.Close()

	// TODO(marius): improve handling of the error cases
	if metaResp.StatusCode != http.StatusOK {
		return nil, errors.Newf("invalid response returned: %d: %s", metaResp.StatusCode, metaResp.Status)
	}

	metadata := box.OAuthAuthorizationMetadata{}
	if err = json.NewDecoder(metaResp.Body).Decode(&metadata); err != nil {
		return nil, errors.Annotatef(err, "unable to decode OAuth authorization metadata")
	}

	if !metadata.Issuer.Equals(actor, true) {
		return nil, errors.Newf("issuer metadata is different than the actor originating this operation")
	}

	if metadata.RegistrationEndpoint == "" {
		return nil, errors.Newf("no registration endpoint in the OAuth authorization metadata")
	}

	uid, _ := uuid.FromBytes(uuidLike(conf.HostName))

	contacts := []string{conf.AdminContact, softwareContact}
	if !slices.Contains(contacts, author) {
		contacts = append(contacts, author)
	}
	registerData := box.ClientRegistrationRequest{
		RedirectUris:            []string{conf.buildOAuth2RedirectURL()},
		ClientName:              conf.HostName,
		TokenEndpointAuthMethod: "client_secret_basic",
		ClientURI:               conf.BuildOAuth2ClientURL(),
		Contacts:                contacts,
		SoftwareID:              uid.String(),
	}

	data := bytes.Buffer{}
	if err = json.NewEncoder(&data).Encode(registerData); err != nil {
		return nil, errors.Annotatef(err, "unable to encode OAuth client registration data")
	}

	registerReq, err := http.NewRequest(http.MethodPost, metadata.RegistrationEndpoint, &data)
	if err != nil {
		return nil, errors.Annotatef(err, "unable to build OAuth registration request")
	}
	regResp, err := cl.Do(registerReq)
	if err != nil {
		return nil, errors.Annotatef(err, "unable to register OAuth client")
	}
	defer regResp.Body.Close()

	if regResp.StatusCode > http.StatusCreated {
		return nil, errors.Newf("invalid response returned: %d: %s", regResp.StatusCode, regResp.Status)
	}

	regRespData := box.ClientRegistrationResponse{}
	if err = json.NewDecoder(regResp.Body).Decode(&regRespData); err != nil {
		return nil, errors.Annotatef(err, "unable to decode OAuth client registration response")
	}

	if regRespData.ClientID == "" || regRespData.ClientSecret == "" {
		return nil, errors.Newf("invalid client metadata returned")
	}
	return &regRespData, nil
}

func TryOAuth2ClientRegistration(fedboxIRI vocab.IRI, conf appConfig) (*credentials.ClientConfig, error) {
	cl := http.DefaultClient

	regRespData, err := createDynamicOAuth2Client(cl, fedboxIRI, conf)
	if err != nil {
		return nil, err
	}

	auth := credentials.ClientConfig{
		RedirectURL:  conf.buildOAuth2RedirectURL(),
		ClientID:     regRespData.ClientID,
		ClientSecret: regRespData.ClientSecret,
		IssuedAt:     time.Unix(regRespData.IssuedAt, 0),
		Expiration:   time.Duration(regRespData.Expires),
	}

	return &auth, nil
}

func (r repository) GetOauth2Config(provider string) oauth2.Config {
	var conf oauth2.Config
	switch strings.ToLower(provider) {
	case "github":
		conf.ClientID = os.Getenv("GITHUB_KEY")
		conf.ClientSecret = os.Getenv("GITHUB_SECRET")
		conf.Endpoint = oauth2.Endpoint{
			AuthURL:  "https://github.com/login/oauth/authorize",
			TokenURL: "https://github.com/login/oauth/access_token",
		}
		conf.RedirectURL = fmt.Sprintf("%s/auth/%s/callback", r.SelfURL, provider)
	case "gitlab":
		conf.ClientID = os.Getenv("GITLAB_KEY")
		conf.ClientSecret = os.Getenv("GITLAB_SECRET")
		conf.Endpoint = oauth2.Endpoint{
			AuthURL:  "https://gitlab.com/login/oauth/authorize",
			TokenURL: "https://gitlab.com/login/oauth/access_token",
		}
		conf.RedirectURL = fmt.Sprintf("%s/auth/%s/callback", r.SelfURL, provider)
	case "facebook":
		conf.ClientID = os.Getenv("FACEBOOK_KEY")
		conf.ClientSecret = os.Getenv("FACEBOOK_SECRET")
		conf.Endpoint = oauth2.Endpoint{
			AuthURL:  "https://graph.facebook.com/oauth/authorize",
			TokenURL: "https://graph.facebook.com/oauth/access_token",
		}
		conf.RedirectURL = fmt.Sprintf("%s/auth/%s/callback", r.SelfURL, provider)
	case "google":
		conf.ClientID = os.Getenv("GOOGLE_KEY")
		conf.ClientSecret = os.Getenv("GOOGLE_SECRET")
		conf.Endpoint = oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth", // access_type=offline
			TokenURL: "https://accounts.google.com/o/oauth2/token",
		}
		conf.RedirectURL = fmt.Sprintf("%s/auth/%s/callback", r.SelfURL, provider)
	case "fedbox":
		fallthrough
	default:
		conf = r.cred.Conf
	}
	return conf
}
