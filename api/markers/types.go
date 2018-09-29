package markers

type Marker struct {
	ID string `json:"id"`
	// StartTime unix timestamp truncates to seconds
	StartTime int64 `json:"start_time"`
	// EndTime unix timestamp truncates to seconds
	EndTime int64 `json:"end_time"`
	// Message is optional free-form text associated with the marker
	Message string `json:"message"`
	// Type is an optional marker identifier, eg 'deploy' or 'chef-run'
	Type string `json:"type"`
	// URL is an optional url associated with the marker
	URL string `json:"url"`
}
