package activitypub

import (
	"fmt"
	"github.com/juju/errors"
	as "github.com/mariusor/activitypub.go/activitystreams"
	j "github.com/mariusor/activitypub.go/jsonld"
	"github.com/mariusor/littr.go/app/log"
	"github.com/spacemonkeygo/httpsig"
	"io"
	"io/ioutil"
	"net/http"
)

type Config struct {
	Logger    log.Logger
	UserAgent string
}

type Client struct {
	logger log.Logger
	signer *httpsig.Signer
	ua     string
}

func NewClient(c Config) Client {
	return Client{
		logger: c.Logger,
		ua:     c.UserAgent,
	}
}

func (c *Client) WithSigner(s *httpsig.Signer) error {
	c.signer = s
	return nil
}

func (c *Client) LoadActor(id as.IRI) (Person, error) {
	var err error

	a := Person{}

	var resp *http.Response
	if resp, err = c.Get(id.String()); err != nil {
		c.logger.Error(err.Error())
		return a, err
	}
	if resp == nil {
		err := fmt.Errorf("nil response from the repository")
		c.logger.Error(err.Error())
		return a, err
	}
	if resp.StatusCode != http.StatusOK {
		err := errors.New("unable to load from the AP end point")
		c.logger.WithContext(log.Ctx{
			"iri": id,
			"signed": c.signer != nil,
		} ).Error(err.Error())
		return a, err
	}
	defer resp.Body.Close()
	var body []byte
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		c.logger.Error(err.Error())
		return a, err
	}
	if err = j.Unmarshal(body, &a); err != nil {
		c.logger.Error(err.Error())
		return a, err
	}

	return a, nil
}

func (c *Client) sign(req *http.Request) error {
	if c.signer == nil {
		return nil
	}
	return c.signer.Sign(req)
}

func (c *Client) req(method string, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.ua)
	err = c.sign(req)
	if err != nil {
		new := errors.Errorf("unable to sign request")
		c.logger.WithContext(log.Ctx{
			"url":      req.URL,
			"method":   req.Method,
			"previous": err.Error(),
		}).Warn(new.Error())

		return req, new
	}
	return req, nil
}

func (c Client) Head(url string) (resp *http.Response, err error) {
	req, err := c.req(http.MethodHead, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func (c Client) Get(url string) (resp *http.Response, err error) {
	req, err := c.req(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func (c *Client) Post(url, contentType string, body io.Reader) (resp *http.Response, err error) {
	req, err := c.req(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return http.DefaultClient.Do(req)
}

func (c Client) Put(url, contentType string, body io.Reader) (resp *http.Response, err error) {
	req, err := c.req(http.MethodPut, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return http.DefaultClient.Do(req)
}

func (c Client) Delete(url, contentType string, body io.Reader) (resp *http.Response, err error) {
	req, err := c.req(http.MethodDelete, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return http.DefaultClient.Do(req)
}
