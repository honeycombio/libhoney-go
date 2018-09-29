package api

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
)

type TeamScopedClient struct {
	APIHost    string
	WriteKey   string
	UserAgent  string
	HTTPClient *http.Client
}

func (c *TeamScopedClient) doRequest(method, endpoint, resourceID string, body io.Reader) (*http.Response, error) {
	url, err := url.Parse(c.APIHost)
	if err != nil {
		return nil, err
	}
	url.Path = path.Join(url.Path, endpoint, resourceID)
	req, err := http.NewRequest(method, url.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Add("X-Honeycomb-Team", c.WriteKey)

	return c.HTTPClient.Do(req)
}

func (c *TeamScopedClient) Create(endpoint string, resource interface{}) error {
	marshaledResource, err := json.Marshal(resource)
	if err != nil {
		return err
	}

	_, err = c.doRequest("POST", endpoint, "", bytes.NewReader(marshaledResource))
	return err
}

func (c *TeamScopedClient) List(endpoint string, destSlice interface{}) error {
	resp, err := c.doRequest("GET", endpoint, "", nil)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(bodyBytes, destSlice)
	if err != nil {
		return err
	}
	return nil
}

func (c *TeamScopedClient) Get(endpoint, id string, destResource interface{}) error {
	resp, err := c.doRequest("GET", endpoint, id, nil)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(bodyBytes, destResource)
	if err != nil {
		return err
	}

	return nil
}

func (c *TeamScopedClient) Update(endpoint, id string, resource interface{}) error {
	marshaledResource, err := json.Marshal(resource)
	if err != nil {
		return err
	}

	_, err = c.doRequest("PUT", endpoint, id, bytes.NewReader(marshaledResource))
	return err
}

func (c *TeamScopedClient) Delete(endpoint, id string) error {
	_, err := c.doRequest("DELETE", endpoint, id, nil)
	return err
}
