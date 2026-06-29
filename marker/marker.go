package marker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type markerClient struct {
	apiKey     string
	apiHost    baseURL
	httpClient *http.Client
}

type markerClientInterface interface {
	CreateMarker(datasetName, message, markerType string) error
}

func NewClient(apiKey string, apiHost baseURL) *markerClient {
	return &markerClient{
		apiKey:  apiKey,
		apiHost: apiHost,
		httpClient: &http.Client{
			Timeout: 3 * time.Second, //TODO: make this configurable
		},
	}
}

func (client *markerClient) buildRequestURL(dataset string) (string, error) {
	return url.JoinPath(string(client.apiHost), "1/markers", url.PathEscape(dataset))
}

func (client *markerClient) CreateMarker(dataset, message, markerType string) error {
	url, err := client.buildRequestURL(dataset)
	if err != nil {
		return err
	}
	reqBody := createMarkerRequest{
		Message: message,
		Type:    markerType,
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Honeycomb-Team", client.apiKey)

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated { // any other 2XX status code is unexpected
		return nil
	}

	var respBody createMarkerErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return err
	}

	if respBody.Error != "" {
		return errors.New(fmt.Sprintf("Error response from server: %s", respBody.Error))
	}

	return errors.New(fmt.Sprintf("Undescribed response from server with status code %d", resp.StatusCode))
}
