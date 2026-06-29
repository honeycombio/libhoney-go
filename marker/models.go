package marker

type baseURL string

const (
	USHost baseURL = "https://api.honeycomb.io"
	EUHost baseURL = "https://api.eu1.honeycomb.io"
)

type createMarkerRequest struct {
	// Some request fields are not implemented here. See https://docs.honeycomb.io/api/markers/create-a-marker

	// StartTime int64  `json:"start_time"`
	// EndTime   int64  `json:"end_time"`
	Message string `json:"message"`
	Type    string `json:"type"`
	// URL       string `json:"url"`
}

type createMarkerSuccessResponse struct {
	// Some request fields are not implemented here. See https://docs.honeycomb.io/api/markers/create-a-marker
	ID string `json:"id"`
}

type createMarkerErrorResponse struct {
	Error string `json:"error"`
}
