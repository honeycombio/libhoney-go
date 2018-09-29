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

type DatasetScopedClient struct {
	APIHost    string
	WriteKey   string
	UserAgent  string
	HTTPClient *http.Client
}

func (c *DatasetScopedClient) doRequest(method, endpoint, dataset, resourceID string, body io.Reader) (*http.Response, error) {
	url, err := url.Parse(c.APIHost)
	if err != nil {
		return nil, err
	}
	url.Path = path.Join(url.Path, endpoint, dataset, resourceID)
	req, err := http.NewRequest(method, url.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Add("X-Honeycomb-Team", c.WriteKey)

	return c.HTTPClient.Do(req)
}

func (c *DatasetScopedClient) Create(endpoint, dataset string, resource interface{}) error {
	marshaledResource, err := json.Marshal(resource)
	if err != nil {
		return err
	}

	_, err = c.doRequest("POST", endpoint, dataset, "", bytes.NewReader(marshaledResource))
	return err
}

func (c *DatasetScopedClient) List(endpoint, dataset string, destSlice interface{}) error {
	resp, err := c.doRequest("GET", endpoint, dataset, "", nil)
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

func (c *DatasetScopedClient) Get(endpoint, dataset, id string, destResource interface{}) error {
	resp, err := c.doRequest("GET", endpoint, dataset, id, nil)
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

func (c *DatasetScopedClient) Update(endpoint, dataset, id string, resource interface{}) error {
	marshaledResource, err := json.Marshal(resource)
	if err != nil {
		return err
	}

	_, err = c.doRequest("PUT", endpoint, dataset, id, bytes.NewReader(marshaledResource))
	return err
}

func (c *DatasetScopedClient) Delete(endpoint, dataset, id string) error {
	_, err := c.doRequest("DELETE", endpoint, dataset, id, nil)
	return err
}
