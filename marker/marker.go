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

// CreateMarker creates a marker based on the API call documented here: https://docs.honeycomb.io/api/markers/create-a-marker
// It returns the Marker ID if successful, and an error if not successful
func (client *markerClient) CreateMarker(dataset, message, markerType string) (string, error) {
	path, err := client.buildRequestURL(dataset)
	if err != nil {
		return "", err
	}
	reqBody := createMarkerRequest{
		Message: message,
		Type:    markerType,
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST", path, bytes.NewBuffer(b))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Honeycomb-Team", client.apiKey)

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated { // any other 2XX status code is unexpected
		var respBody createMarkerSuccessResponse
		if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
			return "", nil // creation was a success, but no id to return, so don't return an error. Alternatively, we could return a custom descriptive error
		}
		return respBody.ID, nil
	}

	var respBody createMarkerErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return "", err
	}

	if respBody.Error != "" {
		return "", errors.New(fmt.Sprintf("Error response from server: %s", respBody.Error))
	}

	return "", errors.New(fmt.Sprintf("Undescribed response from server with status code %d", resp.StatusCode))
}

// DeleteMarker deletes a marker by marker ID (for example, the markerID returned with client.CreateMarker). See https://docs.honeycomb.io/api/markers/delete-a-marker
func (client *markerClient) DeleteMarker(dataset, markerID string) error {
	path, err := client.buildRequestURL(dataset)
	if err != nil {
		return err
	}
	path, err = url.JoinPath(path, url.PathEscape(markerID))
	req, err := http.NewRequest("DELETE", path, nil)
	if err != nil {
		return err
	}

	req.Header.Set("X-Honeycomb-Team", client.apiKey)

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK { // any other 2XX status code is unexpected
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
