package triggers

import queriesAPI "github.com/honeycombio/libhoney-go/api/queries"

type Trigger struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Query       queriesAPI.Query `json:"query"`

	// Threshold
	// Recipients
}
