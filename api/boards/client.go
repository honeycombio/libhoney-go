package boards

import (
	"github.com/honeycombio/libhoney-go/api"
)

const endpoint = "/1/boards"

type Client struct {
	api.TeamScopedClient
}

func (bc *Client) Create(board *Board) error {
	return bc.TeamScopedClient.Create(endpoint, board)
}

func (bc *Client) List() ([]*Board, error) {
	boards := []*Board{}
	err := bc.TeamScopedClient.List(endpoint, &boards)
	if err != nil {
		return nil, err
	}
	return boards, nil
}

func (bc *Client) Get(id string) (*Board, error) {
	board := Board{}
	err := bc.TeamScopedClient.Get(endpoint, id, &board)
	if err != nil {
		return nil, err
	}
	return &board, nil
}

func (bc *Client) Update(board *Board) error {
	return bc.TeamScopedClient.Update(endpoint, board.ID, board)
}

func (bc *Client) Delete(id string) error {
	return bc.TeamScopedClient.Delete(endpoint, id)
}
