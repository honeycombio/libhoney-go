package markers

import (
	"github.com/honeycombio/libhoney-go/api"
)

const endpoint = "/1/markers"

type Client struct {
	api.DatasetScopedClient
}

func (mc *Client) Create(dataset string, marker *Marker) error {
	return mc.DatasetScopedClient.Create(endpoint, dataset, marker)
}

func (mc *Client) List(dataset string) ([]*Marker, error) {
	markers := []*Marker{}
	err := mc.DatasetScopedClient.List(endpoint, dataset, &markers)
	if err != nil {
		return nil, err
	}
	return markers, nil
}

func (mc *Client) Get(dataset, id string) (*Marker, error) {
	marker := Marker{}
	err := mc.DatasetScopedClient.Get(endpoint, dataset, id, &marker)
	if err != nil {
		return nil, err
	}
	return &marker, nil
}

func (mc *Client) Update(dataset string, marker *Marker) error {
	return mc.DatasetScopedClient.Update(endpoint, dataset, marker.ID, marker)
}

func (mc *Client) Delete(dataset, id string) error {
	return mc.DatasetScopedClient.Delete(endpoint, dataset, id)
}
