package boards

import queriesAPI "github.com/honeycombio/libhoney-go/api/queries"

type Board struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Queries     []BoardQuery `json:"queries"`
}

type BoardQuery struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Dataset     string           `json:"dataset"`
	Query       queriesAPI.Query `json:"query"`
}
