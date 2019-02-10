package tests

import (
	"fmt"
	"golang.org/x/xerrors"
	"io"
	"net/http"
	"os"
	"testing"
)

// UserAgent value that the client uses when performing requests
var UserAgent = "test-go-http-client"
var HeaderAccept = `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`

func TestEmpty(t *testing.T) {
	t.Logf("OK")
}

func execReq(url string, met string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(met, url, body)
	req.Header.Set("User-Agent", fmt.Sprintf("-%s", UserAgent))
	req.Header.Set("Accept", HeaderAccept)
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, xerrors.Errorf("Error: invalid HTTP response %d, expected %d", resp.StatusCode, http.StatusOK)
	}
	b := make([]byte, 0)
	if c, err := resp.Body.Read(b); err != nil {
		return nil, xerrors.Errorf("Error: invalid HTTP body! Read %d bytes %s", c, b)
	}
	return b, nil
}

func Test_GETNodeInfo(t *testing.T) {
	apiURL := os.Getenv("API_URL")

	url := fmt.Sprintf("%s/self", apiURL)
	if b, err := execReq(url, http.MethodGet, nil); err != nil {
		t.Errorf("Error %s", err)
	} else {
		t.Logf("OK %s", b)
	}
}
