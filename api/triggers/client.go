package triggers

import "github.com/honeycombio/libhoney-go/api"

const endpoint = "/1/triggers"

type Client struct {
	api.DatasetScopedClient
}

func (tc *Client) Create(dataset string, trigger *Trigger) error {
	return tc.DatasetScopedClient.Create(endpoint, dataset, trigger)
}

func (tc *Client) List(dataset string) ([]*Trigger, error) {
	triggers := []*Trigger{}
	err := tc.DatasetScopedClient.List(endpoint, dataset, &triggers)
	if err != nil {
		return nil, err
	}
	return triggers, nil
}

func (tc *Client) Get(dataset, id string) (*Trigger, error) {
	trigger := Trigger{}
	err := tc.DatasetScopedClient.Get(endpoint, dataset, id, &trigger)
	if err != nil {
		return nil, err
	}
	return &trigger, nil
}

func (tc *Client) Update(dataset string, trigger *Trigger) error {
	return tc.DatasetScopedClient.Update(endpoint, dataset, trigger.ID, trigger)
}

func (tc *Client) Delete(dataset, id string) error {
	return tc.DatasetScopedClient.Delete(endpoint, dataset, id)
}
